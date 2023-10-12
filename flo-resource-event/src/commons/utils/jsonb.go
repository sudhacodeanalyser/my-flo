package utils

import (
	"bytes"
	"database/sql/driver"
	"errors"
)

// Postgres' JSONB type. It's a byte array of already encoded JSON (like json.RawMessage)
// which also saves itself correctly to PG's jsonb type.  It would probably also work on
// PG json types.
type JSONB []byte

func (j JSONB) Value() (driver.Value, error) {
	if j.IsNull() {
		//      log.Trace("returning null")
		return nil, nil
	}
	return string(j), nil
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	s, ok := value.([]byte)
	if !ok {
		errors.New("Scan source was not string")
	}
	// I think I need to make a copy of the bytes.
	// It seems the byte slice passed in is re-used
	*j = append((*j)[0:0], s...)

	return nil
}

// MarshalJSON returns *m as the JSON encoding of m.
func (m JSONB) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

// UnmarshalJSON sets *m to a copy of data.
func (m *JSONB) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("json.RawMessage: UnmarshalJSON on nil pointer")
	}
	*m = append((*m)[0:0], data...)
	return nil
}

func (j JSONB) IsNull() bool {
	return len(j) == 0 || string(j) == "null"
}

func (j JSONB) Equals(j1 JSONB) bool {
	return bytes.Equal([]byte(j), []byte(j1))
}
