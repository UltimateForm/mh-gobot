package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type dcConfig struct {
	DcToken             string
	PopChannel          string
	EventsChannel       string
	PublicEventsChannel string
	LeaderboardsChannel string
	KnownServers        []string
	Debug               bool
}

type rconConfig struct {
	RconUri            string
	RconPassword       string
	GameCommandPrefix  string
	SkirmishWinCap     float64
	SkirmishAltPopType int
	ServerNameOverride string
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
		log.Println("warning: missing pop channel, pop embed will be disabled")
	}
	if src.EventsChannel == "" {
		log.Println("warning: missing events channel, event messages will be disabled")
	}
	if src.PublicEventsChannel == "" {
		log.Println("warning: missing public events channel, public match end messages will be disabled")
	}
	if src.LeaderboardsChannel == "" {
		log.Println("warning: missing leaderboards channel, leaderboard embed will be disabled")
	}
	if src.RconUri == "" {
		log.Fatal("missing rcon uri")
	}
	if src.RconPassword == "" {
		log.Fatal("missing rcon password")
	}
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

var Global botConfig

func init() {
	log.Println("loading config from env...")
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatal(errors.Join(errors.New("failed to load bot config"), err))
	}
	knownServersStr := os.Getenv("KNOWN_SERVERS")
	knownServers := []string{}
	if knownServersStr != "" {
		knownServers = strings.Split(knownServersStr, ",")
		for i := range knownServers {
			knownServers[i] = strings.TrimSpace(knownServers[i])
		}
	}

	Global = botConfig{
		dcConfig: dcConfig{
			DcToken:             os.Getenv("DC_TOKEN"),
			PopChannel:          os.Getenv("POP_CHANNEL"),
			EventsChannel:       os.Getenv("EVENTS_CHANNEL"),
			PublicEventsChannel: os.Getenv("PUBLIC_EVENTS_CHANNEL"),
			LeaderboardsChannel: os.Getenv("LEADERBOARDS_CHANNEL"),
			KnownServers:        knownServers,
			Debug:               os.Getenv("DEBUG") == "1",
		},
		rconConfig: rconConfig{
			RconUri:            fmt.Sprintf("%v:%v", os.Getenv("RCON_ADDRESS"), os.Getenv("RCON_PORT")),
			RconPassword:       os.Getenv("RCON_PASSWORD"),
			GameCommandPrefix:  getEnvOrDefault("GAME_CMD_PREFIX", "!"),
			SkirmishWinCap:     func() float64 { v, _ := strconv.ParseFloat(getEnvOrDefault("SKIRMISH_WIN_CAP", "10"), 64); return v }(),
			SkirmishAltPopType: func() int { v, _ := strconv.Atoi(os.Getenv("SKIRMISH_ALT_POP_TYPE")); return v }(),
			ServerNameOverride: os.Getenv("SERVER_NAME_OVERRIDE"),
		},
	}
}
