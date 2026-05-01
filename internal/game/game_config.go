package game

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/UltimateForm/mh-gobot/internal/data"
)

const (
	CfgSkirmishRoundWinMod   = "skirmish_round_win_mod"
	CfgSkirmishMatchWinMod   = "skirmish_match_win_mod"
	CfgSkirmishSizeFactorCap = "skirmish_size_factor_cap"
	CfgSkirmishWinCap        = "skirmish_win_cap"
	CfgMatchLossRatio        = "match_loss_ratio"
	CfgMatchLossFactorCap    = "match_loss_factor_cap"
)

var gameConfigDefaults = map[string]float64{
	CfgSkirmishRoundWinMod:   0.5,
	CfgSkirmishMatchWinMod:   0.25,
	CfgSkirmishSizeFactorCap: 2.0,
	CfgSkirmishWinCap:        10.0,
	CfgMatchLossRatio:        0.20,
	CfgMatchLossFactorCap:    4.0,
}

var GameConfigDescriptions = map[string]string{
	CfgSkirmishRoundWinMod:   "Multiplier on a player's round score to compute their per-round win bonus. Higher = bigger round bonuses.",
	CfgSkirmishMatchWinMod:   "Multiplier on a player's match-result score for the end-of-match bonus (added on top of the final round bonus).",
	CfgSkirmishSizeFactorCap: "Cap on the team-imbalance factor used to scale bonuses/losses when teams are uneven. Sympathy clamp.",
	CfgSkirmishWinCap:        "Number of round wins needed to end a skirmish match. Read once at startup; takes effect on bot restart.",
	CfgMatchLossRatio:        "Fraction of K used as base match loss for losing players. 0 disables losses entirely.",
	CfgMatchLossFactorCap:    "Cap on the loss factor (clamps lifetime_score/K). Higher = bigger max losses for top-ranked players.",
}

func GameConfigDefaults() map[string]float64 {
	out := make(map[string]float64, len(gameConfigDefaults))
	for k, v := range gameConfigDefaults {
		out[k] = v
	}
	return out
}

type GameConfig struct {
	mu     sync.RWMutex
	values map[string]float64
	logger *log.Logger
}

func NewGameConfig() *GameConfig {
	values := make(map[string]float64, len(gameConfigDefaults))
	for k, v := range gameConfigDefaults {
		values[k] = v
	}
	return &GameConfig{
		values: values,
		logger: log.New(log.Default().Writer(), "[GameConfig] ", log.Default().Flags()),
	}
}

func (g *GameConfig) Seed(ctx context.Context) {
	for key, def := range gameConfigDefaults {
		_, err := data.GetMetaNumber(ctx, key)
		if errors.Is(err, data.DbMetaNumberNotFound) {
			if err := data.SetMetaNumber(ctx, key, def); err != nil {
				g.logger.Printf("failed to seed %s=%.4f: %v", key, def, err)
				continue
			}
			g.logger.Printf("seeded %s=%.4f", key, def)
		} else if err != nil {
			g.logger.Printf("failed to read %s during seed: %v", key, err)
		}
	}
}

func (g *GameConfig) Refresh(ctx context.Context) {
	all, err := data.GetAllMetaNumbers(ctx)
	if err != nil {
		g.logger.Printf("failed to refresh: %v", err)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for key, def := range gameConfigDefaults {
		if v, ok := all[key]; ok {
			g.values[key] = v
		} else {
			g.values[key] = def
		}
	}
}

func (g *GameConfig) Get(key string) float64 {
	g.mu.RLock()
	v, ok := g.values[key]
	g.mu.RUnlock()
	if ok {
		return v
	}
	if def, ok := gameConfigDefaults[key]; ok {
		return def
	}
	return 0
}

func (g *GameConfig) Set(ctx context.Context, key string, value float64) error {
	if _, ok := gameConfigDefaults[key]; !ok {
		return errors.New("unknown game config key")
	}
	if err := data.SetMetaNumber(ctx, key, value); err != nil {
		return err
	}
	g.mu.Lock()
	g.values[key] = value
	g.mu.Unlock()
	return nil
}

func (g *GameConfig) All() map[string]float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make(map[string]float64, len(g.values))
	for k, v := range g.values {
		out[k] = v
	}
	return out
}
