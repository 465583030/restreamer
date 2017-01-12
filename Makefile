export GOPATH=$(shell pwd)

all: bin/server bin/source

clean:
	rm -f bin/* pkg/*

bin/server: src/main.go pkg/librestreamer.a
	go build -o $@ src/main.go

bin/source: src/source.go
	go build -o $@ $^

pkg/librestreamer.a: src/restreamer/api.go src/restreamer/stats.go src/restreamer/connection.go src/restreamer/packet.go src/restreamer/client.go src/restreamer/streamer.go
	go build -o $@ $^
