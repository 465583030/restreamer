package main

import (
	"os"
	"log"
	"time"
	"net/http"
	"encoding/json"
	"restreamer"
)

// configuration file structure
// these should be in a JSON dictionary
// in the configuration file
// note that the keys should be lower case
type Configuration struct {
	// the interface to listen on
	Listen string
	// the connection timeout
	// (both input and output)
	Timeout int
	// the maximum number of packets
	// on the input buffer
	InputBuffer int
	// the size of the output buffer
	// per connection
	// note that each connection will
	// eat at least OutputBuffer * 192 bytes
	// when the queue is full, so
	// you should adjust the value according
	// to the amount of RAM available
	OutputBuffer int
	// the maximum number of concurrent connections
	// per stream URL
	MaxConnections int
	// the list of streams
	Streams []struct {
		// the local URL to serve this stream under
		Serve string
		// the upstream URL
		Remote string
	}
}

// TODO
// this is currently only a stub
// please implement proper status monitoring
func HandleHealth(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Add("Content-Type", "application/json")
	response, err := json.Marshal(map[string]interface{}{
		"status": "ok",
		"viewer": 0,
		"limit": 1000,
		"bandwidth": 0,
	})
	if err == nil {
		writer.WriteHeader(http.StatusOK);
		writer.Write(response)
	} else {
		writer.WriteHeader(http.StatusInternalServerError);
		writer.Write([]byte("500 internal server error"))
		log.Print(err)
	}
}

// TODO
// this is currently only a stub
// please implement proper status monitoring
func HandleStats(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Add("Content-Type", "application/json")
	response, err := json.Marshal(map[string]interface{}{
		"lastUpdate": time.Now().Unix(),
		"total": map[string]interface{}{
			"counter": 0,
			"free": 1000,
		},
		"servers": []interface{}{
			map[string]interface{}{
				"counter": 0,
				"name": "streaming-test.local",
				"free": 1000,
			},
		},
	})
	if err == nil {
		writer.WriteHeader(http.StatusOK);
		writer.Write(response)
	} else {
		writer.WriteHeader(http.StatusInternalServerError);
		writer.Write([]byte("500 internal server error"))
		log.Print(err)
	}
}

// TODO
// this should not be here
// please move all API stuff to a separate module
type StreamStat struct {
	Client *restreamer.Client
}

func (stat *StreamStat) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Add("Content-Type", "text/plain")
	if stat.Client.Connected() {
		writer.WriteHeader(http.StatusOK);
		writer.Write([]byte("200 ok"))
	} else {
		writer.WriteHeader(http.StatusNotFound);
		writer.Write([]byte("404 not found"))
	}
}

func main() {
	var configname string
	if len(os.Args) > 1 {
		configname = os.Args[1]
	} else {
		configname = "server.json"
	}
	
	configfile, err := os.Open(configname)
	if err != nil {
		log.Fatal("Can't read configuration from server.json: ", err)
	}
	decoder := json.NewDecoder(configfile)
	config := Configuration{}
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal("Error parsing configuration: ", err)
	}
	configfile.Close()

	log.Printf("Listen = %s", config.Listen)
	log.Printf("Timeout = %d", config.Timeout)

	i := 0
	mux := http.NewServeMux()
	for _, streamdef := range config.Streams {
		log.Printf("Connecting stream %s to %s", streamdef.Serve, streamdef.Remote)
		queue := make(chan restreamer.Packet, config.InputBuffer)
		client, err := restreamer.NewClient(streamdef.Remote, queue, config.Timeout)
		if err == nil {
			mux.Handle("/check" + streamdef.Serve, &StreamStat{Client: client})
			err = client.Connect()
		}
		if err == nil {
			streamer := restreamer.NewStreamer(queue, config.MaxConnections, config.OutputBuffer)
			mux.Handle(streamdef.Serve, streamer)
			streamer.Connect()
			log.Printf("Handled connection %d", i)
			i++
		} else {
			log.Print(err)
		}
	}
	
	mux.HandleFunc("/health", HandleHealth)
	mux.HandleFunc("/stats", HandleStats)
	
	if i == 0 {
		log.Fatal("No streams available")
	} else {
		log.Print("Starting server")
		log.Fatal(http.ListenAndServe(config.Listen, mux))
	}
}
