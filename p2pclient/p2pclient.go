package main

import (
	"flag"
	"fmt"
	"github.com/libp2p/go-reuseport"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"udpdemo/proto"
)

var (
	LocalAddr         = flag.String("laddr", "0.0.0.0:10001", "local addr: ip:port")
	ServerAddr        = flag.String("raddr", "127.0.0.1:10086", "server addr: ip:port")
	PunchCnt          = flag.Int("n", 30, "打洞包数量")
	RecvServerTimeout = flag.Int("t", 15, "收服务器的包超时时间，单位秒")
)

type PunchPeerInfo struct {
	IsDone  bool
	UDPAddr *net.UDPAddr
}

type PeerMsg struct {
	ID   int
	Info ClientInfo

	UDPAddr net.Addr
	Msg     string
}

type ClientInfo struct {
	Name string
	Addr net.Addr
}

type ChatClient struct {
	onceHeartbeat sync.Once
	LocalAddr     *net.UDPAddr
	ServerAddr    *net.UDPAddr

	id   int
	name string
	conn net.PacketConn

	serverRecvChan chan *proto.ServerResponse
	punchChan      chan *net.UDPAddr // addr

	targetsInfo        *sync.Map // id -> addr
	punchTargetsInfo   *sync.Map // 主动要打洞的地址信息和状态 map[string]*PunchPeerInfo
	wantPunchPeersInfo *sync.Map // 被动打洞地址信息和状态

	clients *sync.Map // ID -> ClientInfo

	peerMsgChan chan *PeerMsg
}

func (c *ChatClient) GetPeerMsg() chan *PeerMsg {
	return c.peerMsgChan
}

func (c *ChatClient) Destroy() {
	close(c.punchChan)
	c.conn.Close()
	close(c.peerMsgChan)
	close(c.serverRecvChan)
	log.Printf("Destroy...")
}

func (c *ChatClient) init() error {
	c.serverRecvChan = make(chan *proto.ServerResponse)
	c.punchChan = make(chan *net.UDPAddr)
	c.targetsInfo = new(sync.Map)
	c.punchTargetsInfo = new(sync.Map)
	c.wantPunchPeersInfo = new(sync.Map)
	c.peerMsgChan = make(chan *PeerMsg, 2)
	c.clients = new(sync.Map)

	return c.listen()
}

// listen 客户端之间的连接
func (c *ChatClient) listen() (err error) {
	c.conn, err = reuseport.ListenPacket("udp", c.LocalAddr.String())
	return
}

func (c *ChatClient) recvMsgLoop() {
	b := make([]byte, 1024)

	log.Printf("start recv")
	for {
		n, addr, err := c.conn.ReadFrom(b)
		if err != nil {
			log.Printf("read error: %+v\n", err)
			break
		}
		if addr.String() == c.ServerAddr.String() {
			if err := c.handleServerMsg(b[:n]); err != nil {
				log.Printf("handle server msg error: %+v", err)
			}
			continue
		}
		c.handleClientMsg(addr, b[:n])
	}
}

func (c *ChatClient) handleClientMsg(addr net.Addr, data []byte) {
	msg := string(data)
	log.Printf("recv [%s] %s\n", addr, msg)

	if proto.IsPunchReply(msg) {
		// 主动打洞，收到了回复，说明打洞成功了
		if val, ok := c.punchTargetsInfo.Load(addr.String()); ok {
			val.(*PunchPeerInfo).IsDone = true
			if id, name, err := proto.ParsePunchReplyInfo(msg); err == nil {
				// 保存对方的个人信息
				c.clients.Store(id, ClientInfo{Name: name, Addr: addr})
				log.Printf("save info: %d %s", id, name)
			} else {
				log.Printf("parse info err: %v", err)
			}
			log.Printf("[%s] 主动打洞，收到了回应\n", addr)
		} else {
			log.Printf("bad punch reply, addr %s not found\n", addr)
		}
		return
	}
	if proto.IsPunchRequest(msg) {
		// 被动打洞，收到打洞者发来的消息，说明被打洞成功了
		val, ok := c.wantPunchPeersInfo.Load(addr.String())
		if !ok {
			return
		}
		val.(*PunchPeerInfo).IsDone = true
		if id, name, err := proto.ParsePunchReqInfo(msg); err == nil {
			c.clients.Store(id, ClientInfo{Name: name, Addr: addr})
			log.Printf("save info: %d %s", id, name)
		} else {
			log.Printf("parse info err: %v", err)
		}
		log.Printf("[%s] 被动打洞，收到了打洞请求\n", addr)
		return
	}

	log.Printf("recv peer msg: <%s> %s", addr, msg)
	id, msg, err := proto.ParseChatMsg(msg)
	if err != nil {
		log.Printf("bad char msg: %s", msg)
		return
	}
	client, ok := c.clients.Load(id)
	if !ok {
		log.Printf("%d not found in clients", id)
		return
	}
	// 普通消息
	c.peerMsgChan <- &PeerMsg{
		UDPAddr: addr,
		Msg:     msg,
		ID:      id,
		Info:    client.(ClientInfo),
	}
	return
}

func (c *ChatClient) handleServerMsg(data []byte) error {
	log.Printf("recv from server: %s\n", data)

	// 心跳
	if proto.IsHeartbeatReply(string(data)) {
		log.Printf("get heartbeat reply: %s", data)
		return nil
	}

	// 看看是不是打洞消息
	isPunch, addr := proto.TryParsePunchMsg(data)
	if isPunch {
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return fmt.Errorf("resolve punch addr error: %+v", err)
		}
		c.punchChan <- udpAddr
		return nil
	}

	// 普通控制消息
	resp, err := proto.ParseServerResponse(data)
	if err != nil {
		return fmt.Errorf("parse server resp error: %+v\n", err)
	}
	log.Printf("server resp: %+v\n", resp)
	c.serverRecvChan <- resp

	return nil
}

// recvPunchLoop 接收来自p2p server的打洞请求
func (c *ChatClient) recvPunchLoop() {
	for addr := range c.punchChan {
		log.Printf("do punch addr: %s\n", addr)

		addr := addr
		c.wantPunchPeersInfo.Store(addr.String(), &PunchPeerInfo{UDPAddr: addr})
		// 需要主动发送打洞消息
		for i := 0; i < *PunchCnt; i++ {
			v, _ := c.wantPunchPeersInfo.Load(addr.String())
			if v.(*PunchPeerInfo).IsDone {
				log.Printf("被动打洞还没发完10次就成功了 %s\n", addr)
				break
			}
			log.Printf("send punch reply to %s OK\n", addr)
			if err := c.sendToPeer(addr, proto.BuildPunchReply(strconv.Itoa(c.id), c.name)); err != nil {
				log.Printf("send to peer error: %+v\n", err)
				break
			}
			time.Sleep(time.Duration(100) * time.Millisecond)
		}
	}
}

func (c *ChatClient) recvServerData() (*proto.ServerResponse, error) {
	select {
	case data := <-c.serverRecvChan:
		return data, nil
	case <-time.After(time.Duration(*RecvServerTimeout) * time.Second):
		return nil, fmt.Errorf("recv data timeout\n")
	}
}
func (c *ChatClient) sendToPeer(addr net.Addr, msg string) error {
	b := []byte(msg)
	n, err := c.conn.WriteTo(b, addr)
	if err != nil || n != len(b) {
		return err
	}
	log.Printf("send to peer <%s> %s OK", addr, msg)
	return nil
}

func (c *ChatClient) SendToPeerByID(id int, msg string) error {
	client, ok := c.clients.Load(id)
	if !ok {
		return fmt.Errorf("%d not found", id)
	}
	return c.sendToPeer(client.(ClientInfo).Addr, proto.BuildChatMsg(c.id, msg))
}

func (c *ChatClient) sendCmdToServer(cmd string) error {
	b := []byte(cmd)
	n, err := c.conn.WriteTo(b, c.ServerAddr)
	if err != nil || n != len(b) {
		return err
	}
	return nil
}

func (c *ChatClient) sendHeartbeatToServerLoop() {
	for c.id != 0 { // has login
		if err := c.sendCmdToServer(proto.BuildHeartbeatMsg(c.id)); err != nil {
			log.Printf("send heartbeat fail: %+v", err)
		}
		time.Sleep(time.Duration(1) * time.Second)
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
	c.onceHeartbeat.Do(func() {
		go c.sendHeartbeatToServerLoop()
	})
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
	c.id = 0
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

	c.targetsInfo.Store(peerID, addr.String())
	c.punchTargetsInfo.Store(addr.String(), &PunchPeerInfo{UDPAddr: addr})
	return nil
}

func (c *ChatClient) punch(targetID int) error {
	return c.sendCmdToServer(proto.Cmd(proto.CmdPunch, strconv.Itoa(c.id), strconv.Itoa(targetID)))
}

func (c *ChatClient) DoPunch(targetID int) error {
	addr, ok := c.targetsInfo.Load(targetID)
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

	v, ok := c.punchTargetsInfo.Load(addr.(string))
	for i := 0; i < *PunchCnt; i++ {
		if ok && v.(*PunchPeerInfo).IsDone {
			// 提前结束
			log.Printf("%d %s getPunchDone when send punch\n", targetID, addr)
			break
		}
		if err := c.sendToPeer(v.(*PunchPeerInfo).UDPAddr, proto.BuildPunchReq(strconv.Itoa(c.id), c.name)); err != nil {
			return fmt.Errorf("send to peer fail: %+v\n", err)
		}
		time.Sleep(time.Duration(100) * time.Millisecond)
	}

	v.(*PunchPeerInfo).IsDone = false
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
		return fmt.Sprintf("login success, ID: %d", c.id)
	case "logout":
		if err := c.DoLogout(); err != nil {
			return fmt.Sprintf("exec cmd error: %+v", err)
		}
		log.Printf("logout success")
	case "get":
		if len(args) != 1 {
			return "bad get cmd"
		}
		v, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Sprintf("%s: bad id format, must be int", args[0])
		}
		if err := c.DoGet(v); err != nil {
			return fmt.Sprintf("exec cmd error: %+v", err)
		}
		addr, _ := c.targetsInfo.Load(v)
		return fmt.Sprintf("get %d addr success, addr: %s", v, addr.(string))
	case "punch":
		if len(args) != 1 {
			return "bad punch cmd"
		}
		v, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Sprintf("%s: bad id format, must be int", args[0])
		}
		if err := c.DoPunch(v); err != nil {
			return fmt.Sprintf("exec cmd error: %+v", err)
		}
		addr, _ := c.targetsInfo.Load(v)
		return fmt.Sprintf("punch %d success, addr: %s", v, addr)
	default:
		return "unknown cmd"
	}
	return ""
}

func (c *ChatClient) Run() (err error) {
	if err := c.init(); err != nil {
		return err
	}
	go c.recvPunchLoop()
	go c.recvMsgLoop()

	//if err := c.DoLogin(*NickName); err != nil {
	//	return err
	//}
	return nil
}

var p2pChatClient *ChatClient

func RunP2PChatClient() {
	localUDPAddr, err := net.ResolveUDPAddr("udp", *LocalAddr)
	if err != nil {
		panic(err)
	}
	serverUDPAddr, err := net.ResolveUDPAddr("udp", *ServerAddr)
	if err != nil {
		panic(err)
	}

	p2pChatClient = &ChatClient{
		LocalAddr:  localUDPAddr,
		ServerAddr: serverUDPAddr,
	}
	if err := p2pChatClient.Run(); err != nil {
		panic(err)
	}
}
