GO_LDFLAGS = -ldflags "-s -w"

all: server

clean:
	rm -rf bin

server:
	go build -o bin/b2bua $(GO_LDFLAGS) b2bua/main.go