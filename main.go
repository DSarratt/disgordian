// Main function
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/BurntSushi/toml"
	"golang.org/x/net/websocket" // Go get golang.org/x/net/websocket
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

// Base URL for the REST api
const BASE_URL = "https://discordapp.com/api"

// What version of the gateway protocol do we speak?
const GATEWAY_VERSION = "?v=5&encoding=json"

// What does a heartbeat look like?
const HEARTBEAT_MSG = `{"op": 1, "d": %d}`

// Filename of the ini config
var CONFIG_FILE string

///////////////////////////////////////////////////////////////////////
// Globals shared between init/main and the ReadBuffer
// Outgoing websocket messages should be sent here
var SendQueue = make(chan string)

// Incoming websocket messages are routed through here
var RecvQueue = make(chan Payload)

// This is the websocket itself
var ws *websocket.Conn

// How long do we wait between heartbeats?
var hbLength int

///////////////////////////////////////////////////////////////////////

// Disgordian's config entries
type ConfigFormat struct {
	BotToken string
}

// And the struct where we store them
var Config ConfigFormat

// What does the basic Discord payload look like?
type Payload struct {
	// Op is a pointer because we need to know the difference between 0 and nil
	Op *int
	S  int
	T  string
	D  map[string]*json.RawMessage
}

// Somewhat circuitous way to print a Payload (by converting it back to JSON...)
func (p Payload) String() string {
	val, err := json.Marshal(p)
	if err != nil {
		// Marshalling failed???
		return "{}"
	} else {
		return string(val)
	}
}

// Logging functions
var (
	Debug   *log.Logger
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

// LogInit sets up the three logging functions (Debug, Info, Error).
// Thanks to William Kennedy - https://www.goinggo.net/2013/11/using-log-package-in-go.html
func LogInit(
	debugHandle io.Writer,
	infoHandle io.Writer,
	errorHandle io.Writer) {

	// TODO: Disable if this impacts performance
	Debug = log.New(debugHandle,
		"DEBUG: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(infoHandle,
		"INFO : ",
		log.Ldate|log.Ltime)

	Error = log.New(errorHandle,
		"ERROR: ",
		log.Ldate|log.Ltime)
}

// Read config file into global struct
func readConfig(filename string) {
	_, err := toml.DecodeFile(filename, &Config)
	if err != nil {
		panic(fmt.Sprintf("Failed to read config file: %v", err))
	}
}

// The main process loop:
// Reads payloads from the websocket and handles them
// Delivers any messages queued for sending
// Sends heartbeats
func main() {
	// Clean up sockets nicely when we finish
	defer ws.Close()

	// Add a signal handler
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)

	// Start buffering incoming messages into our queue
	go ReadBuffer()

	// Start the heartbeat ticker
	var seqNo int
	pacemaker := time.Tick(time.Duration(hbLength) * time.Millisecond)

	// Poll for messages
	for {
		select {
		// Receive any incoming messages
		case msg, open := <-RecvQueue:
			if !open {
				Debug.Printf("Receive channel closed, SendLoop exiting")
				if seqNo == 0 {
					Error.Printf("Exiting before any messages received, probable login failure")
				}
				return
			}
			// Update sequence number
			if msg.S != 0 {
				seqNo = msg.S
			}
			// TODO: Handle incoming messages

		// Deliver any outgoing messages
		case msg, open := <-SendQueue:
			if !open {
				Debug.Printf("Send channel closed, SendLoop exiting")
				return
			}
			Debug.Printf("Sending %v", msg)
			websocket.Message.Send(ws, msg)

		// If it's time to heartbeat, create one
		case <-pacemaker:
			msg := fmt.Sprintf(HEARTBEAT_MSG, seqNo)
			Debug.Printf("Sending %v", msg)
			websocket.Message.Send(ws, msg)

		// Signals aren't handled right now, we just exit
		case <-sigchan:
			return
		}
	}
}

// Read incoming messages from websocket, and push to RecvQueue
func ReadBuffer() {
	defer close(RecvQueue)
	var payload Payload
	for {
		if err := websocket.JSON.Receive(ws, &payload); err != nil {
			// Websocket is probably closed, we can exit now
			Debug.Printf("Websocket closed, disconnecting")
			break
		}
		Debug.Printf("Received payload %v", payload)
		// Push to the main loop
		RecvQueue <- payload
	}
}

// Open the websocket and login
func init() {
	// Get our config file, first of all
	flag.StringVar(&CONFIG_FILE, "config", "config.ini", "Path to the config file")
	flag.Parse()
	readConfig(CONFIG_FILE)

	// Setup logging
	LogInit(os.Stdout, os.Stdout, os.Stdout)

	// GET request to the Discordian API, to find the gateway URL
	resp, err := http.Get(BASE_URL + "/gateway")
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != 200 {
		panic(fmt.Sprintf("Received non-200 status code %d from gateway URL fetch", resp.StatusCode))
	}

	// Read response into a buffer
	body, err := ioutil.ReadAll(resp.Body)
	Debug.Printf("Received %d bytes from gateway URL fetch\n", len(body))

	// Decode the JSON
	var outputMap map[string]string
	err = json.Unmarshal(body, &outputMap)
	if err != nil {
		panic(fmt.Sprintf("Couldn't decode HTTP response: %v", err))
	}
	url, ok := outputMap["url"]
	if !ok {
		panic("Didn't receive a url from the gateway URL fetch")
	}
	Debug.Printf("Received gateway URL: %v", url)

	// TODO: Initialise plugins
	//var pluginlist []func(<-chan *map[string]interface{})
	// Dynamic plugin loading: functions can be first-class, but not packages.
	// Define a startup script that writes a .go file loading all the plugin.Start()
	// functions into an array, then recompiles the bot. Ugh?
	// The 'plugins' package would be nice, but it's only in dev...

	// Start websocket
	ws, err = websocket.Dial(url+GATEWAY_VERSION, "", "https://discordapp.com")
	if err != nil {
		panic(fmt.Sprintf("Failed to open websocket: %v", err))
	}
	Debug.Printf("Websocket opened")

	// Receive first payload, get heartbeat interval
	var payload Payload
	websocket.JSON.Receive(ws, &payload)
	Debug.Printf("Received payload %v", payload)
	if payload.Op == nil {
		panic(fmt.Sprintf("Couldn't find Op of incoming payload"))
	}
	json.Unmarshal(*payload.D["heartbeat_interval"], &hbLength)
	if hbLength == 0 {
		panic(fmt.Sprintf("Couldn't get heartbeat interval from %v", payload))
	}
	Debug.Printf("Heartbeat length: %dms", hbLength)

	// Send login
	loginMsg := fmt.Sprintf(`{
		"op": 2,
		"d": {
			"token": "%v",
			"properties": {
				"$os": "linux",
				"$browser": "Disgordian",
				"$device": "Disgordian",
				"$referrer": "",
				"$referring_domain": ""
			},
			"compress": false,
			"large_threshold": 250,
			"shard": [0,1]
		}}`, Config.BotToken)
	websocket.Message.Send(ws, loginMsg)
	Debug.Printf("Sent login")
}
