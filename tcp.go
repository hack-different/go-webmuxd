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
	TCPStateNew        = 0
	TCPStateConnecting = 1
	TCPStateConnected  = 2
	TCPStateClosing    = 3
	TCPStateClosed     = 4
	TCPStateRefused    = 5
)

type TCPChannelSender interface {
	sendTCPData(data []byte)
}

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
	handler           TCPChannelHandler
	sender            TCPChannelSender
	sourcePort        uint16
	destinationPort   uint16
	txSequence        uint32
	rxSequence        uint32
	txAcknowledgement uint32
	rxAcknowledgement uint32
	rxBytes           uint32
	txBytes           uint32
	window            uint32
	state             int
}

func createChannel(sourcePort uint16, destinationPort uint16, sender TCPChannelSender, handler TCPChannelHandler) *TCPChannel {
	fmt.Printf("Creating TCPChannel srcPort %d, dstPort %d\n", sourcePort, destinationPort)
	channel := &TCPChannel{
		state:             TCPStateNew,
		sender:            sender,
		sourcePort:        sourcePort,
		destinationPort:   destinationPort,
		window:            131072,
		handler:           handler,
		txSequence:        0,
		rxSequence:        0,
		txAcknowledgement: 0,
		rxAcknowledgement: 0,
		rxBytes:           0,
		txBytes:           0,
	}

	channel.sendTCP(TCPHeaderFlagSYN, []byte{})
	channel.state = TCPStateConnecting

	return channel
}

func (channel *TCPChannel) send(data []byte) {
	channel.sendTCP(TCPHeaderFlagACK, data)
}

func (channel *TCPChannel) sendTCP(flags uint16, data []byte) {
	header := &TCPHeader{
		SourcePort:      channel.sourcePort,
		DestinationPort: channel.destinationPort,
		Window:          uint16(channel.window >> 8),
		Sequence:        channel.txSequence,
		Acknowledgement: channel.txAcknowledgement,
		OffsetFlags:     flags | TCPOffset,
	}

	channel.txBytes += uint32(len(data))
	channel.txSequence += uint32(len(data))

	fmt.Printf("TCPChannel sending packet flags %x, seq %d, ack %d, length %d\n", flags, header.Sequence, header.Acknowledgement, len(data))
	if channel.state != TCPStateConnecting {

	}

	headerData, err := restruct.Pack(binary.BigEndian, header)
	if err != nil {
		fmt.Printf("RemoteDevice createChannel encode TCP error %s\n", err)
	}

	packetData := append(headerData, data...)
	fmt.Printf("RemoteDevice sending TCP packet flags %x length %d sequence %d\n", flags, len(packetData), header.Sequence)

	channel.sender.sendTCPData(packetData)
}

func (header *TCPHeader) hasFlag(flag uint16) bool {
	flags := header.OffsetFlags & 0x7F

	return (flags & flag) != 0
}

func (channel *TCPChannel) receivePacket(header *TCPHeader, data []byte) {
	fmt.Printf("TCPChannel received packet flags %x, seq %d, ack %d, length %d\n", header.OffsetFlags, header.Sequence, header.Acknowledgement, len(data))

	channel.rxSequence = header.Sequence
	channel.rxAcknowledgement = header.Acknowledgement

	if header.hasFlag(TCPHeaderFlagRST) {
		channel.state = TCPStateRefused
		channel.handler.connectionStateChange(channel.state)
		return
	}

	if header.hasFlag(TCPHeaderFlagFIN) && channel.state != TCPStateClosing {
		channel.state = TCPStateClosing
		channel.handler.connectionStateChange(channel.state)
		channel.sendTCP(TCPHeaderFlagFIN|TCPHeaderFlagACK, []byte{})
		return
	}

	if channel.state == TCPStateClosing && header.hasFlag(TCPHeaderFlagACK) {
		channel.state = TCPStateClosed
		channel.handler.connectionStateChange(channel.state)
		return
	}

	if channel.state == TCPStateConnecting &&
		header.hasFlag(TCPHeaderFlagSYN) &&
		header.hasFlag(TCPHeaderFlagACK) {

		channel.txSequence++
		channel.txAcknowledgement++
		channel.rxBytes = channel.rxSequence
		channel.state = TCPStateConnected
		channel.handler.connectionStateChange(channel.state)
		return
	}

	if channel.state == TCPStateConnected {
		if len(data) > 0 {
			channel.rxBytes += uint32(len(data))
			channel.txAcknowledgement += uint32(len(data))

			channel.sendTCP(TCPHeaderFlagACK, []byte{})

			channel.handler.receiveData(data)
		}
	}

}
