package main

import (
	"fmt"
	"howett.net/plist"
)

type PropertyListServiceClient interface {
	connected()
	plistReceived(data map[string]interface{})
}

type PropertyListServiceDescriptor struct {
	port uint16
	encrypted bool
}

type PropertyListService struct {
	descriptor PropertyListServiceDescriptor
	channel *TCPChannel
	handler PropertyListServiceClient
}
func (service *PropertyListService) connectionStateChange(state int) {
	switch state {
	case TCPStateConnected:
		service.handler.connected()
	}
}

func (service *PropertyListService) receiveData(data []byte) {
	result := make(map[string]interface{})

	_, err := plist.Unmarshal(data, result)
	if err != nil {
		fmt.Printf("PropertyListSerivce (%d) length %d unmarshal error\n", service.descriptor.port, len(data))
	}

	service.handler.plistReceived(result)
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

	service.channel.send(rawData)
}

func (device *RemoteDevice) createService(descriptor PropertyListServiceDescriptor, handler PropertyListServiceClient) *PropertyListService {
	service := &PropertyListService{
		descriptor: descriptor,
		handler: handler,
	}

	service.channel = device.createChannel(descriptor.port, service)

	return service
}