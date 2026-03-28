package game

import (
	"context"

	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/bwmarrin/discordgo"
)

type GameTrackerCompute interface {
	OnMatchState(state string)
	OnPlayerScore(e *parse.ScorefeedPlayerEvent)
	OnTeamScore(ctx context.Context, dc *discordgo.Session, e *parse.ScorefeedTeamEvent)
}
