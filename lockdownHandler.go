package main

const LockdownPort = 0xf27e

type LockdownQueryTypeMessage struct {
	Request string
	Label   string
}

type LockdownGetValueMessage struct {
	Label   string
	Request string
}

type LockdownService struct {
	propertyListService *PropertyListService
}

func (service *LockdownService) connected() {
	handshake := &LockdownQueryTypeMessage{
		Request: "QueryType",
		Label:   "webserver",
	}
	service.propertyListService.sendPropertyList(handshake)
}

func (service *LockdownService) plistReceived(data map[string]interface{}) {
	if data["Request"] == "QueryType" {
		getValueMessage := &LockdownGetValueMessage{
			Request: "GetValue",
			Label:   "webmuxd",
		}

		service.propertyListService.sendPropertyList(getValueMessage)
	}
}

func (device *RemoteDevice) createLockdownService() *LockdownService {
	serviceDescriptor := PropertyListServiceDescriptor{
		port:      LockdownPort,
		encrypted: false,
	}

	service := &LockdownService{}

	service.propertyListService = device.createService(serviceDescriptor, service)

	return service
}
