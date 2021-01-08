package main

import (
	"bufio"
	"fmt"
	"net"
	"os"

	"github.com/libp2p/go-reuseport"
)

var id int

func sendCmd(conn net.Conn, cmd string) {
	conn.Write([]byte(cmd))
	fmt.Printf("<%s>\n", conn.RemoteAddr())
}

func recvServer(conn net.Conn) {
	b := make([]byte, 1024)
	for {
		n, err := conn.Read(b)
		if err != nil {
			fmt.Printf("read err: %+v", err)
		}
		res := b[:n]
		fmt.Printf("recv: %s\n", res)
		fmt.Println()
	}
}

func listenSamePort() {
	ln, err := reuseport.ListenPacket("udp", "127.0.0.1:10001")
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	udpaddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:10086")
	if err != nil {
		panic(err)
	}
	_, err = ln.WriteTo([]byte("aaa"), udpaddr)
	if err != nil {
		panic(err)
	}

	b := make([]byte, 1024)
	for {
		n, addr, err := ln.ReadFrom(b)
		if err != nil {
			panic(err)
		}
		fmt.Printf("=== [%s] %d: %s\n", addr, n, b[:n])
	}
}

func main() {
	//ip := net.ParseIP("127.0.0.1")
	//srcAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001}
	//dstAddr := &net.UDPAddr{IP: ip, Port: 10086}
	//conn, err := net.DialUDP("udp", srcAddr, dstAddr)
	//conn, err := reuseport.Dial("udp", "127.0.0.1:10001", "127.0.0.1:10086")
	//if err != nil {
	//	fmt.Println(err)
	//}
	//defer conn.Close()

	//go recvServer(conn)
	go listenSamePort()

	//sendCmd(conn, "login tom")
	//sendCmd(conn, "get 1")
	//sendCmd(conn, "get 2")
	//sendCmd(conn, "punch 1 4")
	//sendCmd(conn, "logout 1")

	a := bufio.NewScanner(os.Stdin)
	a.Scan()
	fmt.Printf(a.Text())
}
