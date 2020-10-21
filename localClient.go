package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"gopkg.in/restruct.v1"
	"howett.net/plist"
	"io"
	"net"
)

const ReadBufferSize = 1024
const USBMuxDVersionMaxSupported = 1
const USBMuxDHeaderSize = 16
const USBMuxDResultSize = USBMuxDHeaderSize + 4

const MessageTypeListDevices = "ListDevices"
const MessageTypeListListen = "Listen"
const MessageTypeListResult = "Result"
const MessageTypeConnect = "Connect"
const MessageTypeDeviceAttached = "Attached"

const ConnectionSpeedUSB2 = 480000000

const (
	USBMuxDResultOK                = 0
	USBMuxDResultBadCommand        = 1
	USBMuxDResultBadDevice         = 2
	USBMuxDResultConnectionRefused = 3
	USBMuxDResultBadVersion        = 6
)

const (
	USBMuxDMessageResult       = 1
	USBMuxDMessageConnect      = 2
	USBMuxDMessageListen       = 3
	USBMuxDMessageDeviceAdd    = 4
	USBMuxDMessageDeviceRemove = 5
	USBMuxDMessageDevicePaired = 6
	USBMuxDMessagePlist        = 8
)

type LocalClientTCPHandler struct {
	deviceId      uint32
	port          uint16
	localClient   *LocalClient
	connectHeader *USBMuxDHeader
	device        *RemoteDevice
	channel       *TCPChannel
}

type USBMuxDHeader struct {
	Length  uint32 `struct:"uint32"`
	Version uint32 `struct:"uint32"`
	Message uint32 `struct:"uint32"`
	Tag     uint32 `struct:"uint32"`
}

type USBMuxDResult struct {
	Header USBMuxDHeader
	Result uint32 `struct:"uint32"`
}

type USBMuxDConnect struct {
	Header   USBMuxDHeader
	DeviceId uint32 `struct:"uint32"`
	Port     uint16 `struct:"uint16"`
	Reserved uint16 `struct:"uint16"`
}

type USBMuxDDevice struct {
	DeviceId     uint32
	ProductId    uint16
	SerialNumber [256]byte
	Padding      uint16
	Location     uint32
}

type USBMuxDPlistMessage struct {
	ClientVersionString string
	MessageType         string
	ProgName            string
	kLibUSBMuxVersion   uint64
}

type ResultMessage struct {
	MessageType string
	Number      uint64
}

type ListDevicesMessage struct {
	DeviceList []DeviceAttached
}

type DeviceAttachedProperties struct {
	ConnectionSpeed uint32 `plist:"ConnectionSpeed"`
	ConnectionType  string `plist:"ConnectionType"`
	DeviceID        uint32 `plist:"DeviceID"`
	LocationID      uint32 `plist:"LocationID"`
	ProductID       uint32 `plist:"ProductID"`
	SerialNumber    string `plist:"SerialNumber"`
	NetworkAddress  []byte `plist:"NetworkAddress"`
}

type DeviceAttached struct {
	MessageType string                   `plist:"MessageType"`
	DeviceID    uint32                   `plist:"DeviceID"`
	Properties  DeviceAttachedProperties `plist:"Properties"`
}

type LocalClient struct {
	open        bool
	listening   bool
	deviceIndex uint32
	deviceMap   map[uint32]*RemoteDevice
	hub         *Hub
	connection  *net.Conn
	reader      *bufio.Reader
	writer      *bufio.Writer

	deviceAttached chan *RemoteDevice
	deviceRemoved  chan *RemoteDevice

	channels []*LocalClientTCPHandler

	close chan bool
}

func makeClient(hub *Hub, connection *net.Conn) *LocalClient {
	return &LocalClient{
		hub:            hub,
		open:           true,
		deviceIndex:    1,
		connection:     connection,
		reader:         bufio.NewReader(*connection),
		writer:         bufio.NewWriter(*connection),
		deviceAttached: make(chan *RemoteDevice),
		deviceRemoved:  make(chan *RemoteDevice),
		channels:       make([]*LocalClientTCPHandler, 0),
		close:          make(chan bool),
	}
}

func (client *LocalClient) run() {
	go client.readPump()
}

func (client *LocalClient) readPump() {
	header := USBMuxDHeader{}
	headerData := make([]byte, USBMuxDHeaderSize)
	for {
		if client.open == false {
			fmt.Printf("LocalClient connection broken\n")
			return
		}

		count, err := io.ReadFull(client.reader, headerData)
		if err == io.EOF {
			client.open = false
			client.close <- true
			break
		}
		if err != nil || count != USBMuxDHeaderSize {
			fmt.Printf("LocalClient torn USBMuxD header (got %d bytes) - %s\n", count, err)
		}

		if err = restruct.Unpack(headerData, binary.LittleEndian, &header); err != nil {
			fmt.Printf("LocalClient header unpacking error: %s\n", err)
		}

		if header.Version > USBMuxDVersionMaxSupported {
			fmt.Printf("LocalClient got unsupported version %d\n", header.Version)
		}

		fmt.Printf("LocalClient message %d with tag %d and length of %d\n", header.Message, header.Tag, header.Length)

		switch header.Message {
		case USBMuxDMessageListen:
			client.listening = true
			listenResult := USBMuxDResult{
				Header: USBMuxDHeader{
					Length:  USBMuxDResultSize,
					Version: header.Version,
					Tag:     header.Tag,
					Message: USBMuxDMessageResult,
				},
				Result: USBMuxDResultOK,
			}
			responseBytes, _ := restruct.Pack(binary.LittleEndian, listenResult)
			_, err = client.writer.Write(responseBytes)

		case USBMuxDMessagePlist:
			remainingBytes := header.Length - USBMuxDHeaderSize
			plistBytes := make([]byte, remainingBytes)
			count, err = io.ReadFull(client.reader, plistBytes)
			if err == io.EOF {
				client.open = false
				client.close <- true
				break
			}
			if err != nil || uint32(count) != remainingBytes {
				fmt.Printf("LocalClient torn plist (got %d bytes) - %s\n", count, err)
			}

			plistDictionary := make(map[string]interface{})
			_, err = plist.Unmarshal(plistBytes, &plistDictionary)
			if err != nil {
				fmt.Printf("LocalClient plist unmarshalling error %s\n", err)
			}

			client.handlePlistMessage(header, plistDictionary)
		}
	}
}

func (client *LocalClient) sendResponse(header USBMuxDHeader, result int) {
	responseMessage := &USBMuxDResult{
		Header: USBMuxDHeader{
			Version: header.Version,
			Tag:     header.Tag,
			Message: USBMuxDMessageResult,
			Length:  USBMuxDResultSize,
		},
		Result: uint32(result),
	}

	headerBytes, err := restruct.Pack(binary.LittleEndian, responseMessage)
	if err != nil {
		fmt.Printf("LocalClient header marshal error %s\n", err)
		return
	}

	_, err = client.writer.Write(headerBytes)
	if err != nil {
		fmt.Printf("LocalClient send error %s\n", err)
		return
	}
	err = client.writer.Flush()
	if err != nil {
		fmt.Printf("LocalClient flush error %s\n", err)
		return
	}
	fmt.Printf("LocalClient response %d to tag %d\n", result, header.Tag)
}

func (handler *LocalClientTCPHandler) connectionStateChange(state int) {
	fmt.Printf("LocalClientTCPHandler connectionStateChanged %d\n", state)
	switch state {
	case TCPStateConnected:
		result := &ResultMessage{
			MessageType: MessageTypeListResult,
			Number:      USBMuxDResultOK,
		}
		handler.localClient.sendPlistResponse(*handler.connectHeader, result)
	}
}

func (handler *LocalClientTCPHandler) receiveData(data []byte) {

}

func (client *LocalClient) handlePlistMessage(header USBMuxDHeader, dictionary map[string]interface{}) {
	client.ensureDeviceMap()

	fmt.Printf("LocalClient handlePlistMessage Tag %d, Type: %s\n", header.Tag, dictionary["MessageType"])
	switch dictionary["MessageType"] {

	case MessageTypeListListen:
		client.listening = true
		client.sendResponse(header, USBMuxDResultOK)
	case MessageTypeConnect:
		deviceId := dictionary["DeviceID"].(uint64)
		port := dictionary["PortNumber"].(uint64)

		device := client.deviceMap[uint32(deviceId)]

		handler := &LocalClientTCPHandler{
			deviceId:      uint32(deviceId),
			port:          uint16(port),
			localClient:   client,
			connectHeader: &header,
			device:        device,
		}

		handler.channel = device.createTCPChannel(uint16(port), handler)

	case MessageTypeListDevices:
		deviceList := &ListDevicesMessage{
			DeviceList: make([]DeviceAttached, len(client.deviceMap)),
		}

		for index, device := range client.deviceMap {
			deviceAttach := &DeviceAttached{
				MessageType: MessageTypeDeviceAttached,
				DeviceID:    index,
				Properties: DeviceAttachedProperties{
					// TODO: Add these additional properties to `transport.proto` device connected
					ConnectionSpeed: ConnectionSpeedUSB2,
					ConnectionType:  "USB",
					NetworkAddress:  nil,
					DeviceID:        index,
					LocationID:      index,
					ProductID:       uint32(device.connectedMessage.ProductId),
					SerialNumber:    device.serialNumber,
				},
			}
			deviceList.DeviceList[(index - 1)] = *deviceAttach
		}

		client.sendPlistResponse(header, deviceList)
	}
}

func (client *LocalClient) ensureDeviceMap() {
	if client.deviceMap == nil {
		client.deviceMap = make(map[uint32]*RemoteDevice)

		currentDevices := client.hub.devices
		for _, device := range currentDevices {
			if device != nil {
				client.deviceMap[client.deviceIndex] = device
				client.deviceIndex++
			}
		}
	}
}

func (client *LocalClient) sendPlistResponse(header USBMuxDHeader, message interface{}) {
	plistData, err := plist.Marshal(message, plist.XMLFormat)
	if err != nil {
		fmt.Printf("LocalClient sendPlistResponse marshal error %s\n", err)
		return
	}

	responseMessage := &USBMuxDHeader{
		Message: USBMuxDMessagePlist,
		Tag:     header.Tag,
		Length:  uint32(USBMuxDHeaderSize + len(plistData)),
		Version: header.Version,
	}

	headerBytes, err := restruct.Pack(binary.LittleEndian, responseMessage)
	if err != nil {
		fmt.Printf("LocalClient header marshal error %s\n", err)
		return
	}

	count, err := client.writer.Write(append(headerBytes, plistData...))
	if err != nil {
		fmt.Printf("LocalClient send error %s\n", err)
		return
	}
	err = client.writer.Flush()
	if err != nil {
		fmt.Printf("LocalClient flush error %s\n", err)
		return
	}
	fmt.Printf("LocalClient sent %d bytes in plist response to tag %d\n", count, header.Tag)
}
