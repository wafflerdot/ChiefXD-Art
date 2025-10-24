package main

import (
	"fmt"
	"github.com/alfredosa/GoDiscordBot/bot"
)

func main() {
	bot.Start()

	<-make(chan struct{})
}
