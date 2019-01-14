package main

import (
	"bufio"
	"fmt"
	"github.com/kjx98/jabot"
	"os"
	"strings"
)

func main() {
	cfg := jabot.NewConfig("")
	cfg.Jid = "test@localhost"
	cfg.Passwd = "testme"
	rebot, err := jabot.NewJabot(&cfg)
	if err != nil {
		panic(err)
	}
	rebot.RegisterTimeCmd()

	if err := rebot.Connect(); err != nil {
		fmt.Println("Connect", err)
		return
	}
	go rebot.Dail()
	for rebot.IsConnected() {
		in := bufio.NewReader(os.Stdin)
		line, err := in.ReadString('\n')
		if err != nil {
			continue
		}
		if len(line) >= 4 && line[:4] == "quit" {
			break
		}
		line = strings.TrimRight(line, "\n")

		tokens := strings.SplitN(line, " ", 2)
		if len(tokens) == 2 {
			rebot.SendMessage(tokens[1], tokens[0])
		}
	}
}
