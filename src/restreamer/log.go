/* Copyright (c) 2017 Gregor Riepl
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package restreamer

import (
	"os"
	"log"
	"time"
	"syscall"
	"os/signal"
	"encoding/json"
)

const (
	// the maximum number of unhandled control signals
	signalQueueLength int = 100
	// the maximum number of unwritten log messages
	logQueueLength int = 100
)

var (
	timeFormat string = "[" + time.RFC3339 + "] "
)

// JsonLogger is an interface for loggers that can generate JSON-formatted logs.
//
// It is recommended that logs follow some general guidelines, like adding
// a reference to the module that generated them, or a flag to differentiate
// various kinds of log messages.
//
// Examples:
// { "module": "client", "type": "connect", "stream": "http://test.url/" }
// { "module": "connection", "type": "connect", "source": "1.2.3.4:49999", "url": "/stream" }
// { "module": "connection", "type": "disconnect", "source": "1.2.3.4:49999", "url": "/stream", "duration": 61, "bytes": 12087832 }
type JsonLogger interface {
	// Log writes one or multiple data structures to the log represented by this logger.
	// Each argument is processed through json.Marshal and generates one line in the log.
	//
	// Log lines are prefixed with a time stamp in RFC3339 format, like this:
	// [2006-01-02T15:04:05Z07:00] <JSON>
	//
	// Example usage:
	//   logger.Log(map[string]string{ "key": "value" }, map[string]string{ "key": "value2" })
	Log(json ...interface{})
}

// DummyLogger is a logger placeholder that doesn't actually log anything.
type DummyLogger struct {
}

// Log does nothing.
//
// Just a placeholder for a real big boy logger.
func (*DummyLogger) Log(json ...interface{}) {
}

// ConsoleLogger is a simple logger that prints to stdout
type ConsoleLogger struct {
}

// Log writes a log line to stdout.
//
// Your best bet if you don't want/need a full-blown file logging queue with
// reopening or a central logging server.
func (*ConsoleLogger) Log(lines ...interface{}) {
	for _, line := range lines {
		data, err := json.Marshal(line)
		if err == nil {
			now := time.Now().Format(timeFormat)
			log.Printf("%s%s\n", now, data)
		} else {
			log.Printf("Cannot encode log line %s", line)
		}
	}
}

// A FileLogger allows writing JSON-formatted log lines to a file.
type FileLogger struct {
	// notification channel
	// also used for system signals
	signals chan os.Signal
	// log file name
	name string
	// log file handle
	log *os.File
	// message queue
	messages chan interface{}
	// log line counter
	lines uint64
	// dropped line counter
	drops uint64
	// error counter (encoding errors or closed log file)
	errors uint64
}

// NewLogger creates a new FileLogger and optionally installs a SIGUSR1 handler;
// pass sigusr=true for that purpose. This is useful for log rotation, etc.
//
// Signals are only fully supported on POSIX systems, so no SIGUSR1 is sent
// when running on Microsoft Windows, for example. The signal handler is
// still installed, but it is never notified.
func NewLogger(logfile string, sigusr bool) (*FileLogger, error) {
	// create logger instance
	logger := &FileLogger{
		signals: make(chan os.Signal, signalQueueLength),
		name: logfile,
		messages: make(chan interface{}, logQueueLength), 
	}
	
	// open the log for the first time
	err := logger.reopenLog()
	if err != nil {
		return nil, err
	}
	
	// install signal handler and start listening thread
	signal.Notify(logger.signals, syscall.SIGUSR1)
	go logger.handle()
	
	return logger, nil
}

func (logger *FileLogger) Log(json ...interface{}) {
	// send these down the queue
	for _, line := range json {
		select {
			case logger.messages<- line:
				// ok
			default:
				log.Printf("Log queue is full, message dropped!")
				logger.drops++
		}
	}
}

// Writes a single log line
func (logger *FileLogger) writeLog(line interface{}) {
	// only log if the output is open
	if logger.log != nil {
		data, err := json.Marshal(line)
		if err == nil {
			now := time.Now().Format(timeFormat)
			logger.log.Write([]byte(now))
			logger.log.Write(data)
			logger.log.Write([]byte("\n"))
			logger.lines++
		} else {
			log.Printf("Cannot encode log line %s", line)
			logger.errors++
		}
	} else {
		log.Printf("Output is closed, dropping line %s", line)
		logger.errors++
	}
}

// Closes the log file and disables further logging.
func (logger *FileLogger) Close() {
	log.Printf("Closing log")
	logger.signals<- syscall.SIGHUP
}

// Closes the log and stops/removes the signal handler
func (logger *FileLogger) closeLog() error {
	log.Printf("Really closing log")
	
	// uninstall the singal handler
	signal.Stop(logger.signals)
	// signal stop
	logger.signals <-os.Interrupt
	
	// close the log
	err := logger.log.Close()
	logger.log = nil
	
	return err
}

// (Re-)opens the log file.
func (logger *FileLogger) reopenLog() error {
	log.Printf("Reopening log")
	
	var err error = nil
	
	if logger.log != nil {
		// close first
		err = logger.log.Close()
		logger.log = nil
	}
	if err == nil {
		logger.log, err = os.OpenFile(logger.name, os.O_WRONLY | os.O_APPEND | os.O_CREATE, os.FileMode(0666))
	}
	
	return err
}

// Handles the log queue and the USR1 signal.
// If USR1 is received the log file is closed and reopened.
func (logger *FileLogger) handle() {
	running := true
	
	for running {
		select {
			case signal := <-logger.signals:
				// check signal type
				switch signal {
					case syscall.SIGUSR1:
						// reopen the log file
						err := logger.reopenLog()
						if err != nil {
							// if this fails, print a message to the standard log
							log.Printf("Error reopening log: %s", err)
						}
					case syscall.SIGHUP:
						// reopen the log file
						err := logger.closeLog()
						if err != nil {
							// if this fails, print a message to the standard log
							log.Printf("Error reopening log: %s", err)
						}
					case os.Interrupt:
						// shutdown requested
						running = false
						log.Printf("Shutting down logger")
				}
			case line := <-logger.messages:
				// encode and write the next line
				logger.writeLog(line)
		}
	}
}
