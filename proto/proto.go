package proto

import (
	"fmt"
	"log"
	"strconv"
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

	PunchRequest = "#hello" // 主动
	PunchReply   = "$world" // 回复
)

func IsPunchRequest(msg string) bool {
	return strings.HasPrefix(msg, PunchRequest)
}

func IsPunchReply(msg string) bool {
	return strings.HasPrefix(msg, PunchReply)
}

func BuildPunchReq(args ...string) string {
	return fmt.Sprintf("%s#%s#", PunchRequest, strings.Join(args, "#"))
}

func BuildPunchReply(args ...string) string {
	return fmt.Sprintf("%s$%s$", PunchReply, strings.Join(args, "$"))
}

func parsePunchInfo(msg string, isReq bool) (int, string, error) {
	var splitChar string
	if isReq {
		splitChar = "#"
	} else {
		splitChar = "$"
	}

	msg = msg[1 : len(msg)-1]
	segs := strings.Split(msg, splitChar)
	log.Printf("~~~~~~~~~~~%s ~~~ %+v", msg, segs)
	if len(segs) != 3 {
		return 0, "", fmt.Errorf("bad req")
	}
	id, err := strconv.Atoi(segs[1])
	log.Printf("@@@@@@@@ %d %+v --- %s", id, err, segs[1])
	if err != nil {
		return 0, "", err
	}
	return id, segs[2], nil
}

func ParsePunchReqInfo(msg string) (int, string, error) {
	return parsePunchInfo(msg, true)
}

func ParsePunchReplyInfo(msg string) (int, string, error) {
	return parsePunchInfo(msg, false)
}

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

func BuildChatMsg(srcID int, msg string) string {
	return fmt.Sprintf("%d|%s", srcID, msg)
}

func ParseChatMsg(msg string) (int, string, error) {
	segs := strings.SplitN(msg, "|", 2)
	if len(segs) != 2 {
		return 0, "", fmt.Errorf("split chat msg error")
	}
	id, err := strconv.Atoi(segs[0])
	if err != nil {
		return 0, "", fmt.Errorf("split chat msg atoi error: %v", err)
	}
	return id, segs[1], nil
}
