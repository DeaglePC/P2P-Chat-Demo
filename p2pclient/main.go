package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/libp2p/go-reuseport"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	OK   = "OK"
	Fail = "FAIL"

	PunchMsg = "#hello#"
)

var (
	LocalAddr  = flag.String("laddr", "127.0.0.1:10001", "local addr: ip:port")
	ServerAddr = flag.String("raddr", "127.0.0.1:10086", "server addr: ip:port")
)

func init() {
	flag.Parse()
}

type ServerResponse struct {
	Cmd    string // login/logout/get/punch
	Result bool   // OK->true/FAIL->false
	Data   string
}

type ChatClient struct {
	LocalAddr  string
	ServerAddr string

	id         int
	name       string
	serverConn net.Conn       // 跟p2p服务器的链接
	peerConn   net.PacketConn // 跟对方客户端的链接
	peerAddr   *net.UDPAddr   // 对方客户端的地址：ip:port

	serverRecvChan chan *ServerResponse
	punchChan      chan *net.UDPAddr // addr
}

func (c *ChatClient) Destroy() {
	close(c.punchChan)
	c.peerConn.Close()
	c.serverConn.Close()
}

func (c *ChatClient) init() error {
	c.serverRecvChan = make(chan *ServerResponse)
	c.punchChan = make(chan *net.UDPAddr)
	return c.initConn()
}

func (c *ChatClient) initConn() error {
	if err := c.dial(); err != nil {
		return err
	}
	return c.listenPeer()
}

func (c *ChatClient) recvPeerMsgLoop() {
	b := make([]byte, 1024)

	for {
		n, addr, err := c.peerConn.ReadFrom(b)
		if err != nil {
			fmt.Printf("read from peer error: %+v\n", err)
			break
		}
		fmt.Printf("[%s] %s\n", addr, b[:n])
	}
}

func (c *ChatClient) sendToPeer(addr net.Addr, msg string) error {
	b := []byte(msg)
	n, err := c.peerConn.WriteTo(b, addr)
	if err != nil || n != len(b) {
		return err
	}
	return nil
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

func parseServerResponse(b []byte) (*ServerResponse, error) {
	resp := string(b)
	segs := strings.SplitN(resp, " ", 3)
	if len(segs) < 2 {
		return nil, fmt.Errorf("bad server response")
	}
	offset := len(segs[0]) + len(segs[1]) + 1
	if len(resp) > offset {
		offset++
	}
	return &ServerResponse{
		Cmd:    segs[0],
		Result: segs[1] == OK,
		Data:   resp[offset:],
	}, nil
}

// tryParsePunchMsg 尝试解析打洞消息，返回值： 是否打洞消息，打洞地址
func tryParsePunchMsg(b []byte) (bool, string) {
	resp := string(b)
	segs := strings.Split(resp, " ")
	if len(segs) != 2 {
		return false, ""
	}
	if segs[0] == "getpunch" {
		return true, segs[1]
	}
	return false, ""
}

// 客户端之间的连接
func (c *ChatClient) listenPeer() (err error) {
	c.peerConn, err = reuseport.ListenPacket("udp", c.LocalAddr)
	return
}

// recvPunchLoop 接收来自p2p server的打洞请求
func (c *ChatClient) recvPunchLoop() {
	for addr := range c.punchChan {
		fmt.Printf("do punch addr: %s\n", addr)
		if err := c.sendToPeer(addr, PunchMsg); err != nil {
			fmt.Printf("send to peer error: %+v\n", err)
		}
		fmt.Printf("send punch hello to %s OK\n", addr)
	}
}

func (c *ChatClient) recvServerLoop() {
	defer close(c.serverRecvChan)
	defer close(c.punchChan)

	for {
		data, err := c.readFromServer()
		if err != nil {
			fmt.Printf("readFromServer err: %+v\n", err)
			break
		}
		fmt.Printf("recv from server: %s\n", data)

		// 看看是不是打洞消息
		isPunch, addr := tryParsePunchMsg(data)
		if isPunch {
			udpAddr, err := net.ResolveUDPAddr("udp", addr)
			if err != nil {
				continue
			}
			c.punchChan <- udpAddr
			continue
		}

		// 普通控制消息
		resp, err := parseServerResponse(data)
		if err != nil {
			fmt.Printf("parse server resp error: %+v\n", err)
			continue
		}
		fmt.Printf("server resp: %+v\n", resp)
		c.serverRecvChan <- resp
	}
}

func (c *ChatClient) recvServerData() (*ServerResponse, error) {
	select {
	case data := <-c.serverRecvChan:
		return data, nil
	case <-time.After(3 * time.Second):
		return nil, fmt.Errorf("recv data timeout\n")
	}
}

func (c *ChatClient) login(name string) error {
	c.name = name
	return c.sendCmdToServer("login " + name)
}

func (c *ChatClient) doLogin(name string) error {
	if err := c.login(name); err != nil {
		return fmt.Errorf("send cmd error: %+v", err)
	}
	resp, err := c.recvServerData()
	if err != nil {
		return fmt.Errorf("recv server resp fail: %+v", err)
	}
	if resp == nil || !resp.Result {
		return fmt.Errorf("login fail: %s, try again", resp.Data)
	}
	id, err := strconv.Atoi(resp.Data)
	if err != nil {
		return fmt.Errorf("atoi fail, id must be int: %+v", err)
	}
	c.id = id
	return nil
}

func (c *ChatClient) logout() error {
	c.name = ""
	return c.sendCmdToServer(fmt.Sprintf("logout %d", c.id))
}

func (c *ChatClient) doLogout() error {
	if c.id == 0 {
		return fmt.Errorf("not login")
	}

	if err := c.logout(); err != nil {
		return fmt.Errorf("send cmd error: %+v", err)
	}
	resp, err := c.recvServerData()
	if err != nil {
		return fmt.Errorf("recv server resp fail: %+v", err)
	}
	if resp == nil || !resp.Result {
		return fmt.Errorf("logout fail: %s, try again", resp.Data)
	}
	return nil
}

func (c *ChatClient) getPeerClientAddr(peerID int) error {
	return c.sendCmdToServer(fmt.Sprintf("get %d", peerID))
}

func (c *ChatClient) doGet(peerID int) error {
	if c.id == 0 {
		return fmt.Errorf("not login")
	}

	if err := c.getPeerClientAddr(peerID); err != nil {
		return fmt.Errorf("send cmd error: %+v", err)
	}
	resp, err := c.recvServerData()
	if err != nil {
		return fmt.Errorf("recv server resp fail: %+v", err)
	}
	if resp == nil || !resp.Result {
		return fmt.Errorf("get fail: %s, try again", resp.Data)
	}

	addr, err := net.ResolveUDPAddr("udp", resp.Data)
	if err != nil {
		return fmt.Errorf("resolve addr fail: %+v", err)
	}
	c.peerAddr = addr
	return nil
}

func (c *ChatClient) punch(targetID int) error {
	return c.sendCmdToServer(fmt.Sprintf("punch %d %d", c.id, targetID))
}

func (c *ChatClient) doPunch(targetID int) error {
	if c.peerAddr == nil {
		return fmt.Errorf("not get peer addr now")
	}

	if err := c.punch(targetID); err != nil {
		return fmt.Errorf("send cmd error: %+v", err)
	}
	resp, err := c.recvServerData()
	if err != nil {
		return fmt.Errorf("recv server resp fail: %+v", err)
	}
	if resp == nil || !resp.Result {
		return fmt.Errorf("punch fail: %s, try again", resp.Data)
	}

	// TODO send punch hello until ok
	return c.sendToPeer(c.peerAddr, PunchMsg)
}

func parseInput(text string) (cmd string, args []string) {
	segs := strings.Split(text, " ")
	if len(segs) == 0 {
		return "", nil
	}
	cmd = segs[0]
	args = segs[1:]
	return
}

func (c *ChatClient) scan() {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("%s %s> ", c.LocalAddr, c.name)
		scanner.Scan()
		text := scanner.Text()
		cmd, args := parseInput(text)
		switch cmd {
		case "login":
			if len(args) != 1 {
				fmt.Printf("bad login cmd\n")
				continue
			}
			if err := c.doLogin(args[0]); err != nil {
				fmt.Printf("exec cmd error: %+v\n", err)
				continue
			}
			fmt.Printf("login success, ID: %d\n", c.id)
		case "logout":
			if err := c.doLogout(); err != nil {
				fmt.Printf("exec cmd error: %+v\n", err)
				continue
			}
			fmt.Printf("logout success\n")
		case "get":
			if len(args) != 1 {
				fmt.Printf("bad get cmd\n")
				continue
			}
			v, err := strconv.Atoi(args[0])
			if err != nil {
				fmt.Printf("%s: bad id format, must be int\n", args[0])
				continue
			}
			if err := c.doGet(v); err != nil {
				fmt.Printf("exec cmd error: %+v\n", err)
				continue
			}
			fmt.Printf("get %d addr success, addr: %s\n", v, c.peerAddr)
		case "punch":
			if len(args) != 1 {
				fmt.Printf("bad punch cmd\n")
				continue
			}
			v, err := strconv.Atoi(args[0])
			if err != nil {
				fmt.Printf("%s: bad id format, must be int\n", args[0])
				continue
			}
			if err := c.doPunch(v); err != nil {
				fmt.Printf("exec cmd error: %+v\n", err)
				continue
			}
			fmt.Printf("punch %d success, addr: %s\n", v, c.peerAddr)
		case "":
			continue
		default:
			fmt.Printf("unknown cmd\n")
			continue
		}
	}
}

func (c *ChatClient) Run() (err error) {
	if err := c.init(); err != nil {
		return err
	}
	go c.recvServerLoop()
	go c.recvPunchLoop()
	go c.recvPeerMsgLoop()

	c.scan()
	return nil
}

func main() {
	c := ChatClient{
		LocalAddr:  *LocalAddr,
		ServerAddr: *ServerAddr,
	}
	if err := c.Run(); err != nil {
		panic(err)
	}
}
