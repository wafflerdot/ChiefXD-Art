package main

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

// Applies a minimal Rich Presence:
// - Activity Type: Watching
// - Name: "ChiefXD"
// No other elements are set.
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
