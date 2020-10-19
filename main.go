package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
)

var socketFile = flag.String("socket", "/tmp/remote_usbmuxd.sock", "local unix socket")
var addressFlag = flag.String("listen", "127.0.0.1", "remote service address")
var portFlag = flag.Int("port", 8080, "remote service port")

func main() {
	flag.Parse()

	if err := os.RemoveAll(*socketFile); err != nil {
		log.Fatal(err)
	}

	localSocket, err := net.Listen("unix", *socketFile)
	if err != nil {
		log.Fatal("listen error:", err)
	}
	defer localSocket.Close()
	fmt.Printf("Local socket opened at %s\n", *socketFile)

	hub := newHub(&localSocket)

	go hub.runLocalConnections()

	go hub.run()

	http.HandleFunc("/v1/device", func(writer http.ResponseWriter, reader *http.Request) {
		hub.handleRemoteConnection(writer, reader)
	})

	listenBind := fmt.Sprintf("%s:%d", *addressFlag, *portFlag)
	fmt.Printf("Serving remote socket opened at %s\n", listenBind)
	err = http.ListenAndServe(listenBind, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
