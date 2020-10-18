package main

import (
	"fmt"
	"net"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Local listening socket
	localSocket *net.Listener

	// Registered clients.
	devices map[*RemoteDevice]bool
}

func newHub(localSocket *net.Listener) *Hub {
	return &Hub{
		localSocket: localSocket,
		devices:    make(map[*RemoteDevice]bool),
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

	}
}