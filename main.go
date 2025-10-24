package main

import (
	_ "fmt"

	"github.com/alfredosa/GoDiscordBot/bot"
)

func main() {
	bot.Start()

	<-make(chan struct{})
}
