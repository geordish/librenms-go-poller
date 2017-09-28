package main

import (
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	gosnmp "github.com/soniah/gosnmp"
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

// Oh god, this isn't threadsafe :(
var stats amqp.StatisticMessage

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
			log.Printf("Host: %s, Module: %s", message.Hostname, message.PollingModule)

			stats = amqp.StatisticMessage{}
			stats.Records = make(map[int]map[string]amqp.Stat)
			stats.Hostname = message.Hostname
			stats.PollingModule = message.PollingModule

			if message.SNMP.IP == "192.168.88.1" {
				if message.PollingModule == "ports" {
					params := &gosnmp.GoSNMP{
						Target:    message.SNMP.IP,
						Port:      uint16(message.SNMP.Port),
						Community: message.SNMP.Community,
						Version:   gosnmp.Version2c,
						Timeout:   time.Duration(2) * time.Second,
					}
					err := params.Connect()
					if err != nil {
						log.Fatalf("Connect() err: %v", err)
					}
					defer params.Conn.Close()

					oids := "1.3.6.1.2.1.31.1.1.1"
					err2 := params.BulkWalk(oids, printValue)
					if err2 != nil {
						log.Printf("BulkWalk() err: %v", err2)
					}
					stats.Time = time.Now()
					amqp.SendStatsMessage(ch, stats)
				}
			}
		}
	}()

	log.Printf(" [*] Waiting for messages. To exit press CTRL+C")
	<-forever
}

/*
	This is all pretty awful, and not at all scalable.
	Looking around, there doesn't appear to be a go SNMP
	implementation that can deal with MIBs in a readable format.
	We have a few options.

	1) 	Continue like below. There will be lots (and lots...) of mapping OIDs to
		their textual readble formats, and trying to work out how to break the index off. Nasty.
	2) 	Exec to snmpwalk commands like we do in the existing poller. I'm not convinced
		that this is a great idea. There is quite a bit of extra overhead involved in
		shelling out to another process.
	3) 	Implement SNMP calls using the Net-SNMP C API
		(http://www.net-snmp.org/wiki/index.php/TUT:Simple_Application) and cgo
		(https://golang.org/cmd/cgo/)

		The final method is probably the most scalable, but looks to be quite a bit of work.
*/
func printValue(pdu gosnmp.SnmpPDU) error {
	m := make(map[string]string)

	m[".1.3.6.1.2.1.31.1.1.1.1"] = "ifName"
	m[".1.3.6.1.2.1.31.1.1.1.2"] = "ifInMulticastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.3"] = "ifInBroadcastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.4"] = "ifOutMulticastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.5"] = "ifOutBroadcastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.6"] = "ifHCInOctets"
	m[".1.3.6.1.2.1.31.1.1.1.7"] = "ifHCInUcastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.8"] = "ifHCInMulticastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.9"] = "ifHCInBroadcastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.10"] = "ifHCOutOctets"
	m[".1.3.6.1.2.1.31.1.1.1.11"] = "ifHCOutUcastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.12"] = "ifHCOutMulticastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.13"] = "ifHCOutBroadcastPkts"
	m[".1.3.6.1.2.1.31.1.1.1.14"] = "ifLinkUpDownTrapEnable"
	m[".1.3.6.1.2.1.31.1.1.1.15"] = "ifHighSpeed"
	m[".1.3.6.1.2.1.31.1.1.1.16"] = "ifPromiscuousMode"
	m[".1.3.6.1.2.1.31.1.1.1.17"] = "ifConnectorPresent"
	m[".1.3.6.1.2.1.31.1.1.1.18"] = "ifAlias"
	m[".1.3.6.1.2.1.31.1.1.1.19"] = "ifCounterDiscontinuityTime"

	indexpos := strings.LastIndex(pdu.Name, ".")
	oid := pdu.Name[0:indexpos]
	index, _ := strconv.Atoi(pdu.Name[indexpos+1:])

	fmt.Printf("%s.%d = ", m[oid], index)

	mm, ok := stats.Records[index]
	if !ok {
		mm = make(map[string]amqp.Stat)
		stats.Records[index] = mm
	}
	var s amqp.Stat
	s.Name = m[oid]
	switch pdu.Type {
	case gosnmp.OctetString:
		s.Type = "String"
		s.Value = string(pdu.Value.([]byte))
		b := pdu.Value.([]byte)
		fmt.Printf("STRING: %s\n", string(b))
	case gosnmp.Counter32:
		s.Type = "Counter"
		s.Value = fmt.Sprintf("%d", gosnmp.ToBigInt(pdu.Value))
		fmt.Printf("COUNTER32: %d\n", gosnmp.ToBigInt(pdu.Value))
	case gosnmp.Gauge32:
		s.Type = "Gauge"
		s.Value = fmt.Sprintf("%d", gosnmp.ToBigInt(pdu.Value))
		fmt.Printf("GAUGE32: %d\n", gosnmp.ToBigInt(pdu.Value))
	case gosnmp.Counter64:
		s.Type = "Counter"
		s.Value = fmt.Sprintf("%d", gosnmp.ToBigInt(pdu.Value))
		fmt.Printf("COUNTER64: %d\n", gosnmp.ToBigInt(pdu.Value))
	default:
		fmt.Printf("TYPE %d: %d\n", pdu.Type, gosnmp.ToBigInt(pdu.Value))
	}

	mm[m[oid]] = s
	return nil
}
