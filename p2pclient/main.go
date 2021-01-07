package main

import "flag"

func init() {
	flag.Parse()
	initLog()
}

func main() {
	RunP2PChatClient()
	displayPeerMsg()
	runUI()
}
