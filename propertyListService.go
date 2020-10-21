package main

import (
	"encoding/binary"
	"fmt"
	"gopkg.in/restruct.v1"
	"howett.net/plist"
)

type PropertyListServiceClient interface {
	connected()
	plistReceived(data map[string]interface{})
}

type PropertyListServiceDescriptor struct {
	port      uint16
	encrypted bool
}

type PropertyListDatagram struct {
	Length uint32
	Data   []byte
}

type PropertyListService struct {
	descriptor PropertyListServiceDescriptor
	channel    *TCPChannel
	handler    PropertyListServiceClient
	sending    *PropertyListDatagram
	receiving  *PropertyListDatagram
}

func (service *PropertyListService) connectionStateChange(state int) {
	fmt.Printf("PropertyListService state change %d\n", state)
	switch state {
	case TCPStateConnected:
		service.handler.connected()
	}
}

func (service *PropertyListService) receiveData(data []byte) {
	if service.receiving == nil {
		service.receiving = &PropertyListDatagram{}

		restruct.Unpack(data, binary.BigEndian, service.receiving)
		service.receiving.Data = data[4:]
		fmt.Printf("PropertyListService beginReceive got %d bytes of %d\n", len(service.receiving.Data), service.receiving.Length)
	} else {
		service.receiving.Data = append(service.receiving.Data, data...)
		fmt.Printf("PropertyListService continueReceive got %d new bytes (at %d of %d)\n", len(data), len(service.receiving.Data), service.receiving.Length)
	}

	if service.receiving != nil && service.receiving.Length == uint32(len(service.receiving.Data)) {
		fmt.Printf("PropertyListService completeReceive with %d bytes\n", service.receiving.Length)
		result := make(map[string]interface{})

		_, err := plist.Unmarshal(service.receiving.Data, result)
		if err != nil {
			fmt.Printf("PropertyListSerivce (%d) length %d unmarshal error\n", service.descriptor.port, len(data))
		}

		service.receiving = nil

		service.handler.plistReceived(result)
	}
}

func (service *PropertyListService) sendPropertyList(data interface{}) {
	if service.channel.state != TCPStateConnected {
		fmt.Printf("Tried to send property list to service with state %d\n", service.channel.state)
		return
	}

	rawData, err := plist.Marshal(data, plist.BinaryFormat)
	if err != nil {
		fmt.Printf("PropertyListService (%d) marshal error\n", service.descriptor.port)
		return
	}

	datagram := &PropertyListDatagram{
		Length: uint32(len(rawData)),
		Data:   rawData,
	}

	datagramBytes, _ := restruct.Pack(binary.BigEndian, datagram)

	service.channel.send(datagramBytes)
}

func (device *RemoteDevice) createService(descriptor PropertyListServiceDescriptor, handler PropertyListServiceClient) *PropertyListService {
	service := &PropertyListService{
		descriptor: descriptor,
		handler:    handler,
	}

	service.channel = device.createTCPChannel(descriptor.port, service)

	return service
}
