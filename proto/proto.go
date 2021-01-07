package proto

import (
	"fmt"
	"strings"
)

const (
	CmdSplitChar = " "
	ArgSplitChar = " "
	BadArgsHint  = "FAIL bad args"

	CmdLogin    = "login"
	CmdLogout   = "logout"
	CmdGet      = "get"
	CmdPunch    = "punch"
	CmdGetPunch = "getpunch"

	Success = "OK"
	Failure = "FAIL"
)

func Cmd(cmd string, args ...string) string {
	return strings.Join([]string{cmd, strings.Join(args, ArgSplitChar)}, CmdSplitChar)
}

func ParseCmd(rawData []byte) (cmd string, args []string) {
	segments := strings.Split(string(rawData), CmdSplitChar)
	fmt.Printf("segments: %v\n", segments)
	cmd = segments[0]
	args = segments[1:]
	return
}

func BadArgsMsg(cmd string) string {
	return strings.Join([]string{cmd, BadArgsHint}, CmdSplitChar)
}

func ResponseMsg(cmd, msg string, isSuccess bool) string {
	var res string
	if isSuccess {
		res = Success
	} else {
		res = Failure
	}

	text := []string{cmd, res}
	if len(msg) > 0 {
		text = append(text, msg)
	}
	return strings.Join(text, CmdSplitChar)
}

func SuccessMsg(cmd, msg string) string {
	return ResponseMsg(cmd, msg, true)
}

func FailureMsg(cmd, msg string) string {
	return ResponseMsg(cmd, msg, false)
}

type ServerResponse struct {
	Cmd    string // login/logout/get/punch
	Result bool   // OK->true/FAIL->false
	Data   string
}

func ParseServerResponse(b []byte) (*ServerResponse, error) {
	resp := string(b)
	segs := strings.SplitN(resp, CmdSplitChar, 3)
	if len(segs) < 2 {
		return nil, fmt.Errorf("bad server response")
	}
	offset := len(segs[0]) + len(segs[1]) + 1
	if len(resp) > offset {
		offset++
	}
	return &ServerResponse{
		Cmd:    segs[0],
		Result: segs[1] == Success,
		Data:   resp[offset:],
	}, nil
}

// TryParsePunchMsg 尝试解析打洞消息，返回值： 是否打洞消息，打洞地址
func TryParsePunchMsg(b []byte) (bool, string) {
	resp := string(b)
	segs := strings.Split(resp, CmdSplitChar)
	if len(segs) != 2 {
		return false, ""
	}
	if segs[0] == CmdGetPunch {
		return true, segs[1]
	}
	return false, ""
}
