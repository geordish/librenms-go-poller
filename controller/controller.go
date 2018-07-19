package main

import (
  "database/sql"
  "fmt"
  "github.com/davecgh/go-spew/spew"
  _ "github.com/go-sql-driver/mysql"
  "gopkg.in/yaml.v2"
  "io/ioutil"
  "librenms/amqp"
  "log"
)

var CONFIG Config

func failOnError(err error, msg string) {
  if err != nil {
    log.Fatalf("%s: %s", msg, err)
  }
}

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
  IfXmcbc bool     `yaml:"ifXmcbc"`
  Rfc1628 bool     `yaml:"rfc1628_compat"`
  Over    []struct {
    Graph string `yaml:"graph"`
    Text  string `yaml:"text"`
  } `yaml:"over"`
  Icon               string   `yaml:"icon"`
  Goodif             []string `yaml:"good_if"`
  IfName             bool     `yaml:"ifname"`
  Processors_Stacked bool     `yaml:"processor_stacked"`
  Discovery          []struct {
    SysDescr        []string `yaml:"sysDescr"`
    SysDescr_except []string `yaml:"sysDescr_except"`
    SysObjectId     string `yaml:"sysObjectID"`
    SysDescr_regex	string `yaml:"sysDescr_regex"`
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
  err = yaml.Unmarshal(file, &CONFIG)
  if err != nil {
    log.Printf(FileName)
    log.Fatal(err)
  }
}

func main() {
  spew.Dump("Stop error about spew not being ran")
  var (
    device_id int
    hostname  string
    ip        []byte
    community string
    /*
    authlevel string
    authname string
    authpass string
    authalgo string
    cryptopass string
    cryptoalgo string
    */
    snmpver   string
    port      int
    transport string
    ip_string string
    os        string
  )

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

  //db, err := sql.Open("mysql", "librenms:librenms@tcp(127.0.0.1:3306)/librenms")
  mysql_connect_string := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", CONFIG.Database.User, CONFIG.Database.Password, CONFIG.Database.Host, CONFIG.Database.Port, CONFIG.Database.Database)
  log.Print(mysql_connect_string)
  db, err := sql.Open("mysql", mysql_connect_string)
  if err != nil {
    panic(err.Error())
  }
  defer db.Close()

  err = db.Ping()
  if err != nil {
    panic(err.Error())
  }
  stmtOut, err := db.Query("SELECT device_id, hostname, ip, community, snmpver, port, transport, os FROM devices;")
  //stmtOut, err := db.Prepare("SELECT device_id, hostname, ip, community, authlevel, authname, authpass, authalgo, crytopass, cryptoalgo, snmpver, port, transport FROM devices;")
  if err != nil {
    panic(err.Error())
  }
  defer stmtOut.Close()

  for stmtOut.Next() {
    err := stmtOut.Scan(
      &device_id,
      &hostname,
      &ip,
      &community,
      &snmpver,
      &port,
      &transport,
      &os,
    )
    if err != nil {
      panic(err.Error())
    }

    Definition := loadOS(os)

    ip_string = ""
    if len(ip) == 4 {
      //IPv4
      ip_string = fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])

    } else if len(ip) == 16 {
      //IPv6
      ip_string = fmt.Sprintf("%02X%02X:%02X%02X:%02X%02X:%02X%02X:%02X%02X:%02X%02X:%02X%02X:%02X%02X",
      ip[0], ip[1], ip[2], ip[3],
      ip[4], ip[5], ip[6], ip[7],
      ip[8], ip[9], ip[10], ip[11],
      ip[12], ip[13], ip[14], ip[15])
    } else {
      log.Println("No IP Address")
      ip_string = hostname
    }

    var m amqp.Message
    m.Hostname = hostname
    m.Time = 12
    m.SNMP.Version = snmpver
    m.SNMP.Community = community
    m.SNMP.Port = port
    m.SNMP.IP = ip_string
    m.SNMP.Transport = transport

    for k, v := range CONFIG.Poller_Modules {
      do_poll := v
      if val, ok := Definition.Poller_Modules[k]; ok {
	do_poll = val
      }
      if do_poll != 0 {
	m.PollingModule = k
	spew.Dump(m)
	amqp.SendMessage(ch, q, m)
      }
    }
  }
  err = stmtOut.Err()

  if err != nil {
    panic(err.Error())
  }
}
