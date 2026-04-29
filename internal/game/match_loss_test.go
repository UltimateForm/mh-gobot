package game

import "testing"

func TestComputeMatchLoss(t *testing.T) {
	tests := []struct {
		name           string
		lifetimeScore  int
		avgScoreK      float64
		sizeFactor     float64
		lossRatio      float64
		maxFactor      float64
		wantLossFactor float64
		wantBase       int
		wantRawLoss    int
		wantActualLoss int
	}{
		{
			name:           "zero score loses nothing",
			lifetimeScore:  0,
			avgScoreK:      500,
			sizeFactor:     1.0,
			lossRatio:      0.25,
			maxFactor:      4.0,
			wantLossFactor: 0.0,
			wantBase:       125,
			wantRawLoss:    0,
			wantActualLoss: 0,
		},
		{
			name:           "score equals K, factor is 1.0",
			lifetimeScore:  500,
			avgScoreK:      500,
			sizeFactor:     1.0,
			lossRatio:      0.25,
			maxFactor:      4.0,
			wantLossFactor: 1.0,
			wantBase:       125,
			wantRawLoss:    125,
			wantActualLoss: 125,
		},
		{
			name:           "score above cap clamped to maxFactor",
			lifetimeScore:  50000,
			avgScoreK:      500,
			sizeFactor:     1.0,
			lossRatio:      0.25,
			maxFactor:      4.0,
			wantLossFactor: 4.0,
			wantBase:       125,
			wantRawLoss:    500,
			wantActualLoss: 500,
		},
		{
			name:           "size factor 2 halves the loss",
			lifetimeScore:  500,
			avgScoreK:      500,
			sizeFactor:     2.0,
			lossRatio:      0.25,
			maxFactor:      4.0,
			wantLossFactor: 1.0,
			wantBase:       125,
			wantRawLoss:    63,
			wantActualLoss: 63,
		},
		{
			name:           "lossRatio 0 disables losses",
			lifetimeScore:  10000,
			avgScoreK:      500,
			sizeFactor:     1.0,
			lossRatio:      0,
			maxFactor:      4.0,
			wantLossFactor: 0.0,
			wantBase:       0,
			wantRawLoss:    0,
			wantActualLoss: 0,
		},
		{
			name:           "actualLoss clamped to lifetime score",
			lifetimeScore:  100,
			avgScoreK:      500,
			sizeFactor:     1.0,
			lossRatio:      0.25,
			maxFactor:      4.0,
			wantLossFactor: 0.2,
			wantBase:       125,
			wantRawLoss:    25,
			wantActualLoss: 25,
		},
		{
			name:           "low score below K — small factor, small loss",
			lifetimeScore:  50,
			avgScoreK:      500,
			sizeFactor:     1.0,
			lossRatio:      0.25,
			maxFactor:      4.0,
			wantLossFactor: 0.1,
			wantBase:       125,
			wantRawLoss:    13,
			wantActualLoss: 13,
		},
		{
			name:           "K grows with avgScore — larger base",
			lifetimeScore:  2000,
			avgScoreK:      2000,
			sizeFactor:     1.0,
			lossRatio:      0.25,
			maxFactor:      4.0,
			wantLossFactor: 1.0,
			wantBase:       500,
			wantRawLoss:    500,
			wantActualLoss: 500,
		},
		{
			name:           "K is 5039",
			lifetimeScore:  23000,
			avgScoreK:      5039,
			sizeFactor:     1.0,
			lossRatio:      0.25,
			maxFactor:      4.0,
			wantLossFactor: 4,
			wantBase:       1260,
			wantRawLoss:    5040,
			wantActualLoss: 5040,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeMatchLoss("p1", tt.lifetimeScore, tt.avgScoreK, tt.sizeFactor, tt.lossRatio, tt.maxFactor)
			if got.LossFactor != tt.wantLossFactor {
				t.Errorf("LossFactor = %v, want %v", got.LossFactor, tt.wantLossFactor)
			}
			if got.BaseAmount != tt.wantBase {
				t.Errorf("BaseAmount = %v, want %v", got.BaseAmount, tt.wantBase)
			}
			if got.RawLoss != tt.wantRawLoss {
				t.Errorf("RawLoss = %v, want %v", got.RawLoss, tt.wantRawLoss)
			}
			if got.ActualLoss != tt.wantActualLoss {
				t.Errorf("ActualLoss = %v, want %v", got.ActualLoss, tt.wantActualLoss)
			}
			if got.PlayerID != "p1" {
				t.Errorf("PlayerID = %v, want p1", got.PlayerID)
			}
			if got.LifetimeScore != tt.lifetimeScore {
				t.Errorf("LifetimeScore = %v, want %v", got.LifetimeScore, tt.lifetimeScore)
			}
			if got.SizeFactor != tt.sizeFactor {
				t.Errorf("SizeFactor = %v, want %v", got.SizeFactor, tt.sizeFactor)
			}
		})
	}
}
