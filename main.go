// Main function
package main

import (
	"encoding/json"
	"fmt"
	//"golang.org/x/net/websocket"			// Go get golang.org/x/net/websocket
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
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

func LogInit(
	// Thanks to William Kennedy
	// https://www.goinggo.net/2013/11/using-log-package-in-go.html
	debugHandle io.Writer,
	infoHandle io.Writer,
	errorHandle io.Writer) {

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
	// TODO: Read config from file
	config := make(map[string]interface{})
	config["bot_token"] = ""
	
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
	// If we get more than 2kB of response something has gone wrong
	body, err := ioutil.ReadAll(resp.Body)
	Debug.Printf("Received %d bytes from gateway URL fetch\n", len(body))
	// Decode the JSON
	var output_map map[string]interface{}
	err = json.Unmarshal(body, &output_map)
	if err != nil {
		panic(err)
	}
	url, ok := output_map["url"]
	if !ok {
		panic("Didn't receive a url from the gateway URL fetch")
	}
	Debug.Printf("Received gateway URL: %q", url)
	
	// Initialise plugins
	//var pluginlist []func(<-chan *map[string]interface{})
	// Dynamic plugin loading: functions can be first-class, but not packages.
	// Define a startup script that writes a .go file loading all the plugin.Start()
	// functions into an array, then recompiles the bot. Ugh?
	// The 'plugins' package would be nice, but it's only in dev...
	
	// Start websocket
	
	// Login, hello
	
	// Enter read loop
}
