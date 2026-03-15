package config

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type dcConfig struct {
	DcToken    string
	PopChannel string
}

type rconConfig struct {
	RconUri      string
	RconPassword string
}

type botConfig struct {
	dcConfig
	rconConfig
}

// will ensure valid config, fatals out if invalid
func (src *botConfig) Validate() {
	if src.DcToken == "" {
		log.Fatal("missing dc token")
	}
	if src.PopChannel == "" {
		log.Fatal("missing pop channel")
	}
	if src.RconUri == "" {
		log.Fatal("missing rcon uri")
	}
	if src.RconPassword == "" {
		log.Fatal("missing rcon password")
	}
}

var Global botConfig

func init() {
	log.Println("loading config from env...")
	if err := godotenv.Load(); err != nil {
		log.Fatal(errors.Join(errors.New("failed to load bot config"), err))
	}
	Global = botConfig{
		dcConfig: dcConfig{
			DcToken:    os.Getenv("DC_TOKEN"),
			PopChannel: os.Getenv("POP_CHANNEL"),
		},
		rconConfig: rconConfig{
			RconUri:      fmt.Sprintf("%v:%v", os.Getenv("RCON_ADDRESS"), os.Getenv("RCON_PORT")),
			RconPassword: os.Getenv("RCON_PASSWORD"),
		},
	}
}
