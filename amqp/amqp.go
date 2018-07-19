package amqp

// TODO: Establish way of keeping state within module

import (
  "encoding/json"
  "fmt"
  "github.com/streadway/amqp"
  "time"
)

type Message struct {
  Hostname string
  Time     int64
  SNMP     struct {
    Community string
    Version   string
    Port      int
    IP        string
    Transport string
  }
  PollingModule string
}

/*

{
  "Hostname": "my device",
  "PollingModule": "ports",
  "Records": [
  {
    "ID": 1
    "Values": [
    {
      "Name": "INOCTETS",
      "Value": 12345,
      "Type": "DERIVE" // Very RRDTool specific. Is there something better? Maybe just SNMP types? Counter in this case?
      "Min": 0
      "Max": 9999999
    }
    ]

  }
  ]
}
*/
type Stat struct {
  Name  string
  Value string
  Type  string
  Min   int
  Max   int
}
type StatisticMessage struct {
  Hostname      string
  PollingModule string
  Time          time.Time
  Records       map[int]map[string]Stat
}

func ConnectAMQP(User string, Password string, Host string, Port int) (*amqp.Connection, *amqp.Channel,
string, error) {
  amqp_connect_string := fmt.Sprintf("amqp://%s:%s@%s:%d/", User, Password, Host, Port)
  conn, err := amqp.Dial(amqp_connect_string)
  if err != nil {
    return nil, nil, "Failed to connect to AMQP", err
  }
  ch, err := conn.Channel()
  if err != nil {
    return conn, ch, "Failed to open a channel", err
  }
  return conn, ch, "", nil
}

func CloseAMQP(conn *amqp.Connection, ch *amqp.Channel) error {
  err := ch.Close()
  if err != nil {
    return err
  }
  return conn.Close()
}

func DeclareAMQPQueue(ch *amqp.Channel, name string) (amqp.Queue, string, error) {
  q, err := ch.QueueDeclare(
    name,  // name
    false, // durable
    false, // delete when unused
    false, // exclusive
    false, // no-wait
    nil,   // arguments
  )
  if err != nil {
    return q, "Failed to declare a queue", err
  }
  return q, "", nil
}

func DeclareStatsExchange(ch *amqp.Channel) (string, error) {
  err := ch.ExchangeDeclare(
    "librenms_stats",   // name
    "fanout", // type
    true,     // durable
    false,    // auto-deleted
    false,    // internal
    false,    // no-wait
    nil,      // arguments
  )
  if err != nil {
    return "Failed to declare fanout exchange", err
  }
  return "", err
}

func DeclareStatsQueue(ch *amqp.Channel) (amqp.Queue, string, error) {

  q, err := ch.QueueDeclare(
    "",    // name
    false, // durable
    false, // delete when usused
    true,  // exclusive
    false, // no-wait
    nil,   // arguments
  )
  if err != nil {
    return q, "Failed to declare a queue", err
  }

  err = ch.QueueBind(
    q.Name, // queue name
    "",     // routing key
    "librenms_stats", // exchange
    false,
    nil)
    if err != nil {
      return q, "Failed to bind to queue", err
    }

    return q, "", nil
  }

  func SendMessage(ch *amqp.Channel, q amqp.Queue, m Message) (status string, err error) {
    encoded_message, err := json.Marshal(m)

    if err != nil {
      return fmt.Sprintf("Failed to encode %v as JSON.", m), err
    }
    err = ch.Publish(
      "",     // exchange
      q.Name, // routing key
      false,  // mandatory
      false,  // immediate
      amqp.Publishing{
        ContentType: "text/json",
        Body:        encoded_message,
      })
      if err != nil {
        return "Failed to publish message", err
      }
      return "", nil
    }

    func SendStatsMessage(ch *amqp.Channel, m StatisticMessage) (status string, err error) {
      encoded_message, err := json.Marshal(m)

      if err != nil {
        return fmt.Sprintf("Failed to encode %v as JSON.", m), err
      }
      err = ch.Publish(
        "librenms_stats",     // exchange
        "", // routing key
        false,  // mandatory
        false,  // immediate
        amqp.Publishing{
          ContentType: "text/json",
          Body:        encoded_message,
        })
        if err != nil {
          return "Failed to publish message", err
        }
        return "", nil
      }

      func ConsumeQueue(ch *amqp.Channel, q amqp.Queue) (<-chan amqp.Delivery, error) {
        msgs, err := ch.Consume(
          q.Name,
          "",
          true,
          false,
          false,
          false,
          nil,
        )
        return msgs, err
      }

