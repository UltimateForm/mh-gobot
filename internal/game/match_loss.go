package game

import "math"

// MatchLossCalc captures the full breakdown of a single losing player's
// end-of-match point loss. Future work will persist these to a public
// match-result log; keep the fields stable.
type MatchLossCalc struct {
	PlayerID           string
	Username           string  // display only; not persisted by the future match log
	LifetimeScore      int     // pre-loss; reflects score after this match's per-kill bonuses
	BaseAmount         int     // K * LossRatio
	LossFactor         float64 // clamp(score/K, 0, MaxFactor)
	SizeFactor         float64 // team-imbalance divisor (>= 1.0)
	RawLoss            int     // base * factor / sizeFactor (pre-floor)
	ActualLoss         int     // min(rawLoss, lifetimeScore) — what is actually subtracted
	ParticipationRatio float64 // display only; rounds present / total rounds
}

// ComputeMatchLoss returns the loss breakdown for a single losing player.
// Pure function: no DB, no embed, no side-effects. Caller is responsible
// for applying ActualLoss to the persisted score.
//
// avgScoreK must already be floored to scoreWeightFloor by the caller (the
// existing weight system uses the same K).
//
// sizeFactor >= 1.0; values > 1.0 mean the losing team was outnumbered and
// the loss should be reduced proportionally (sympathy).
//
// lossRatio == 0 disables losses entirely (kill switch).
func ComputeMatchLoss(
	playerID string,
	lifetimeScore int,
	avgScoreK float64,
	sizeFactor float64,
	lossRatio float64,
	maxFactor float64,
) MatchLossCalc {
	calc := MatchLossCalc{
		PlayerID:      playerID,
		LifetimeScore: lifetimeScore,
		SizeFactor:    sizeFactor,
	}
	if lossRatio <= 0 || avgScoreK <= 0 || sizeFactor <= 0 {
		return calc
	}
	factor := math.Min(math.Max(float64(lifetimeScore)/avgScoreK, 0), maxFactor)
	calc.LossFactor = factor
	calc.BaseAmount = int(math.Round(avgScoreK * lossRatio))
	calc.RawLoss = max(int(math.Round(float64(calc.BaseAmount)*factor/sizeFactor)), 0)
	calc.ActualLoss = max(min(calc.RawLoss, lifetimeScore), 0)
	return calc
}
