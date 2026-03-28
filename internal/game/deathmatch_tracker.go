package game

import (
	"context"
	"log"

	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/bwmarrin/discordgo"
)

type DeathmatchTracker struct {
	logger *log.Logger
}

func NewDeathmatchTracker() *DeathmatchTracker {
	return &DeathmatchTracker{
		logger: log.New(log.Default().Writer(), "[DeathmatchTracker] ", log.Default().Flags()),
	}
}

func (t *DeathmatchTracker) OnMatchState(state string) {}

func (t *DeathmatchTracker) OnPlayerScore(e *parse.ScorefeedPlayerEvent) {
	if e.ScoreChange <= 0 {
		return
	}
	if err := data.AddPlayerScore(context.Background(), e.PlayerID, int(e.ScoreChange)); err != nil {
		t.logger.Printf("failed to add score for %s: %v", e.PlayerID, err)
	}
}

func (t *DeathmatchTracker) OnTeamScore(ctx context.Context, dc *discordgo.Session, e *parse.ScorefeedTeamEvent) {
}
