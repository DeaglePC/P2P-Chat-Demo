package main

import (
	"log"
	"os"
)

func init() {
	logFile, err := os.OpenFile("./p2p-chat-client.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	log.SetOutput(logFile)
	log.SetFlags(log.Llongfile | log.Ldate | log.Ltime)
}
