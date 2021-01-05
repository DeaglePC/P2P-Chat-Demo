package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/libp2p/go-reuseport"
	"net"
	"os"
	"strings"
)

const (
	OK   = "OK"
	Fail = "FAIL"
)

var (
	LocalAddr  = flag.String("laddr", "127.0.0.1:10001", "local addr: ip:port")
	ServerAddr = flag.String("raddr", "127.0.0.1:10086", "server addr: ip:port")
	Name       = flag.String("name", "tom", "your name")
	PeerID     = flag.Int("pid", 1, "target client ID")
)

type ChatClient struct {
	LocalAddr  string
	ServerAddr string
	PeerID     int

	id         int
	serverConn net.Conn       // 跟p2p服务器的链接
	peerConn   net.PacketConn // 跟对方客户端的链接
	peerAddr   string         // 对方客户端的地址：ip:port
}

func (c *ChatClient) dial() (err error) {
	c.serverConn, err = reuseport.Dial("udp", c.LocalAddr, c.ServerAddr)
	return
}

func (c *ChatClient) sendCmdToServer(cmd string) error {
	b := []byte(cmd)
	n, err := c.serverConn.Write(b)
	if err != nil || n != len(b) {
		return err
	}
	return nil
}

func (c *ChatClient) readFromServer() ([]byte, error) {
	b := make([]byte, 1024)
	n, err := c.serverConn.Read(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}

// parseResp 把结果和数据/提示信息分开，rawData: "OK data" / "FAIL msg"
// return: result, data/msg
func parseResp(rawData []byte) (bool, string) {
	if len(rawData) == 0 {
		return false, ""
	}

	resp := string(rawData)
	if strings.HasPrefix(resp, Fail) {
		offset := len(Fail)
		if len(resp) > offset {
			offset++ // 去掉空格
		}
		return false, resp[offset+1:]
	} else if strings.HasPrefix(resp, OK) {
		offset := len(OK)
		if len(resp) > offset {
			offset++ // 去掉空格
		}
		return true, resp[offset:]
	}

	return false, "unknown response result"
}

func (c *ChatClient) login() error {
	return c.sendCmdToServer("login " + *Name)
}

func (c *ChatClient) logout() error {
	return c.sendCmdToServer(fmt.Sprintf("logout %d", c.id))
}

func (c *ChatClient) getPeerClientAddr() error {
	return c.sendCmdToServer(fmt.Sprintf("get %d", c.PeerID))
}

func (c *ChatClient) recvServerLoop() {
	for {
		data, err := c.readFromServer()
		if err != nil {
			fmt.Printf("readFromServer err: %+v\n", err)
		}
		fmt.Printf("recv from server: %s\n", data)
	}
}

func (c *ChatClient) Run() (err error) {
	if err = c.dial(); err != nil {
		return
	}
	go c.recvServerLoop()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		scanner.Scan()
		//cmd := scanner.Text()
		//switch scanner.Text() {
		//case :
		//
		//}
	}
}

func main() {
	c := ChatClient{
		LocalAddr:  *LocalAddr,
		ServerAddr: *ServerAddr,
		PeerID:     *PeerID,
	}
	if err := c.Run(); err != nil {
		panic(err)
	}
}
