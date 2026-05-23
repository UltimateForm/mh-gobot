package game

import (
	"time"

	"github.com/UltimateForm/mh-gobot/internal/data"
)

// IsDecaying answers "is this player currently being decayed?" using the same
// rules the decay job uses. It is the single source of truth for decay status —
// no DB column is stored. Callers should fetch decayCutoffScore once via
// data.ReadDecayCutoffScore and reuse it across many calls in batch contexts.
//
// decayCutoffScore is the score of the lowest player still inside the top
// CfgDecayTopPct slice. A score below the cutoff means the player has already
// fallen out of the eligible top and is no longer decaying.
func IsDecaying(p data.Player, cfg *GameConfig, decayCutoffScore int, now time.Time) bool {
	if cfg.Get(CfgDecayEnabled) == 0 {
		return false
	}
	if p.LastMatchPlayedAt == nil {
		return false
	}
	if p.Score < decayCutoffScore {
		return false
	}
	graceDays := cfg.Get(CfgDecayGraceDays)
	graceCutoff := now.Add(-time.Duration(graceDays * float64(24*time.Hour)))
	return p.LastMatchPlayedAt.Before(graceCutoff)
}
