package data

type Player struct {
	PlayerID string
	Username string
	RawScore int
	Score    int
	// TODO: consider (AT THE VERY LEAST BRO) removing kills/deaths since with ledger we are achieving that already
	// data redundancy kinda jumps to the eye
	Kills      int
	Deaths     int
	Assists    int
	RoundsWon  int
	MatchesWon int
}

type RankedPlayer struct {
	Player
	Rank int
}

type PlayerPlacement struct {
	Rank    int
	Snippet []RankedPlayer
}

type PlayerAggregates struct {
	TotalPlayers int
	TotalKills   int
	TotalDeaths  int
}
