package main

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

// Rich Presence:
// - Activity Type: Watching
// - Name: "ChiefXD"
func onReadySetPresence(s *discordgo.Session, _ *discordgo.Ready) {
	if err := s.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: "ChiefXD",
				Type: discordgo.ActivityTypeWatching,
			},
		},
	}); err != nil {
		log.Println("failed to set rich presence:", err)
	}
}
