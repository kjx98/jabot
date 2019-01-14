package main

import (
	"bufio"
	"fmt"
	"flag"
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
