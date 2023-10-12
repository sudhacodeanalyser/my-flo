package com.flo.communication

import com.flo.communication.utils.IKafkaConsumerMetrics
import net.manub.embeddedkafka.{EmbeddedKafka, EmbeddedKafkaConfig}
import org.apache.kafka.common.serialization.{StringDeserializer, StringSerializer}
import org.scalatest.WordSpec

class EncryptedKafkaConsumerTest extends WordSpec with EmbeddedKafka with EncryptionTestUtils {
  private val topic = "kafka-consumer-encrypted-test"
  private val groupId = "group0"
  private val serializer = "org.apache.kafka.common.serialization.StringSerializer"

  "The EncryptedKafkaConsumer" should {
    "receive encrypted data from kafka" in {
      val customBrokerConfig = Map(
        "group.min.session.timeout.ms" -> "1"
      )
      val embeddedKafkaConfig = EmbeddedKafkaConfig(
        kafkaPort = 0,
        zooKeeperPort = 0,
        customBrokerProperties = customBrokerConfig
      )

      implicit val kafkaSerializer = new StringSerializer
      implicit val kafkaDeserializer = new StringDeserializer

      withRunningKafkaOnFoundPort(embeddedKafkaConfig) { implicit actualConfig =>
        val kafkaHost = s"localhost:${actualConfig.kafkaPort}"

        // send message
        val messageToSend = encrypt("{\"a\":\"Hello\",\"b\":\"World\"}")
        publishToKafka(topic, messageToSend)

        // create consumer
        val metrics = new IKafkaConsumerMetrics {
          override def newDeserializationError(): Unit = {}

          override def newProcessorError(): Unit = {}

          override def newSuccess(): Unit = {}
        }

        val consumer = new KafkaConsumer(
          kafkaHost,
          groupId,
          topic,
          metrics,
          maxPollRecords = 1,
          sessionTimeoutInMilliseconds = 4000,
          messageDecoder = Some((message: String) => decrypt(message))
        )
        var records: List[TopicRecord[MessageData]] = Nil

        consumer.consume[MessageData](
          x => {
            val args = x.replace("{", "").replace("}", "").replaceAll("\"", "").split(",").map(y => y.split(":")(1))

            MessageData(args(0), args(1))
          },
          x => {
            records = records ++ List(x)
            consumer.shutdown()
          }
        )

        if (!records.isEmpty) {
          val msg = records.head
          assert("Hello" == msg.data.a)
          assert("World" == msg.data.b)
        } else {
          fail()
        }
      }
    }
  }
}