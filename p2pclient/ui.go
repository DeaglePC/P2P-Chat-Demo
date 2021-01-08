package main

import (
	"fmt"
	"github.com/marcusolsson/tui-go"
	"log"
	"strconv"
	"strings"
	"time"
)

type ChatUI struct {
	history       *tui.Box
	historyScroll *tui.ScrollArea
	historyBox    *tui.Box

	input    *tui.Entry
	inputBox *tui.Box
	chatBox  *tui.Box

	hintLabel *tui.Label
	hintBox   *tui.Box

	root *tui.Box
	UI   tui.UI
}

func (c *ChatUI) Init() (err error) {
	c.history = tui.NewVBox()
	c.historyScroll = tui.NewScrollArea(c.history)
	c.historyScroll.SetAutoscrollToBottom(true)

	c.historyBox = tui.NewVBox(c.historyScroll)
	c.historyBox.SetBorder(true)

	c.input = tui.NewEntry()
	c.input.SetFocused(true)
	c.input.SetSizePolicy(tui.Expanding, tui.Maximum)

	c.inputBox = tui.NewHBox(c.input)
	c.inputBox.SetBorder(true)
	c.inputBox.SetSizePolicy(tui.Expanding, tui.Maximum)

	c.chatBox = tui.NewVBox(c.historyBox, c.inputBox)
	c.chatBox.SetSizePolicy(tui.Expanding, tui.Expanding)

	c.input.OnSubmit(onSubmit)

	c.hintLabel = tui.NewLabel("")
	c.hintBox = tui.NewHBox(c.hintLabel)

	c.root = tui.NewVBox(c.chatBox, c.hintBox)

	c.UI, err = tui.New(c.root)
	if err != nil {
		return err
	}

	c.UI.SetKeybinding("Esc", onQuit)
	return nil
}

func (c *ChatUI) SetHint(text string) {
	c.hintLabel.SetText(text)
}

func (c *ChatUI) Quit() {
	c.UI.Quit()
}

func (c *ChatUI) Run() error {
	return c.UI.Run()
}

func (c *ChatUI) AppendMsg(prefix, text string) {
	c.history.Append(tui.NewHBox(
		tui.NewLabel(time.Now().Format("15:04")),
		tui.NewPadder(1, 0, tui.NewLabel(fmt.Sprintf("<%s>", prefix))),
		tui.NewLabel(text),
		tui.NewSpacer(),
	))
}

var chatUI *ChatUI

func runUI() {
	chatUI = &ChatUI{}
	if err := chatUI.Init(); err != nil {
		panic(err)
	}
	if err := chatUI.Run(); err != nil {
		log.Fatal(err)
	}
}

func onSubmit(e *tui.Entry) {
	text := e.Text()
	e.SetText("")

	if text[0] != '#' {
		segs := strings.SplitN(text, " ", 2)
		if len(segs) != 2 {
			return
		}
		id, err := strconv.Atoi(segs[0])
		if err != nil {
			chatUI.SetHint("bad id")
		}
		if err := p2pChatClient.SendToPeerByID(id, segs[1]); err != nil {
			chatUI.SetHint(fmt.Sprintf("send fail: %+v", err))
			return
		}
		chatUI.AppendMsg(*LocalAddr, text)
		return
	}

	input := text[1:]
	if len(input) == 0 {
		chatUI.SetHint("bad input")
		e.SetText("")
		return
	}
	chatUI.SetHint(p2pChatClient.ExecInput(input))
}

func onQuit() {
	chatUI.Quit()
}

func displayPeerMsg() {
	go func() {
		c := p2pChatClient.GetPeerMsg()
		for data := range c {
			if chatUI == nil {
				continue
			}
			log.Printf("================ [%d %s] %s", data.ID, data.Info.Name, data.Msg)
			chatUI.UI.Update(func() {
				chatUI.AppendMsg(data.UDPAddr.String(), fmt.Sprintf("[%d %s] %s", data.ID, data.Info.Name, data.Msg))
			})
		}
	}()
}
