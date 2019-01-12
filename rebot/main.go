package main

import (
	"fmt"
	"github.com/kjx98/jabot"
)

func main() {
	cfg := jabot.NewConfig("")
	cfg.Jid = "test@localhost"
	cfg.Passwd = "testme"
	rebot, err := jabot.NewJabot(&cfg)
	if err != nil {
		panic(err)
	}
	rebot.SetRobotName("JacK")
	rebot.RegisterTimeCmd()

	if err := rebot.Connect(); err != nil {
		fmt.Println("Connect", err)
		return
	}
	rebot.Dail()
}
