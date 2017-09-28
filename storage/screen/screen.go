package main

import (
	"bytes"
	"encoding/json"
    "github.com/davecgh/go-spew/spew"
    "gopkg.in/yaml.v2"
    "io/ioutil"
    "librenms/amqp"
    "log"
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

    q, status, err := amqp.DeclareStatsQueue(ch)
    if err != nil {
        panic(status)
    }
	
	msgs, _ := amqp.ConsumeQueue(ch, q)

	forever := make(chan bool)

	go func() {
			for d := range msgs {
				var prettyJSON bytes.Buffer
				error := json.Indent(&prettyJSON, d.Body, "", "\t")
				if error != nil {
					log.Println("JSON parse error: ", error)
					return
				}
				log.Printf(string(prettyJSON.Bytes()))
			}
	}()

	log.Printf(" [*] Waiting for logs. To exit press CTRL+C")
	<-forever
}
