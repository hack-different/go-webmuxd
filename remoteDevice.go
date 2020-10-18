package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"log"
	"net/http"
	"time"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512

	messageTypeData = 1
)

type RemoteConnection struct {
	hub *Hub

	connection *websocket.Conn

	// TODO: This map concept is imperfect because the map will only grow
	devices map[*RemoteDevice]bool

	open bool

	send chan *ClientMessage

	close chan bool
}

// Client is a middleman between the websocket connection and the hub.
type RemoteDevice struct {
	// Associated hub
	hub *Hub

	// Associated Connection
	connection *RemoteConnection

	// Device serial number
	serialNumber string
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (remote *RemoteConnection) readPump() {
	if remote.connection == nil {
		return
	}

	defer remote.connection.Close()

	remote.connection.SetReadLimit(maxMessageSize)
	remote.connection.SetReadDeadline(time.Now().Add(pongWait))
	remote.connection.SetPongHandler(func(string) error {
		remote.connection.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := remote.connection.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)

			}
			break
		}

		var serverMessage = &ServerMessage{}
		err = proto.Unmarshal(message, serverMessage)
		if err != nil {
			log.Println(err)
		}

		switch serverMessage.Message.(type) {
		case *ServerMessage_DeviceConnected:
			deviceConnectedMessage := serverMessage.GetDeviceConnected()
			fmt.Printf("Device Connected %s\n", deviceConnectedMessage.SerialNumber)
			remote.hub.devices[deviceConnectedMessage.SerialNumber] = &RemoteDevice{
				hub:          remote.hub,
				connection:   remote,
				serialNumber: deviceConnectedMessage.SerialNumber,
			}

		case *ServerMessage_FromDevice:

		case *ServerMessage_ToDeviceResult:

		}
	}
}

func (remote *RemoteConnection)cleanupConnection() {
	remote.close <- true
	remote.hub.remoteConnections[remote] = false

	for device := range remote.devices {
		// TODO: Notify devices are not available
		remote.hub.devices[device.serialNumber] = nil
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (remote *RemoteConnection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer remote.connection.Close()

	for {
		select {
			case message := <- remote.send:
				writer, err := remote.connection.NextWriter(messageTypeData)
				if err != nil {
					fmt.Printf("ClientMessage NextWriter Error: %s\n", err)
				} else {
					data, err := proto.Marshal(message)
					if err != nil {
						fmt.Printf("ClientMessage Marshal Error: %s\n",err)
					} else {
						count, err := writer.Write(data)
						if err != nil {
							fmt.Printf("ClientMessage Write Error: %s", err)
						} else {
							fmt.Printf("ClientMessage Wrote %d bytes", count)
						}
					}
				}
			case <- remote.close:
				remote.open = false
				return
		}
	}
}

func (hub *Hub) makeRemoteConnection(wsConnection *websocket.Conn) *RemoteConnection {
	remoteConnection := &RemoteConnection{
		hub:        hub,
		connection: wsConnection,
		devices:    make(map[*RemoteDevice]bool),
		open: true,
		close: make(chan bool),
	}

	hub.remoteConnected <- remoteConnection

	return remoteConnection
}

// serveWs handles websocket requests from the peer.
func (hub *Hub) handleRemoteConnection(writer http.ResponseWriter, reader *http.Request) {
	fmt.Printf("New device connection: %s\n", reader.RemoteAddr)
	wsConnection, err := hub.upgrader.Upgrade(writer, reader, nil)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Printf("Upgrade success for %s\n", wsConnection.RemoteAddr())

	remoteConnection := hub.makeRemoteConnection(wsConnection)

	go remoteConnection.readPump()
	go remoteConnection.writePump()
}