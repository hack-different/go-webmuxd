package main

import (
	"bufio"
	"net"
)

const USBMuxDHeaderSize = 16
const ReadBufferSize = 1024

type USBMuxDHeader struct {
	length  uint32
	version uint32
	message uint32
	tag     uint32
}

type DeviceAttachedProperties struct {
	ConnectionSpeed uint   `plist:"ConnectionSpeed"`
	ConnectionType  string `plist:"ConnectionType"`
	DeviceID        uint   `plist:"DeviceID"`
	LocationID      uint   `plist:"LocationID"`
	ProductID       uint   `plist:"ProductID"`
	SerialNumber    string `plist:"SerialNumber"`
}

type DeviceAttached struct {
	MessageType string                   `plist:"MessageType"`
	DeviceID    uint                     `plist:"DeviceID"`
	Properties  DeviceAttachedProperties `plist:"Properties"`
}

type LocalClient struct {
	connection *net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

func makeClient(connection *net.Conn) *LocalClient {
	client := new(LocalClient)
	client.connection = connection
	client.reader = bufio.NewReader(*connection)
	client.writer = bufio.NewWriter(*connection)

	return client
}

func (client *LocalClient) run()  {

}
