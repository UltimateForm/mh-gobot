package game

import (
	"context"
	"log"
	"sync"

	"github.com/UltimateForm/mh-gobot/internal/data"
)

type RankTierProvider struct {
	mu     sync.Mutex
	tiers  []data.RankTier // sorted ascending by ScoreGate
	logger *log.Logger
}

func NewRankTierProvider() *RankTierProvider {
	return &RankTierProvider{
		logger: log.New(log.Default().Writer(), "[RankTiers] ", log.Default().Flags()),
	}
}

func (p *RankTierProvider) Refresh(ctx context.Context) {
	tiers, err := data.ReadRankTiers(ctx)
	if err != nil {
		p.logger.Printf("failed to refresh rank tiers: %v", err)
		return
	}
	p.mu.Lock()
	p.tiers = tiers
	p.mu.Unlock()
	p.logger.Printf("refreshed %d rank tier(s)", len(tiers))
}

// Current returns the highest tier whose ScoreGate is <= score. The bool
// is false if the player is below all configured gates.
func (p *RankTierProvider) Current(score int) (data.RankTier, bool) {
	p.mu.Lock()
	defer p.mu.Unlock() // llock here is likely overkill but let's roll with it for now
	var match data.RankTier
	found := false
	for i := range p.tiers {
		if p.tiers[i].ScoreGate <= score {
			match = p.tiers[i]
			found = true
		} else {
			break
		}
	}
	return match, found
}

// Next returns the lowest tier whose ScoreGate is > score. The bool is
// false if the player is at or above the top tier.
func (p *RankTierProvider) Next(score int) (data.RankTier, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.tiers {
		if p.tiers[i].ScoreGate > score {
			return p.tiers[i], true
		}
	}
	return data.RankTier{}, false
}

func (p *RankTierProvider) All() []data.RankTier {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]data.RankTier, len(p.tiers))
	copy(out, p.tiers)
	return out
}
