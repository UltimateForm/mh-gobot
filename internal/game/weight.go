package game

import (
	"context"
	"log"
	"math"
	"sync"

	"github.com/UltimateForm/mh-gobot/internal/data"
)

const scoreWeightFloor = 500.0

func ScoreWeight(currentScore int, avgScore float64) float64 {
	k := math.Max(avgScore, scoreWeightFloor)
	return k / (k + float64(currentScore))
}

type ScoreWeightProvider struct {
	mu       sync.RWMutex
	avgScore float64
	logger   *log.Logger
}

func NewScoreWeightProvider() *ScoreWeightProvider {
	return &ScoreWeightProvider{
		avgScore: scoreWeightFloor,
		logger:   log.New(log.Default().Writer(), "[ScoreWeight] ", log.Default().Flags()),
	}
}

func (p *ScoreWeightProvider) Refresh(ctx context.Context) {
	agg, err := data.ReadAggregates(ctx)
	if err != nil {
		p.logger.Printf("failed to refresh avg score: %v", err)
		return
	}
	p.mu.Lock()
	p.avgScore = agg.AvgScore
	p.mu.Unlock()
	p.logger.Printf("refreshed K=%.0f (avg_score=%.0f, floor=%.0f)", math.Max(agg.AvgScore, scoreWeightFloor), agg.AvgScore, scoreWeightFloor)
}

func (p *ScoreWeightProvider) Weight(currentScore int) float64 {
	p.mu.RLock()
	avg := p.avgScore
	p.mu.RUnlock()
	return ScoreWeight(currentScore, avg)
}

func (p *ScoreWeightProvider) AvgScore() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.avgScore
}
