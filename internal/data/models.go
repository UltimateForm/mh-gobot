package data

type Player struct {
	PlayerID string
	Username string
	RawScore int
	Score    int
	Kills    int
	Deaths   int
	Assists  int
}

type RankedPlayer struct {
	Player
	Rank int
}

type PlayerPlacement struct {
	Rank    int
	Snippet []RankedPlayer
}
