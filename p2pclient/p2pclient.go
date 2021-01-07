package main

import (
	"flag"
	"fmt"
	"github.com/libp2p/go-reuseport"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"udpdemo/proto"
)

const (
	PunchMsg   = "#hello#" // 主动
	PunchReply = "$world$" // 回复
)

var (
	LocalAddr  = flag.String("laddr", "127.0.0.1:10001", "local addr: ip:port")
	ServerAddr = flag.String("raddr", "127.0.0.1:10086", "server addr: ip:port")
	//NickName   = flag.String("name", "tom", "name")
)

func init() {
	flag.Parse()
}

type PunchPeerInfo struct {
	IsDone  bool
	UDPAddr *net.UDPAddr
}

type PeerMsg struct {
	UDPAddr net.Addr
	Msg     string
}

type ChatClient struct {
	LocalAddr  string
	ServerAddr string

	id         int
	name       string
	serverConn net.Conn       // 跟p2p服务器的链接
	peerConn   net.PacketConn // 跟对方客户端的链接

	serverRecvChan chan *proto.ServerResponse
	punchChan      chan *net.UDPAddr // addr

	targetsInfo      map[int]string            // id -> addr
	punchTargetsInfo map[string]*PunchPeerInfo // 主动要打洞的地址信息和状态

	wantPunchPeersInfo map[string]*PunchPeerInfo // 被动打洞地址信息和状态

	peerMsgChan chan *PeerMsg
}

func (c *ChatClient) GetPeerMsg() chan *PeerMsg {
	return c.peerMsgChan
}

func (c *ChatClient) Destroy() {
	close(c.punchChan)
	c.peerConn.Close()
	c.serverConn.Close()
	close(c.peerMsgChan)
	close(c.serverRecvChan)
	log.Printf("Destroy...")
}

func (c *ChatClient) init() error {
	c.serverRecvChan = make(chan *proto.ServerResponse)
	c.punchChan = make(chan *net.UDPAddr)
	c.targetsInfo = make(map[int]string)
	c.punchTargetsInfo = make(map[string]*PunchPeerInfo)
	c.wantPunchPeersInfo = make(map[string]*PunchPeerInfo)
	c.peerMsgChan = make(chan *PeerMsg, 2)

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
			log.Printf("read from peer error: %+v\n", err)
			break
		}

		msg := string(b[:n])
		log.Printf("recv from peer [%s] %s\n", addr, msg)
		if msg == PunchReply {
			// 主动打洞，收到了回复，说明打洞成功了
			log.Printf("[%s] udp hole punch success!\n", addr)
			if _, ok := c.punchTargetsInfo[addr.String()]; ok {
				c.punchTargetsInfo[addr.String()].IsDone = true
			} else {
				log.Printf("bad punch reply, addr %s not found\n", addr)
			}
			continue
		}
		if msg == PunchMsg {
			// 被动打洞，收到打洞者发来的消息，说明被打洞成功了
			c.wantPunchPeersInfo[addr.String()].IsDone = true
			log.Printf("被动打洞，收到了 %s hello\n", addr)
			continue
		}

		// 普通消息
		c.peerMsgChan <- &PeerMsg{
			UDPAddr: addr,
			Msg:     msg,
		}
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

func (c *ChatClient) SendToPeerByID(id int, msg string) error {
	addrStr, ok := c.targetsInfo[id]
	if !ok {
		return fmt.Errorf("%d not found", id)
	}
	addr := c.punchTargetsInfo[addrStr].UDPAddr
	return c.sendToPeer(addr, msg)
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

// listenPeer 客户端之间的连接
func (c *ChatClient) listenPeer() (err error) {
	c.peerConn, err = reuseport.ListenPacket("udp", c.LocalAddr)
	return
}

// recvPunchLoop 接收来自p2p server的打洞请求
func (c *ChatClient) recvPunchLoop() {
	for addr := range c.punchChan {
		log.Printf("do punch addr: %s\n", addr)

		addr := addr
		c.wantPunchPeersInfo[addr.String()] = &PunchPeerInfo{UDPAddr: addr}
		// 需要主动发送打洞消息
		for i := 0; i < 10; i++ {
			if c.wantPunchPeersInfo[addr.String()].IsDone {
				log.Printf("被动打洞还没发完10次就成功了 %s\n", addr)
				break
			}
			log.Printf("send punch reply to %s OK\n", addr)
			if err := c.sendToPeer(addr, PunchReply); err != nil {
				log.Printf("send to peer error: %+v\n", err)
				break
			}
			time.Sleep(time.Duration(100) * time.Millisecond)
		}
	}
}

func (c *ChatClient) recvServerLoop() {
	for {
		data, err := c.readFromServer()
		if err != nil {
			log.Printf("readFromServer err: %+v\n", err)
			break
		}
		log.Printf("recv from server: %s\n", data)

		// 看看是不是打洞消息
		isPunch, addr := proto.TryParsePunchMsg(data)
		if isPunch {
			udpAddr, err := net.ResolveUDPAddr("udp", addr)
			if err != nil {
				continue
			}
			c.punchChan <- udpAddr
			continue
		}

		// 普通控制消息
		resp, err := proto.ParseServerResponse(data)
		if err != nil {
			log.Printf("parse server resp error: %+v\n", err)
			continue
		}
		log.Printf("server resp: %+v\n", resp)
		c.serverRecvChan <- resp
	}
}

func (c *ChatClient) recvServerData() (*proto.ServerResponse, error) {
	select {
	case data := <-c.serverRecvChan:
		return data, nil
	case <-time.After(3 * time.Second):
		return nil, fmt.Errorf("recv data timeout\n")
	}
}

func (c *ChatClient) login(name string) error {
	c.name = name
	return c.sendCmdToServer(proto.Cmd(proto.CmdLogin, name))
}

func (c *ChatClient) DoLogin(name string) error {
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
	return c.sendCmdToServer(proto.Cmd(proto.CmdLogout, strconv.Itoa(c.id)))
}

func (c *ChatClient) DoLogout() error {
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
	return c.sendCmdToServer(proto.Cmd(proto.CmdGet, strconv.Itoa(peerID)))
}

func (c *ChatClient) DoGet(peerID int) error {
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

	c.targetsInfo[peerID] = addr.String()
	c.punchTargetsInfo[addr.String()] = &PunchPeerInfo{UDPAddr: addr}

	return nil
}

func (c *ChatClient) punch(targetID int) error {
	return c.sendCmdToServer(proto.Cmd(proto.CmdPunch, strconv.Itoa(c.id), strconv.Itoa(targetID)))
}

func (c *ChatClient) DoPunch(targetID int) error {
	addr, ok := c.targetsInfo[targetID]
	if !ok {
		return fmt.Errorf("not get peer %d addr now", targetID)
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

	for i := 0; i < 10; i++ {
		if c.punchTargetsInfo[addr].IsDone {
			// 提前结束
			log.Printf("%d %s getPunchDone when send punch\n", targetID, addr)
			break
		}
		if err := c.sendToPeer(c.punchTargetsInfo[addr].UDPAddr, PunchMsg); err != nil {
			return fmt.Errorf("send to peer fail: %+v\n", err)
		}
		time.Sleep(time.Duration(100) * time.Millisecond)
	}

	c.punchTargetsInfo[addr].IsDone = false
	log.Printf("send all punch req\n")
	return nil
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

func (c *ChatClient) ExecInput(text string) string {
	cmd, args := parseInput(text)
	switch cmd {
	case "login":
		if len(args) != 1 {
			return "bad login cmd"
		}
		if err := c.DoLogin(args[0]); err != nil {
			return fmt.Sprintf("exec cmd error: %+v", err)
		}
		return fmt.Sprintf("login success, ID: %d\n", c.id)
	case "logout":
		if err := c.DoLogout(); err != nil {
			return fmt.Sprintf("exec cmd error: %+v\n", err)
		}
		fmt.Printf("logout success\n")
	case "get":
		if len(args) != 1 {
			return "bad get cmd"
		}
		v, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Sprintf("%s: bad id format, must be int\n", args[0])
		}
		if err := c.DoGet(v); err != nil {
			return fmt.Sprintf("exec cmd error: %+v\n", err)
		}
		return fmt.Sprintf("get %d addr success, addr: %s\n", v, c.targetsInfo[v])
	case "punch":
		if len(args) != 1 {
			return "bad punch cmd"
		}
		v, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Sprintf("%s: bad id format, must be int\n", args[0])
		}
		if err := c.DoPunch(v); err != nil {
			return fmt.Sprintf("exec cmd error: %+v\n", err)
		}
		return fmt.Sprintf("punch %d success, addr: %s\n", v, c.targetsInfo[v])
	default:
		return "unknown cmd"
	}
	return ""
}

func (c *ChatClient) Run() (err error) {
	if err := c.init(); err != nil {
		return err
	}
	go c.recvServerLoop()
	go c.recvPunchLoop()
	go c.recvPeerMsgLoop()

	//if err := c.DoLogin(*NickName); err != nil {
	//	return err
	//}
	return nil
}

var p2pChatClient *ChatClient

func RunP2PChatClient() {
	p2pChatClient = &ChatClient{
		LocalAddr:  *LocalAddr,
		ServerAddr: *ServerAddr,
	}
	if err := p2pChatClient.Run(); err != nil {
		panic(err)
	}
}
