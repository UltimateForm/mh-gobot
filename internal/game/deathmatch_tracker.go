package game

import (
	"context"
	"log"
	"math"

	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/bwmarrin/discordgo"
)

type DeathmatchTracker struct {
	weightProvider *ScoreWeightProvider
	logger         *log.Logger
}

func NewDeathmatchTracker(wp *ScoreWeightProvider) *DeathmatchTracker {
	return &DeathmatchTracker{
		weightProvider: wp,
		logger:         log.New(log.Default().Writer(), "[DeathmatchTracker] ", log.Default().Flags()),
	}
}

func (t *DeathmatchTracker) OnMatchState(state string) {}
func (t *DeathmatchTracker) OnKill(e *parse.KillfeedEvent)  {}

func (t *DeathmatchTracker) OnPlayerScore(e *parse.ScorefeedPlayerEvent) {
	if e.ScoreChange <= 0 {
		return
	}
	ctx := context.Background()
	player, err := data.ReadPlayer(ctx, e.PlayerID)
	currentScore := 0
	if err == nil {
		currentScore = player.Score
	}
	weight := t.weightProvider.Weight(currentScore)
	weightedDelta := int(math.Round(float64(e.ScoreChange) * weight))
	if err := data.AddPlayerScore(ctx, e.PlayerID, weightedDelta); err != nil {
		t.logger.Printf("failed to add score for %s: %v", e.PlayerID, err)
	}
}

func (t *DeathmatchTracker) OnTeamScore(ctx context.Context, dc *discordgo.Session, e *parse.ScorefeedTeamEvent) {
}
