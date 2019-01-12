package main

import (
	"github.com/kjx98/jabot"
)

func main() {
	cfg := jabot.NewConfig("")
	rebot, err := jabot.NewJabot(&cfg)
	if err != nil {
		panic(err)
	}
	rebot.SetRobotName("JacK")
	rebot.RegisterTimeCmd()

	rebot.Connect()
	rebot.Dail()
}
