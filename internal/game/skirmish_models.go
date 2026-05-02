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
	TeamSwitched     bool
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

func (p *SkirmishPlayer) GetTotalKills() int {
	total := 0
	for _, perf := range p.Rounds {
		total += perf.Kills
	}
	return total
}

func (p *SkirmishPlayer) GetTotalDeaths() int {
	total := 0
	for _, perf := range p.Rounds {
		total += perf.Deaths
	}
	return total
}

func (p *SkirmishPlayer) GetTotalAssists() int {
	total := 0
	for _, perf := range p.Rounds {
		total += perf.Assists
	}
	return total
}

func (p *SkirmishPlayer) GetParticipationRatio(totalRounds int) float64 {
	if totalRounds == 0 {
		return 0
	}
	return float64(len(p.Rounds)) / float64(totalRounds)
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
