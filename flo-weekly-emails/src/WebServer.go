package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	instana "github.com/instana/go-sensor"
	ot "github.com/opentracing/opentracing-go"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/schema"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	//_ "gitlab.com/flotechnologies/flo-weekly-emails/docs"
)

const (
	HTTP_DEFAULT_PORT            = "8080"
	ENVVAR_HTTP_PORT             = "FLO_HTTP_PORT"
	ENVVAR_HTTP_LOG_PATH_IGNORES = "FLO_HTTP_LOG_PATH_IGNORES" //sample: FLO_HTTP_LOG_PATH_IGNORES=/ping|GET POST /queue|HEAD /something/something
)

type WebServer struct {
	router     *gin.Engine
	httpSvr    *http.Server
	instana    *instana.Sensor
	validate   *Validator
	closers    []ICloser
	logIgnores map[string]string
	state      int32
	log        *Logger
}

type ICloser interface {
	Open()
	Close()
	Name() string
}

func CreateWebServer(validator *Validator, log *Logger, registerRoutes func(*WebServer), closers []ICloser) *WebServer {
	if log == nil {
		log = DefaultLogger()
	}
	port, e := strconv.ParseInt(getEnvOrDefault(ENVVAR_HTTP_PORT, HTTP_DEFAULT_PORT), 10, 32)
	if e != nil {
		log.Fatal("CreateWebServer: http port %v", e.Error())
		return nil
	} else if port <= 0 || port > 65535 {
		log.Fatal("CreateWebServer: port range is invalid %v", port)
		return nil
	} else if registerRoutes == nil {
		log.Fatal("CreateWebServer: registerRoutes is nil")
		return nil
	}
	ws := WebServer{
		log:      log.CloneAsChild("ws"),
		router:   gin.New(),
		closers:  closers,
		validate: validator,
	}
	ws.presetLogIgnores()
	ws.router.Use(gin.Recovery()) //should be the outer most middleware for best crash protection
	if ws.log.isDebug {
		ws.router.Use(gin.LoggerWithFormatter(ws.logColor))
	} else {
		ws.router.Use(gin.LoggerWithFormatter(ws.logLine))
		ws.initInstana()
	}

	ws.router.NoRoute(ws.noRoute)
	ws.router.NoMethod(ws.noMethod)
	ws.router.GET("/docs", func(c *gin.Context) { // Swagger Setup
		c.Redirect(http.StatusFound, "/swagger/index.html")
	})
	ws.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/swagger/doc.json")))
	ws.httpSvr = &http.Server{ // Create web server instance
		Addr:         fmt.Sprintf("0.0.0.0:%v", port),
		WriteTimeout: time.Second * 15, // Good practice to set timeouts to avoid Slowloris attacks.
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      ws.router, // Pass our instance of gorilla/mux in.
	}
	registerRoutes(&ws) // register routes
	return &ws
}

func (ws *WebServer) noRoute(c *gin.Context) {
	ws.HttpError(c, 404, "Path Not Found", nil)
}

func (ws *WebServer) noMethod(c *gin.Context) {
	ws.HttpError(c, 405, "Method Missing", nil)
}

func (ws *WebServer) Open() *WebServer {
	if ws == nil {
		return nil
	}
	ws.log.Debug("Opening %v workers...", len(ws.closers))
	// Run our server in a goroutine (separate thread) so that it doesn't block.
	go func(w *WebServer) {
		w.log.Notice("Open: Starting HTTP Api @ %v", w.httpSvr.Addr)
		if err := w.httpSvr.ListenAndServe(); err != nil {
			w.log.Error(err.Error())
		}
	}(ws)
	for i, c := range ws.closers {
		if c == nil {
			continue
		}
		go func(n int, w ICloser) {
			defer panicRecover(ws.log, "Open worker #%v %v", n, w.Name())
			ws.log.Trace("Opening worker #%v %v", n, w.Name())
			w.Open()
			ws.log.Debug("Open worker OK #%v %v", n, w.Name())
		}(i, c)
	}
	ws.log.Info("Opened")
	return ws
}

func (ws *WebServer) Close() {
	if ws == nil || !atomic.CompareAndSwapInt32(&ws.state, 0, 1) {
		return
	}
	ws.log.PushScope("Close")
	defer ws.log.PopScope()

	ws.log.Info("Begin")
	// Create a deadline to wait for.
	wait := time.Duration(3 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	if err := ws.httpSvr.Shutdown(ctx); err != nil {
		ws.log.IfWarn(err)
	}
	for _, c := range ws.closers {
		ws.tryClose(c)
	}
	ws.log.Info("Done")
}

func (ws *WebServer) tryClose(c ICloser) {
	if c == nil {
		return
	}
	defer panicRecover(ws.log, "tryClose")
	c.Close()
}

func (ws *WebServer) initInstana() *WebServer {
	var deltaName = APP_NAME
	sn := strings.TrimSpace(getEnvOrDefault("INSTANA_SERVICE_NAME", deltaName))
	// Get environment
	var env = strings.TrimSpace(getEnvOrDefault("ENVIRONMENT", getEnvOrDefault("ENV", "")))
	// If INSTANA_SERVICE_NAME is not set and we have ENV set, see which one it is
	if deltaName == sn && len(env) > 0 {
		// If we are NOT prod/production, then append suffix
		if !strings.EqualFold(env, "prod") && !strings.EqualFold(env, "production") {
			sn = deltaName + "-" + strings.ToLower(env)
		}
	}
	ws.instana = instana.NewSensor(sn)
	// Initialize the Open Tracing. Do not log anything other than WARN/ERRORS. Logz.io and Kibana logs from stdio.
	ot.InitGlobalTracer(instana.NewTracerWithOptions(&instana.Options{
		Service:  sn,
		LogLevel: instana.Warn}))
	return ws
}

func (ws *WebServer) presetLogIgnores() *WebServer {
	ws.logIgnores = make(map[string]string)
	for _, ip := range strings.Split(getEnvOrDefault(ENVVAR_HTTP_LOG_PATH_IGNORES, ""), "|") {
		verbsPath := strings.Split(ip, " ")
		if vl := len(verbsPath); vl == 1 { //path only
			ws.logIgnores[ip] = ""
		} else if vl > 1 { //verbs & path
			ws.logIgnores[verbsPath[vl-1]] = strings.Join(verbsPath[0:vl-1], " ")
		}
	}
	return ws
}

func (ws *WebServer) canLog(param gin.LogFormatterParams) bool {
	if param.StatusCode < 1 || param.StatusCode >= 300 {
		return true
	} else if verbs, ok := ws.logIgnores[param.Request.URL.Path]; ok {
		if verbs == "" {
			return false
		}
		return !strings.Contains(verbs, param.Method)
	}
	return true
}

func (ws *WebServer) logColor(param gin.LogFormatterParams) string {
	if !ws.canLog(param) {
		return ""
	}
	statusColor := LL_DebugColor
	status := "DEBUG"
	if param.StatusCode >= 500 {
		statusColor = LL_ErrorColor
		status = "ERROR"
	} else if param.StatusCode >= 400 || param.Latency.Seconds() > 1 {
		statusColor = LL_WarningColor
		status = "WARN"
	} else if param.StatusCode <= 100 {
		statusColor = LL_InfoColor
	}
	return fmt.Sprintf("%v%v%v %v%v %v%v %vms %v %v %v %v %v %v%v%v\n",
		LL_TraceColor,
		param.TimeStamp.Format("15:04:05"),
		LL_ResetColor,
		statusColor,
		status,
		param.StatusCode,
		LL_ResetColor,
		param.Latency.Milliseconds(),
		LL_BgGray,
		param.Request.Method,
		param.Request.URL.Path,
		LL_ResetColor,
		param.Request.Header,
		statusColor,
		param.ErrorMessage,
		LL_ResetColor,
	)
}

func (ws *WebServer) logLine(param gin.LogFormatterParams) string {
	if !ws.canLog(param) {
		return ""
	}
	var status string
	if param.StatusCode >= 500 {
		status = "ERROR"
	} else if param.StatusCode >= 400 || param.Latency.Seconds() > 1 {
		status = "WARN"
	} else {
		status = "DEBUG"
	}
	return fmt.Sprintf("%v %v %v %vms %v %v %v %v\n",
		param.TimeStamp.Format("2006-01-02T15:04:05Z"),
		status,
		param.StatusCode,
		param.Latency.Milliseconds(),
		param.Request.Method,
		param.Request.URL.Path,
		param.Request.Header,
		param.ErrorMessage,
	)
}

func (ws *WebServer) Logger() *Logger {
	return ws.log
}

func (ws *WebServer) Validator() *Validator {
	return ws.validate
}

type HttpErr struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Trace   string `json:"developer,omitempty"`
}

func (e *HttpErr) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (ws *WebServer) HttpError(c *gin.Context, code int, msg string, err error) error {
	ws.log.PushScope("HttpE")
	defer ws.log.PopScope()

	rv := HttpErr{Code: code, Message: msg}
	if err != nil {
		rv.Trace = err.Error()
	}

	ll := LL_WARN
	if code == 0 || code >= 500 {
		ll = LL_ERROR
	} else if code < 400 {
		ll = LL_NOTICE
	}
	ws.log.Log(ll, "HTTP %v | %v | %v", rv.Code, rv.Message, rv.Trace)
	c.JSON(code, rv)
	return &rv
}

func (ws *WebServer) HttpReadBody(c *gin.Context, v interface{}) error {
	ws.log.PushScope("HttpRB")
	defer ws.log.PopScope()

	r := c.Request
	if r.Body == nil || r.ContentLength <= 0 {
		return ws.HttpError(c, 400, "Empty body", nil)
	}
	defer r.Body.Close()

	err := json.NewDecoder(r.Body).Decode(&v) //quicker than marshaling to str
	if err != nil {
		return ws.HttpError(c, 400, "Can't parse body", err)
	}
	if err := ws.validate.Struct(v); err != nil {
		return ws.HttpError(c, 400, "Bad arguments", err)
	}
	return nil
}

func (ws *WebServer) HttpReadQuery(c *gin.Context, v interface{}) error {
	ws.log.PushScope("HttpRQ")
	defer ws.log.PopScope()

	decoder := schema.NewDecoder()
	decoder.IgnoreUnknownKeys(true)
	if err := decoder.Decode(v, c.Request.URL.Query()); err != nil {
		return ws.HttpError(c, 400, "Can't parse query", err)
	}
	if err := ws.validate.Struct(v); err != nil {
		//var fieldErrs []validator.FieldError = err.(validator.ValidationErrors)
		return ws.HttpError(c, 400, "Bad arguments", err)
	}
	return nil
}
