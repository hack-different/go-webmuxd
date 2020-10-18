package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Local listening socket
	localSocket *net.Listener

	// Registered clients.
	devices map[*RemoteDevice]bool

	upgrader *websocket.Upgrader
}

func newHub(localSocket *net.Listener) *Hub {
	upgrader := websocket.Upgrader{ CheckOrigin: func(request *http.Request) bool {
		return true
	}}

	return &Hub{
		localSocket: localSocket,
		devices:    make(map[*RemoteDevice]bool),
		upgrader: &upgrader,
	}
}

func (hub *Hub) runDeviceConnections() {
	for {
		select {

		}

	}
}

func (hub *Hub) runLocalConnections() {
	listener := *(hub.localSocket)

	for {
		localClient, err := listener.Accept()

		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Printf("Local connection accepted %s\n", localClient.RemoteAddr())
			device := &RemoteDevice{}
			hub.devices[device] = true
		}

		client := &LocalClient{ connection: &localClient }
		go client.run()
	}
}