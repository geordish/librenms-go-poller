package main

import (
  "encoding/json"
  "github.com/davecgh/go-spew/spew"
  "github.com/k-sone/snmpgo"
  "gopkg.in/yaml.v2"
  "io/ioutil"
  "librenms/amqp"
  "log"
  "strconv"
  "strings"
  "time"
)

var CONFIG Config

type Config struct {
  RabbitMQ struct {
    User     string `yaml:"user"`
    Password string `yaml:"password"`
    Host     string `yaml:"host"`
    Port     int    `yaml:"port"`
  } `yaml:"rabbitmq"`
  Database struct {
    User     string `yaml:"user"`
    Password string `yaml:"password"`
    Host     string `yaml:"host"`
    Port     int    `yaml:"port"`
    Database string `yaml:"database"`
  } `yaml:"database"`

  Storage struct {
    RRDCached struct  {
      Enabled bool  `yaml:"enabled"`
      Host    string `yaml:"host"`
      Port    int    `yaml:"port"`
    } `yaml:"rrdcached"`
  } `yaml:"storage"`

  Poller_Modules map[string]interface{} `yaml:"poller_modules"`
}

type OSDefinition struct {
  OS      string   `yaml:"os"`
  Group   string   `yaml:"group"`
  NoBulk  int      `yaml:"nobulk"`
  MibDir  []string `yaml:"mib_dir"`
  Text    string   `yaml:"text"`
  Type    string   `yaml:"type"`
  IfXmcbc int64    `yaml:"ifXmcbc"`
  Over    []struct {
    Graph string `yaml:"graph"`
    Text  string `yaml:"text"`
  } `yaml:"over"`
  Icon               string   `yaml:"icon"`
  Goodif             []string `yaml:"good_if"`
  IfName             int      `"yaml:ifname"`
  Processors_Stacked int      `yaml:"processor_stacked"`
  Discovery          []struct {
    SysDescr        []string `yaml:"sysDescr"`
    SysDescr_except []string `yaml:"sysDescr_except"`
    SysObjectId     []string `yaml:"sysObjectId"`
  } `yaml:"discovery"`
  Bad_ifXEntry      []string               `yaml:"bad_ifXEntry"`
  Poller_Modules    map[string]interface{} `yaml:"poller_modules"`
  Discovery_Modules map[string]interface{} `yaml:"discovery_modules"`
  Register_mibs     map[string]interface{} `yaml:"register_mibs"`
}

func loadOS(os string) OSDefinition {
  FileName := "definitions/" + os + ".yaml"
  file, err := ioutil.ReadFile(FileName)
  if err != nil {
    log.Fatal(err)
  }
  var t OSDefinition
  err = yaml.UnmarshalStrict(file, &t)
  if err != nil {
    log.Printf(FileName)
    log.Fatal(err)
  }
  return t
}

func loadConfig() {
  FileName := "default.yaml"
  file, err := ioutil.ReadFile(FileName)
  if err != nil {
    log.Fatal(err)
  }
  err = yaml.UnmarshalStrict(file, &CONFIG)
  if err != nil {
    log.Printf(FileName)
    log.Fatal(err)
  }
}

func main() {
  spew.Dump("Stop error about spew not being ran")

  loadConfig()

  conn, ch, status, err := amqp.ConnectAMQP(CONFIG.RabbitMQ.User, CONFIG.RabbitMQ.Password, CONFIG.RabbitMQ.Host, CONFIG.RabbitMQ.Port)

  defer amqp.CloseAMQP(conn, ch)
  if err != nil {
    panic(status)
  }

  q, status, err := amqp.DeclareAMQPQueue(ch, "librenms_pollers")
  if err != nil {
    panic(status)
  }

  status, err = amqp.DeclareStatsExchange(ch)
  if err != nil {
    panic(status)
  }

  msgs, _ := amqp.ConsumeQueue(ch, q)

  forever := make(chan bool)

  go func() {
    for d := range msgs {
      var message amqp.Message
      err := json.Unmarshal(d.Body, &message)
      if err != nil {
        spew.Dump(err)
      }
      log.Printf("Host: %s, IP: %s, Module: %s", message.Hostname, message.SNMP.IP, message.PollingModule)

      var stats amqp.StatisticMessage

      stats = amqp.StatisticMessage{}
      stats.Records = make(map[int]map[string]amqp.Stat)
      stats.Hostname = message.Hostname
      stats.PollingModule = message.PollingModule

      if message.PollingModule == "ports" {
        target := message.SNMP.IP + ":" + strconv.Itoa(message.SNMP.Port)
        snmp, err := snmpgo.NewSNMP(snmpgo.SNMPArguments{
          Version: snmpgo.V2c,
          Address: target,
          Retries: 1,
          Community: message.SNMP.Community,
        })
        if err != nil {
          log.Fatalf("Connect() err: %v", err)
          return
        }

        oids, err := snmpgo.NewOids([]string{
          "1.3.6.1.2.1.31.1.1.1",
        })

        if err != nil {
          log.Fatalf("Failed to parse Oids() err: %v", err)
          return
        }

        if err = snmp.Open(); err != nil {
          log.Fatalf("Connect() err: %v", err)
          return
        }
        defer snmp.Close()

        pdu, err := snmp.GetBulkWalk(oids, 0, 10)
        if err != nil {
          log.Fatalf("Connect() err: %v", err)
          return
        }

        if pdu.ErrorStatus() != snmpgo.NoError {
          log.Fatalf("Connect() err: %v, %v",  pdu.ErrorStatus(), pdu.ErrorIndex())
          return
        }

        for _, val := range pdu.VarBinds() {
          log.Printf("%s = %s: %s\n", val.Oid, val.Variable.Type(), val.Variable)
          m := make(map[string]string)
          m["1.3.6.1.2.1.31.1.1.1.1"] = "ifName"
          m["1.3.6.1.2.1.31.1.1.1.2"] = "ifInMulticastPkts"
          m["1.3.6.1.2.1.31.1.1.1.3"] = "ifInBroadcastPkts"
          m["1.3.6.1.2.1.31.1.1.1.4"] = "ifOutMulticastPkts"
          m["1.3.6.1.2.1.31.1.1.1.5"] = "ifOutBroadcastPkts"
          m["1.3.6.1.2.1.31.1.1.1.6"] = "ifHCInOctets"
          m["1.3.6.1.2.1.31.1.1.1.7"] = "ifHCInUcastPkts"
          m["1.3.6.1.2.1.31.1.1.1.8"] = "ifHCInMulticastPkts"
          m["1.3.6.1.2.1.31.1.1.1.9"] = "ifHCInBroadcastPkts"
          m["1.3.6.1.2.1.31.1.1.1.10"] = "ifHCOutOctets"
          m["1.3.6.1.2.1.31.1.1.1.11"] = "ifHCOutUcastPkts"
          m["1.3.6.1.2.1.31.1.1.1.12"] = "ifHCOutMulticastPkts"
          m["1.3.6.1.2.1.31.1.1.1.13"] = "ifHCOutBroadcastPkts"
          m["1.3.6.1.2.1.31.1.1.1.14"] = "ifLinkUpDownTrapEnable"
          m["1.3.6.1.2.1.31.1.1.1.15"] = "ifHighSpeed"
          m["1.3.6.1.2.1.31.1.1.1.16"] = "ifPromiscuousMode"
          m["1.3.6.1.2.1.31.1.1.1.17"] = "ifConnectorPresent"
          m["1.3.6.1.2.1.31.1.1.1.18"] = "ifAlias"
          m["1.3.6.1.2.1.31.1.1.1.19"] = "ifCounterDiscontinuityTime"
          myoid := val.Oid.String()
          indexpos := strings.LastIndex(myoid, ".")
          oid := myoid[0:indexpos]
          index, _ := strconv.Atoi(myoid[indexpos+1:])

          mm, ok := stats.Records[index]
          if !ok {
            mm = make(map[string]amqp.Stat)
            stats.Records[index] = mm
          }
          var s amqp.Stat
          s.Name = m[oid]
          s.Type = val.Variable.Type()
          s.Value = val.Variable.String()

          mm[m[oid]] = s
        }

        stats.Time = time.Now()
        amqp.SendStatsMessage(ch, stats)
      }
    }
  }()

  log.Printf(" [*] Waiting for messages. To exit press CTRL+C")
  <-forever
}

