// 所有命令，客户端未收到回复则进行重试
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"udpdemo/proto"
)

var Port = flag.Int("port", 10086, "listen port")

func init() {
	flag.Parse()
	initLog()
}

type ClientInfo struct {
	ID   int
	Name string

	UDPAddr *net.UDPAddr
}

type UDPMsg struct {
	Data       []byte
	RemoteAddr *net.UDPAddr
}

var (
	GClientID = 0
)

func getID() int {
	GClientID++
	return GClientID
}

type Server struct {
	Addr     *net.UDPAddr
	listener *net.UDPConn

	Clients map[int]ClientInfo
}

func (s *Server) ListenAndServer() {
	var err error
	s.listener, err = net.ListenUDP("udp", s.Addr)
	if err != nil {
		return
	}
	log.Printf("Local: <%s> \n", s.listener.LocalAddr().String())
	defer s.listener.Close()

	c := make(chan UDPMsg)

	go s.handleData(c)
	s.recvData(c)
}

func (s *Server) handleData(c <-chan UDPMsg) {
	for data := range c {
		log.Printf("[%s] handle data now: %s\n", data.RemoteAddr, data.Data)
		cmd, args := proto.ParseCmd(data.Data)
		if err := s.execCmd(data.RemoteAddr, cmd, args...); err != nil {
			log.Printf("exec cmd error: %+v\n", err)
		}
	}
}

func (s *Server) recvData(c chan<- UDPMsg) {
	defer close(c)
	data := make([]byte, 1024)
	for {
		n, remoteAddr, err := s.listener.ReadFromUDP(data)
		if err != nil {
			log.Printf("error during read: %s", err)
		}
		c <- UDPMsg{
			Data:       data[:n],
			RemoteAddr: remoteAddr,
		}
	}
}

func (s *Server) sendTo(addr *net.UDPAddr, data []byte) error {
	if n, err := s.listener.WriteToUDP(data, addr); err != nil || n != len(data) {
		return fmt.Errorf("[login] write error: %+v, n: %d", err, n)
	}
	return nil
}

func (s *Server) execCmd(addr *net.UDPAddr, cmd string, args ...string) error {
	switch strings.ToLower(cmd) {
	case proto.CmdLogin:
		if len(args) != 1 {
			return s.sendTo(addr, []byte(proto.BadArgsMsg(proto.CmdLogin)))
		}
		return s.login(addr, args[0])
	case proto.CmdLogout:
		if len(args) != 1 {
			return s.sendTo(addr, []byte(proto.BadArgsMsg(proto.CmdLogout)))
		}
		v, err := strconv.Atoi(args[0])
		if err != nil {
			return s.sendTo(addr, []byte(proto.FailureMsg(proto.CmdLogout, "id must be int")))
		}
		return s.logout(addr, v)
	case proto.CmdGet:
		if len(args) != 1 {
			return s.sendTo(addr, []byte(proto.BadArgsMsg(proto.CmdGet)))
		}
		v, err := strconv.Atoi(args[0])
		if err != nil {
			return s.sendTo(addr, []byte(proto.FailureMsg(proto.CmdGet, "id must be int")))
		}
		return s.getUserInfo(addr, v)
	case proto.CmdPunch:
		if len(args) != 2 {
			return s.sendTo(addr, []byte(proto.BadArgsMsg(proto.CmdPunch)))
		}
		v1, err1 := strconv.Atoi(args[0])
		v2, err2 := strconv.Atoi(args[1])
		if err1 != nil || err2 != nil {
			return s.sendTo(addr, []byte(proto.FailureMsg(proto.CmdPunch, "id must be int")))
		}
		return s.punch(addr, v1, v2)
	}
	return nil
}

// checkClient 检查id是否存在，true存在，false不存在，如果不存在，给addr发送不存在的消息
func (s *Server) checkClient(addr *net.UDPAddr, cmd string, id int) (bool, error) {
	if _, ok := s.Clients[id]; !ok {
		err := s.sendTo(addr, []byte(proto.FailureMsg(cmd, fmt.Sprintf("%d is not exists", id))))
		return false, err
	}
	return true, nil
}

// login 登录，保存用户信息
// request: login name
// response: login [OK userID]/[FAIL msg]
func (s *Server) login(addr *net.UDPAddr, name string) error {
	id := getID()
	if _, ok := s.Clients[id]; ok {
		return s.sendTo(addr, []byte(proto.FailureMsg(proto.CmdLogin, fmt.Sprintf("%d has exists\n", id))))
	}

	s.Clients[id] = ClientInfo{
		ID:      id,
		Name:    name,
		UDPAddr: addr,
	}
	log.Printf("Clients: %+v\n", s.Clients)

	return s.sendTo(addr, []byte(proto.SuccessMsg(proto.CmdLogin, fmt.Sprintf("%d", id))))
}

// logout 登出
// request：logout userID
// response: logout [OK msg]/[FAIL msg]
func (s *Server) logout(addr *net.UDPAddr, id int) error {
	if ok, err := s.checkClient(addr, proto.CmdLogout, id); !ok {
		return err
	}

	log.Printf("delete client: %+v\n", s.Clients[id])
	delete(s.Clients, id)
	log.Printf("Clients: %+v\n", s.Clients)

	return s.sendTo(addr, []byte(proto.SuccessMsg(proto.CmdLogout, "")))
}

// getUserInfo 获取id的地址信息
// request: get userID
// response: get OK ip:port/FAIL msg
func (s *Server) getUserInfo(addr *net.UDPAddr, id int) error {
	if ok, err := s.checkClient(addr, proto.CmdGet, id); !ok {
		return err
	}

	return s.sendTo(addr, []byte(proto.SuccessMsg(proto.CmdGet, s.Clients[id].UDPAddr.String())))
}

// punch 打洞消息，告诉targetID关于userID的地址信息，使得targetID可以发送打洞消息给userID
// request: punch userID targetID
// user response: punch OK/FAIL msg
// target msg: getpunch ip:port
func (s *Server) punch(addr *net.UDPAddr, userID, targetID int) error {
	if ok, err := s.checkClient(addr, proto.CmdPunch, userID); !ok {
		return err
	}
	if ok, err := s.checkClient(addr, proto.CmdPunch, targetID); !ok {
		return err
	}

	userInfo := s.Clients[userID]
	targetInfo := s.Clients[targetID]
	err := s.sendTo(targetInfo.UDPAddr, []byte(proto.Cmd(proto.CmdGetPunch, userInfo.UDPAddr.String())))
	if err != nil {
		targetErr := s.sendTo(addr, []byte(proto.FailureMsg(proto.CmdPunch, fmt.Sprintf("send punch to %d fail", targetID))))
		return fmt.Errorf("send punch data to target fail: %+v, send to target err: %+v", err, targetErr)
	}

	return s.sendTo(addr, []byte(proto.SuccessMsg(proto.CmdPunch, "")))
}

func main() {
	server := Server{
		Addr:    &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: *Port},
		Clients: make(map[int]ClientInfo),
	}
	server.ListenAndServer()
}
