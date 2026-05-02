package game

type SkirmishPlayerPerformance struct {
	Kills     int
	Deaths    int
	Assists   int
	Score     int
	KilledIds []string // victim player IDs (for both kills and assists)
}

type SkirmishPlayer struct {
	// key is match round number
	Rounds           map[int]SkirmishPlayerPerformance
	PlayerId         string
	Name             string
	Team             int
	QuitAtRound      int
	MatchResultScore int
}

func (p *SkirmishPlayer) AddRound(round int, perf SkirmishPlayerPerformance) {
	p.Rounds[round] = perf
}

func (p *SkirmishPlayer) GetTotalScore() int {
	totalPerRounds := 0
	for _, perf := range p.Rounds {
		totalPerRounds += perf.Score
	}
	return totalPerRounds + p.MatchResultScore
}

type SkirmishMatchRound struct {
	Team1  []string
	Team2  []string
	Winner int
}

type SkirmishMatch struct {
	Rounds []SkirmishMatchRound
	Winner int
}
