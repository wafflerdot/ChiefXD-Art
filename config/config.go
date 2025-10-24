package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Token     string `json:"token"`
	BotPrefix string `json:"bot_prefix"`
}

func ReadConfig() (*Config, error) {
	fmt.Println("Reading config.json")
	data, err := os.ReadFile("./config.json")
	if err != nil {
		return nil, err
	}
	fmt.Println("Unmarshalling config.json")
	var cfg Config
	err = json.Unmarshal([]byte(data), &cfg)
	if err != nil {
		fmt.Println("Error unmarshalling config.json")
		return nil, err
	}
	return &cfg, nil
}
