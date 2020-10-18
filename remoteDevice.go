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
)


var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client is a middleman between the websocket connection and the hub.
type RemoteDevice struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (remoteDevice *RemoteDevice) readPump() {
	defer func() { remoteDevice.conn.Close() }()

	remoteDevice.conn.SetReadLimit(maxMessageSize)
	remoteDevice.conn.SetReadDeadline(time.Now().Add(pongWait))
	remoteDevice.conn.SetPongHandler(func(string) error { remoteDevice.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := remoteDevice.conn.ReadMessage()
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
		case *ServerMessage_FromDeviceMessage:

		case *ServerMessage_ToDeviceResult:

		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (remoteDevice *RemoteDevice) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer remoteDevice.conn.Close()

	for {
		select {

		}
	}
}

// serveWs handles websocket requests from the peer.
func handleDevice(hub *Hub, writer http.ResponseWriter, reader *http.Request) {
	fmt.Printf("New device connection: %s\n", reader.RemoteAddr)
	conn, err := hub.upgrader.Upgrade(writer, reader, nil)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Printf("Upgrade success for %s\n", conn.RemoteAddr())
	client := &RemoteDevice{hub: hub, conn: conn, send: make(chan []byte, 256)}


	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}

