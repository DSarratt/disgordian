// Main function
package main

// N.B. When testing with "go run", you have to list all relevant files: go run main.go config.go

// TODO: proper exit handler (a sync.Once exit() function?)
// TODO: handle SIGINT/SIGKILL
// TODO: Scoping/break into functions

import (
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket" // Go get golang.org/x/net/websocket
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Base URL for the REST api
const BASE_URL = "https://discordapp.com/api"

// What version of the gateway protocol do we speak?
const GATEWAY_VERSION = "?v=5&encoding=json"

// These are shared between multiple goroutines
var seq_no int
var seq_lock = &sync.Mutex{}

// Outgoing websocket messages should be sent here
var SendQueue = make(chan string)

// This is the websocket itself
var ws *websocket.Conn

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

func SendHeartbeats(ws *websocket.Conn, hb_length int) {
	// Loops indefinitely, sending heartbeats
	heartbeat_msg := `{"op": 1, "d": %d}`
	for {
		time.Sleep(time.Duration(hb_length) * time.Millisecond)
		seq_lock.Lock()
		temp := fmt.Sprintf(heartbeat_msg, seq_no)
		seq_lock.Unlock()
		SendQueue <- temp
	}
}

func SendLoop(ws *websocket.Conn, ch <-chan string) {
	// Reads indefinitely from a channel for messages to send down the websocket
	for msg := range ch {
		Debug.Printf("Sending %q", msg)
		websocket.Message.Send(ws, msg)
	}
	Debug.Printf("Send channel closed, SendLoop exiting")
}

// Gracefully close any open handles and exit
// This function should be called once only
func ShutDown() {
	close(SendQueue)
	ws.Close()
}

var ShutDownOnce sync.Once

func main() {
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
	var output_map map[string]string
	err = json.Unmarshal(body, &output_map)
	if err != nil {
		panic(fmt.Sprintf("Couldn't decode HTTP response: %q", err))
	}
	url, ok := output_map["url"]
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
		panic(fmt.Sprintf("Failed to open websocket: %q", err))
	}
	defer ShutDownOnce.Do(ShutDown)
	Debug.Printf("Websocket opened")

	// Receive first payload, get heartbeat interval
	var payload Payload
	websocket.JSON.Receive(ws, &payload)
	Debug.Printf("Received payload %v", payload)
	if payload.Op == nil {
		panic(fmt.Sprintf("Couldn't find Op of incoming payload"))
	}
	var hb_length int
	json.Unmarshal(*payload.D["heartbeat_interval"], &hb_length)
	if hb_length == 0 {
		panic(fmt.Sprintf("Couldn't get heartbeat interval from &q", payload))
	}
	Debug.Printf("Heartbeat length: %dms", hb_length)

	// Send login
	login_msg := fmt.Sprintf(`{
		"op": 2,
		"d": {
			"token": %q,
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
		}}`, config["bot_token"])
	websocket.Message.Send(ws, login_msg)
	Debug.Printf("Sent login")

	// Receive the READY message and store sequence number
	websocket.JSON.Receive(ws, &payload)
	Debug.Printf("Received payload %v", payload)
	if payload.Op == nil {
		panic(fmt.Sprintf("Couldn't find Op of incoming payload"))
	}
	seq_lock.Lock()
	seq_no = payload.S
	seq_lock.Unlock()
	Debug.Printf("Sequence number is %d", seq_no)

	// Start the heartbeating loop
	go SendHeartbeats(ws, hb_length)

	// Start the message sending loop
	go SendLoop(ws, SendQueue)

	// Read incoming messages indefinitely
	// N.B. the socket is not gracefully closed on exit!
	for {
		if err = websocket.JSON.Receive(ws, &payload); err != nil {
			// Websocket is probably closed, we can exit now
			Debug.Printf("Websocket closed, disconnecting")
			break
		}
		Debug.Printf("Received payload %v", payload)
		// Update sequence number
		if payload.S != 0 {
			seq_lock.Lock()
			seq_no = payload.S
			seq_lock.Unlock()
		}
	}
}
