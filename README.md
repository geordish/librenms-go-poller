# Experimental Poller written in Go for Librenms (https://librenms.org)
# Install
## Prerequisites
A working LibreNMS install. Disable the existing poller. Discovery still needs to be running

Go is (obviously) required

Debian: `apt-get install golang`

RabbitMQ is used for passing messages between the components

Debian: `apt-get install rabbitmq-server`


## Setting up the go workspace
Follow this page to set up a workspace for Go: https://golang.org/doc/code.html

Install the following packages (go get <package>)
* github.com/streadway/amqp
* github.com/davecgh/go-spew/spew
* github.com/go-sql-driver/mysql
* gopkg.in/yaml.v2
* github.com/soniah/gosnmp

clone the poller into src/librenms
`git clone https://github.com/geordish/librenms-go-poller.git librenms`

Build the components
`go install librenms/controller`
`go install librenms/worker`
`go install librenms/storage/screen`

Copy default.yaml into /bin directory
`cp librenms/default.yaml bin/`

Copy OS definitions into bin directory
`cp -R /opt/librenms/includes/definitions bin/`

# Configuration
Configuration for the AMQP server and database is possible within default.yaml

# Running
The controller is (currently) designed to work on a cron job. It sends a list of jobs to a Queue within RabbitMQ. This queue is defined to hand out each job to an avaliable worker

The worker runs indefinitely. Multiple of these can be run across different servers. This should help the poller scale horizontally. Once the worker has processed a job, it writes the collected stats into another RabbitMQ queue. 

This queue is defined as a fanout queue, meaning each listener gets a copy of the data. This means multiple storage strategies should be simple to implement (RRDTool/InfluxDB...). A module is created that attaches to the stats queue, and processes the stats accordingly.
 
Currently there is no actual storage of data. The only stats storage strategy currently implements just dumps the stats out to a terminal

# Known Issues
We have been a bit fast and loose in the past with the YAML OS definitions (I'm not sure if this is allowed in the spec. I assume it is since PHP has no issues with parsing it)

There are a number of places where the definition for an OS cannot be loaded due to a syntax error. Most of these are quite simple. Something like the following has been done:
` - OID: 1.2.3.4.5`

This needs changing to:
` - OID:
    - 1.2.3.4.5`

There are a few other instances where this is the case, such as sysDescr

# Further work
There is a lot that needs to be done to make this remotely usable. 
* Make the project adhere to Effective Go guidelines (https://golang.org/doc/effective_go.html)
* Implement a decent SNMP collector
* Modularise the polling modules cleanly (Go Plugins perhaps?)
* Actually implement some storage strategies

