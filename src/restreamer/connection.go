package restreamer

import (
	"log"
	"time"
	"net/http"
)

// a single active connection
type Connection struct {
	// per-connection packet queue
	Queue chan Packet
	// internal communication channel
	// for signalling connection shutdown
	shutdown chan bool
	// internal flag
	// true while the connection is up
	running bool
}

// creates a new connection object
// call ServeHTTP on it to start serving
func NewConnection(qsize int) (*Connection) {
	conn := &Connection{
		Queue: make(chan Packet, qsize),
		shutdown: make(chan bool),
		running: true,
	}
	return conn
}

// close a connection
func (conn *Connection) Close() error {
	conn.shutdown<- true
	return nil
}

// serves a connection,
// continuously streaming packets from the queue
func (conn *Connection) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	log.Printf("Serving incoming connection from %s\n", request.RemoteAddr);
	
	// set the content type (important)
	writer.Header().Set("Content-Type", "video/mpeg")
	// use Add and Set to set more headers here
	// chunked mode should be on by default
	writer.WriteHeader(http.StatusOK)
	
	// start reading packets
	for conn.running {
		select {
			case packet := <-conn.Queue:
				// packet received, log
				//log.Printf("Sending packet (length %d):\n%s\n", len(packet), hex.Dump(packet))
				// send the packet out
				_, err := writer.Write(packet[:PACKET_SIZE])
				if err != nil {
					log.Printf("Connection from %s closed\n", request.RemoteAddr)
					conn.running = false
				}
				//log.Printf("Wrote packet of %d bytes\n", bytes)
			case <-time.After(1 * time.Second):
				// timeout, just cycle
			case <-conn.shutdown:
				// and shut down
				conn.running = false
		}
	}
	
	log.Printf("Stopping to serve data to %s\n", request.RemoteAddr);
}
