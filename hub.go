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
	// Local client listening socket
	localSocket *net.Listener

	// Remote connection WS upgrader
	upgrader *websocket.Upgrader

	// Remote connections (Set)
	remoteConnections map[*RemoteConnection]bool

	// Registered devices.
	// TODO: this is insecure because the key is the serial number from the client
	devices map[string]*RemoteDevice

	// Local clients
	clients map[*net.Conn]*LocalClient

	// When devices are attached they send to this channel to notify waiting local clients
	deviceAttached chan *RemoteDevice

	deviceRemoved chan *RemoteDevice

	remoteConnected chan *RemoteConnection

	remoteDisconnected chan *RemoteConnection

	localConnected chan *LocalClient

	localDisconnected chan *LocalClient

	close chan bool

	open bool
}

func newHub(localSocket *net.Listener) *Hub {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(request *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	return &Hub{
		localSocket:        localSocket,
		devices:            make(map[string]*RemoteDevice),
		remoteConnections:  make(map[*RemoteConnection]bool),
		upgrader:           &upgrader,
		clients:            make(map[*net.Conn]*LocalClient),
		deviceAttached:     make(chan *RemoteDevice),
		deviceRemoved:      make(chan *RemoteDevice),
		remoteConnected:    make(chan *RemoteConnection),
		remoteDisconnected: make(chan *RemoteConnection),
		localConnected:     make(chan *LocalClient),
		localDisconnected:  make(chan *LocalClient),
		close:              make(chan bool),
		open:               true,
	}
}

func (hub *Hub) run() {
	for {
		select {
		case device := <-hub.deviceAttached:
			hub.devices[device.serialNumber] = device
			for _, localClient := range hub.clients {
				localClient.deviceAttached <- device
			}
		case device := <-hub.deviceRemoved:
			hub.devices[device.serialNumber] = nil
			for _, localClient := range hub.clients {
				localClient.deviceRemoved <- device
			}
		case remote := <-hub.remoteConnected:
			hub.remoteConnections[remote] = true
		case remote := <-hub.remoteDisconnected:
			hub.remoteConnections[remote] = false
		case local := <-hub.localConnected:
			hub.clients[local.connection] = local
		case local := <-hub.localDisconnected:
			hub.clients[local.connection] = nil
		case <-hub.close:
			hub.open = false
		}
	}
}

func (hub *Hub) runLocalConnections() {
	listener := *(hub.localSocket)

	for {
		localConnection, err := listener.Accept()

		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Printf("Local connection accepted %s\n", localConnection.RemoteAddr())

			client := makeClient(hub, &localConnection)

			hub.localConnected <- client

			go client.run()
		}
	}
}
