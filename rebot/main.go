package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/kjx98/jabot"
	"os"
	"strings"
)

var username = flag.String("username", "test@localhost", "username")
var password = flag.String("password", "testme", "password")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: rebot [options]\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	cfg := jabot.NewConfig("")
	cfg.Jid = *username
	cfg.Passwd = *password
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
		line = strings.TrimRight(line, "\n")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tokens := strings.SplitN(line, " ", 2)
		if strings.ToLower(tokens[0]) == "quit" {
			break
		}
		switch strings.ToLower(tokens[0]) {
		case "list":
			contacts := rebot.GetContacts()
			for _, cc := range contacts {
				fmt.Println("Contact:", cc.Name, " Jid:", cc.Jid, " NickName:",
					cc.NickName)
			}
		}

		if len(tokens) == 2 {
			rebot.SendMessage(tokens[1], tokens[0])
		}
	}
}
