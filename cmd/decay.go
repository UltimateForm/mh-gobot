package cmd

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/game"
)

const (
	decayTickInterval = time.Hour
	decayMinGap       = 24 * time.Hour
	metaDecayLastRun  = "score_decay_last_run_at"
)

var decayLogger = log.New(log.Default().Writer(), "[Decay] ", log.Default().Flags())

// DecayResult summarises one decay tick outcome for logging/UI.
type DecayResult struct {
	RowsAffected int64
	TopN         int
	Skipped      string // reason if no work happened ("disabled", "no active players", "topN=0", "too soon"); empty when we acted
}

// runDecayLoop fires an hourly ticker and runs decay when ≥ decayMinGap has
// elapsed since the last successful run.
func runDecayLoop(ctx context.Context, cfg *game.GameConfig) {
	ticker := time.NewTicker(decayTickInterval)
	defer ticker.Stop()
	// Initial check on startup
	if _, err := runDecayIfDue(ctx, cfg); err != nil {
		decayLogger.Printf("startup decay check error: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			decayLogger.Println("exiting due to context done")
			return
		case <-ticker.C:
			if _, err := runDecayIfDue(ctx, cfg); err != nil {
				decayLogger.Printf("tick error: %v", err)
			}
		}
	}
}

// runDecayIfDue runs a decay tick only if ≥ decayMinGap has elapsed since the
// last successful run. Returns a DecayResult describing what happened.
func runDecayIfDue(ctx context.Context, cfg *game.GameConfig) (DecayResult, error) {
	now := time.Now()
	last, err := readLastDecayRun(ctx)
	if err != nil {
		return DecayResult{}, err
	}
	if !last.IsZero() && now.Sub(last) < decayMinGap {
		return DecayResult{Skipped: "too soon"}, nil
	}
	return runDecayCore(ctx, cfg, now)
}

// runDecayCore performs one decay tick unconditionally and bumps the meta
// timestamp on success. Use this for the /decay_now admin path.
func runDecayCore(ctx context.Context, cfg *game.GameConfig, now time.Time) (DecayResult, error) {
	if cfg.Get(game.CfgDecayEnabled) == 0 {
		decayLogger.Println("decay disabled, skipping")
		return DecayResult{Skipped: "disabled"}, nil
	}
	topN, err := computeTopN(ctx, cfg)
	if err != nil {
		return DecayResult{}, err
	}
	if topN == 0 {
		decayLogger.Println("no active players or topN computed as 0, skipping")
		return DecayResult{Skipped: "topN=0"}, nil
	}
	pct := cfg.Get(game.CfgDecayPctPerDay)
	graceDays := cfg.Get(game.CfgDecayGraceDays)
	inactiveCutoff := now.Add(-time.Duration(graceDays * float64(24*time.Hour)))

	affected, err := data.DecayTopPlayers(ctx, pct, topN, inactiveCutoff)
	if err != nil {
		return DecayResult{}, err
	}
	if err := data.SetMeta(ctx, metaDecayLastRun, now.Format(time.RFC3339)); err != nil {
		decayLogger.Printf("warning: failed to write %s meta: %v", metaDecayLastRun, err)
	}
	decayLogger.Printf("applied %.1f%% decay to %d/%d eligible top players (inactive since %s)", pct*100, affected, topN, inactiveCutoff.Format(time.RFC3339))
	return DecayResult{RowsAffected: affected, TopN: topN}, nil
}

func computeTopN(ctx context.Context, cfg *game.GameConfig) (int, error) {
	totalActive, err := data.CountActivePlayers(ctx)
	if err != nil {
		return 0, err
	}
	if totalActive == 0 {
		return 0, nil
	}
	topPct := cfg.Get(game.CfgDecayTopPct)
	topN := int(float64(totalActive) * topPct)
	return topN, nil
}

func readLastDecayRun(ctx context.Context) (time.Time, error) {
	v, err := data.GetMeta(ctx, metaDecayLastRun)
	if errors.Is(err, data.DbMetaNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		decayLogger.Printf("warning: meta %s has unparseable value %q, treating as never-run: %v", metaDecayLastRun, v, err)
		return time.Time{}, nil
	}
	return t, nil
}
