package main

import (
	"encoding/binary"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"gopkg.in/restruct.v1"
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

const (
	MUXProtocolSendMagic    = 0xfeedface
	MUXProtocolReceiveMagic = 0xfaceface

	MUXProtocolVersion = 0
	MUXProtocolControl = 1
	MUXProtocolSetup   = 2
	MUXProtocolTCP     = 6

	MUXProtocolResultError   = 0x03
	MUXProtocolResultWarning = 0x05
	MUXProtocolResultInfo    = 0x07
)

type MUXHeader struct {
	Protocol         uint32
	Length           uint32
	Magic            uint32
	TransmitSequence uint16
	ReceiveSequence  uint16
}

const (
	MUXMajorVersion = 2
	MUXMinorVersion = 8
)

type MUXVersion struct {
	Major   uint32
	Minor   uint32
	Padding uint32
}

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

	versionHeader *MUXVersion

	connectedMessage *DeviceConnected

	channels map[uint16]*TCPChannel

	transmitSequence uint16
	receiveSequence  uint16
	sourcePort       uint16
	LockdownService  *LockdownService
}

func (device *RemoteDevice) sendPacket(packetProtocol int, data []byte) {
	if packetProtocol == MUXProtocolSetup {
		device.receiveSequence = 0xFFFF
		device.transmitSequence = 0x0000
	}

	muxHeader := &MUXHeader{
		Protocol:         uint32(packetProtocol),
		Length:           0,
		Magic:            MUXProtocolSendMagic,
		TransmitSequence: device.transmitSequence,
		ReceiveSequence:  device.receiveSequence,
	}
	device.transmitSequence++

	headerSize, err := restruct.SizeOf(muxHeader)
	if err != nil {
		fmt.Printf("RemoteDevice sizing muxHeader error %s\n", err)
	}
	muxHeader.Length = uint32(headerSize + len(data))

	headerData, err := restruct.Pack(binary.BigEndian, muxHeader)
	fmt.Printf("RemoteDevice sending %d packet tx %d rx %d of length %d\n", muxHeader.Protocol, muxHeader.TransmitSequence, muxHeader.ReceiveSequence, muxHeader.Length)
	if err != nil {
		fmt.Printf("RemoteDevice packing muxHeader error %s\n", err)
	}

	correlationId := uuid.New().String()

	clientMessage := &ClientMessage{
		Message: &ClientMessage_ToDevice{
			ToDevice: &DataToDevice{
				SerialNumber:  device.serialNumber,
				CorrelationId: correlationId,
				Data:          append(headerData, data...),
			},
		},
	}

	messageData, err := proto.Marshal(clientMessage)
	if err != nil {
		fmt.Printf("RemoteDevice error marshalling client message %s\n", err)
	}

	err = device.connection.connection.WriteMessage(websocket.BinaryMessage, messageData)
	if err != nil {
		fmt.Printf("RemoteDevice WriteMessage error %s\n", err)
	}
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
			device := &RemoteDevice{
				sourcePort:       1024,
				hub:              remote.hub,
				connection:       remote,
				serialNumber:     deviceConnectedMessage.SerialNumber,
				connectedMessage: deviceConnectedMessage,
				channels:         make(map[uint16]*TCPChannel),
			}

			remote.hub.devices[deviceConnectedMessage.SerialNumber] = device
			device.sendVersion()

		case *ServerMessage_FromDevice:
			fromDeviceMessage := serverMessage.GetFromDevice()
			fmt.Printf("Got %d bytes of data from device %s\n", len(fromDeviceMessage.Data), fromDeviceMessage.SerialNumber)
			remote.hub.devices[fromDeviceMessage.SerialNumber].receiveData(fromDeviceMessage.Data)

		case *ServerMessage_ToDeviceResult:
			toDeviceResult := serverMessage.GetToDeviceResult()
			fmt.Printf("Result of ToDevice transfer %s is %t\n", toDeviceResult.CorrelationId, toDeviceResult.Success)
		}
	}
}

func (device *RemoteDevice) receiveData(data []byte) {
	if len(data) < USBMuxDHeaderSize {
		fmt.Printf("Insufficant data for MUX header (got %d bytes)\n", len(data))
		return
	}

	muxHeader := &MUXHeader{}
	err := restruct.Unpack(data[:USBMuxDHeaderSize], binary.BigEndian, muxHeader)
	if err != nil {
		fmt.Printf("RemoteDevice mux header decoding error %s\n", err)
		return
	}

	fmt.Printf("Got MUX Packet from device (type %d, length %d, tx %d, rx %d)\n",
		muxHeader.Protocol, muxHeader.Length, muxHeader.TransmitSequence, muxHeader.ReceiveSequence)

	if muxHeader.Protocol != MUXProtocolVersion && muxHeader.Magic != MUXProtocolReceiveMagic {
		fmt.Printf("RemoteDevice mux header (%d) magic error %x != %x\n", muxHeader.Protocol, muxHeader.Magic, MUXProtocolReceiveMagic)
		return
	}

	if muxHeader.Protocol != MUXProtocolVersion {
		device.receiveSequence = muxHeader.ReceiveSequence
	}

	switch muxHeader.Protocol {
	case MUXProtocolVersion:
		if device.versionHeader == nil {
			versionHeader := MUXVersion{}
			err = restruct.Unpack(data[8:20], binary.BigEndian, versionHeader)
			if err != nil {
				fmt.Printf("RemoteDevice mux header decoding error %s\n", err)
				return
			}
			device.versionHeader = &versionHeader
			device.sendPacket(MUXProtocolSetup, []byte{0x05})

			device.LockdownService = device.createLockdownService()
		}
	case MUXProtocolControl:
		controlData := data[USBMuxDHeaderSize+1 : muxHeader.Length]
		controlType := data[USBMuxDHeaderSize]
		switch controlType {
		case MUXProtocolResultError:
			fmt.Printf("Control Frame Error: %s\n", controlData)
		case MUXProtocolResultWarning:
			fmt.Printf("Control Frame Warn: %s\n", controlData)
		case MUXProtocolResultInfo:
			fmt.Printf("Control Frame Info: %s\n", controlData)
		default:
			fmt.Printf("Unknown Control Frame %d: %s\n", controlType, controlData)
		}
	case MUXProtocolTCP:
		tcpHeader := &TCPHeader{}
		tcpHeaderData := data[USBMuxDHeaderSize : USBMuxDHeaderSize+TCPHeaderSize]
		err = restruct.Unpack(tcpHeaderData, binary.BigEndian, tcpHeader)
		if err != nil {
			fmt.Printf("RemoteDevice mux header decoding error %s\n", err)
			return
		}

		channel := device.channels[tcpHeader.SourcePort]
		if channel == nil {
			fmt.Printf("Could not find an active channle for src %d and dst %d\n", tcpHeader.SourcePort, tcpHeader.DestinationPort)
		} else {
			channel.receivePacket(tcpHeader, data[USBMuxDHeaderSize+TCPHeaderSize:muxHeader.Length])
		}

	default:
		fmt.Printf("RemoteDevice unknown (%d) with length %d and magic %x\n", muxHeader.Protocol, muxHeader.Length, muxHeader.Magic)
	}

	remainingBytes := data[muxHeader.Length:]
	if len(remainingBytes) > 0 {
		device.receiveData(remainingBytes)
	}
}

func (device *RemoteDevice) sendVersion() {
	versionHeader := &MUXVersion{
		Major:   MUXMajorVersion,
		Minor:   MUXMinorVersion,
		Padding: 0,
	}

	bytes, err := restruct.Pack(binary.BigEndian, versionHeader)
	if err != nil {
		fmt.Printf("RemoteDevice packing version error %s\n", err)
	}
	device.sendPacket(MUXProtocolVersion, bytes)
}

func (remote *RemoteConnection) cleanupConnection() {
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
		case message := <-remote.send:
			writer, err := remote.connection.NextWriter(messageTypeData)
			if err != nil {
				fmt.Printf("ClientMessage NextWriter Error: %s\n", err)
			} else {
				data, err := proto.Marshal(message)
				if err != nil {
					fmt.Printf("ClientMessage Marshal Error: %s\n", err)
				} else {
					count, err := writer.Write(data)
					if err != nil {
						fmt.Printf("ClientMessage Write Error: %s", err)
					} else {
						fmt.Printf("ClientMessage Wrote %d bytes", count)
					}
				}
			}
		case <-remote.close:
			remote.open = false
			return
		}
	}
}

func (device *RemoteDevice) createTCPChannel(port uint16, handler TCPChannelHandler) *TCPChannel {
	channel := createChannel(device.sourcePort, port, device, handler)
	device.sourcePort++
	device.channels[port] = channel

	return channel
}

func (device *RemoteDevice) sendTCPData(data []byte) {
	device.sendPacket(MUXProtocolTCP, data)
}

func (hub *Hub) makeRemoteConnection(wsConnection *websocket.Conn) *RemoteConnection {
	remoteConnection := &RemoteConnection{
		hub:        hub,
		connection: wsConnection,
		devices:    make(map[*RemoteDevice]bool),
		open:       true,
		close:      make(chan bool),
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
