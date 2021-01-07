package main

import (
	"flag"
	"log"
	"os"
)

var LogFile = flag.String("logfile", "./p2p-chat-client.log", "file path")

func initLog() {
	logFile, err := os.OpenFile(*LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	log.SetOutput(logFile)
	log.SetFlags(log.Llongfile | log.Ldate | log.Ltime)
}
