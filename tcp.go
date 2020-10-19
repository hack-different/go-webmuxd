package main

import (
	"encoding/binary"
	"fmt"
	"gopkg.in/restruct.v1"
)

const (
	TCPHeaderFlagFIN = 0x01
	TCPHeaderFlagSYN = 0x02
	TCPHeaderFlagRST = 0x04
	TCPHeaderFlagPSH = 0x08
	TCPHeaderFlagACK = 0x10
	TCPHeaderFlagURG = 0x20
)

const (
	TCPStateConnecting = 0
	TCPStateConnected = 1
)

type TCPChannelHandler interface {
	receiveData(data []byte)
	connectionStateChange(state int)
}

type TCPHeader struct {
	SourcePort      uint16
	DestinationPort uint16
	Sequence        uint32
	Acknowledgement uint32
	OffsetFlags     uint16
	Window          uint16
	Checksum        uint16
	Urgent          uint16
}

const TCPHeaderSize = 20
const TCPOffset = 0x05 << 12

type TCPChannel struct {
	device *RemoteDevice
	handler TCPChannelHandler
	sourcePort uint16
	destinationPort uint16
	sequence uint32
	acknowledgement uint32
	window uint32
	state int
}

func (channel *TCPChannel) sendTCP(flags uint16, data []byte) {
	tcpSYNHeader := &TCPHeader{
		SourcePort:      channel.sourcePort,
		DestinationPort: channel.destinationPort,
		Window:          uint16(channel.window >> 8),
		Sequence:        channel.sequence,
		Acknowledgement: channel.acknowledgement,
		OffsetFlags:     flags | TCPOffset,
	}
	channel.sequence++
	headerData, err := restruct.Pack(binary.BigEndian, tcpSYNHeader)
	if err != nil {
		fmt.Printf("RemoteDevice createChannel encode TCP error %s\n", err)
	}

	packetData :=  append(headerData, data...)
	fmt.Printf("RemoteDevice sending TCP packet flags %x length %d\n", flags, len(packetData))

	channel.device.sendPacket(MUXProtocolTCP, packetData)
}

func (header *TCPHeader) hasFlag(flag uint16) bool {
	flags := header.OffsetFlags & 0x7F

	return (flags & flag) != 0
}

func (channel *TCPChannel) receivePacket(header *TCPHeader, data []byte) {
	channel.acknowledgement = header.Sequence

	if channel.state == TCPStateConnecting &&
		header.hasFlag(TCPHeaderFlagSYN) &&
		header.hasFlag(TCPHeaderFlagACK) {

		channel.sendTCP(TCPHeaderFlagACK, []byte{})

		channel.state = TCPStateConnected

		channel.handler.connectionStateChange(channel.state)
	}

	channel.handler.receiveData(data)
}