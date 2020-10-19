package main

const LockdownPort = 0xf27e

type LockdownQueryTypeMessage struct {
	Request string
	Label string
}

type LockdownService struct {
	propertyListService *PropertyListService
}

func (service *LockdownService) connected() {
	handshake := &LockdownQueryTypeMessage{
		Request: "QueryType",
		Label: "webserver",
	}
	service.propertyListService.sendPropertyList(handshake)
}

func (service *LockdownService) plistReceived(data map[string]interface{}){

}

func (device *RemoteDevice) createLockdownService() *LockdownService  {
	serviceDescriptor := PropertyListServiceDescriptor{
		port: LockdownPort,
		encrypted: false,
	}

	service := &LockdownService{}

	service.propertyListService = device.createService(serviceDescriptor, service)

	return service
}