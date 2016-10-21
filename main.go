// Main function
package main

// N.B. When testing with "go run", you have to list all relevant files: go run main.go config.go

import (
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket" // Go get golang.org/x/net/websocket
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

// Base URL for the REST api
const BASE_URL = "https://discordapp.com/api"

// What version of the gateway protocol do we speak?
const GATEWAY_VERSION = "?v=5&encoding=json"

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

func main() {
	// TODO: Scoping/break into functions
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
	Debug.Printf("Received gateway URL: %q", url)

	// TODO: Initialise plugins
	//var pluginlist []func(<-chan *map[string]interface{})
	// Dynamic plugin loading: functions can be first-class, but not packages.
	// Define a startup script that writes a .go file loading all the plugin.Start()
	// functions into an array, then recompiles the bot. Ugh?
	// The 'plugins' package would be nice, but it's only in dev...

	// Start websocket
	ws, err := websocket.Dial(url+GATEWAY_VERSION, "", "https://discordapp.com")
	if err != nil {
		panic(fmt.Sprintf("Failed to open websocket: %q", err))
	}
	defer ws.Close()
	Debug.Printf("Websocket opened")

	// TODO: get websocket.JSON working?
	/*
		var data Payload
		websocket.JSON.Receive(ws, &data)
		fmt.Println(data)
	*/
	// Receive first packet, decode from JSON
	// TODO: Timeout on socket read
	var buffer []byte
	websocket.Message.Receive(ws, &buffer)
	Debug.Printf("Received websocket payload: %q", buffer)
	var payload map[string]interface{}
	err = json.Unmarshal(buffer, &payload)
	if err != nil {
		panic(fmt.Sprintf("Couldn't decode websocket payload: %q", err))
	}
	dload, ok := payload["d"].(map[string]interface{})
	if !ok {
		panic(fmt.Sprintf("Couldn't get event data from payload: %q", payload["d"]))
	}
	temp, ok := dload["heartbeat_interval"].(float64)
	if !ok {
		panic(fmt.Sprintf("Couldn't get heartbeat from payload: %q", dload))
	}
	heartbeat := int(temp)
	Debug.Printf("Heartbeat: %dms", heartbeat)

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
	websocket.Message.Receive(ws, &buffer)
	Debug.Printf("Received reply")
	err = json.Unmarshal(buffer, &payload)
	if err != nil {
		panic(fmt.Sprintf("Couldn't decode websocket payload: %q", err))
	}
	temp, ok = payload["s"].(float64)
	if !ok {
		panic(fmt.Sprintf("Couldn't get sequence number from payload: %q", payload))
	}
	sequence := int(temp)
	Debug.Printf("Sequence number is %d", sequence)

	// TODO: Go run heartbeats
	heartbeat_msg := `{"op": 1, "d": %d}`
	for i := 0; i < 3; i++ {
		time.Sleep(time.Duration(heartbeat) * time.Millisecond)
		Debug.Printf(heartbeat_msg, sequence)
		websocket.Message.Send(ws, fmt.Sprintf(heartbeat_msg, sequence))
	}

	// TODO: Enter read loop
}
