package game

type SkirmishKDA struct {
	Kills int `json:"kills"`
	// Deaths expected to be 1 max per round
	Deaths  int `json:"deaths"`
	Assists int `json:"assists"`
	Score   int `json:"score"`
}

type SkirmishPlayer struct {
	Rounds   map[int]SkirmishKDA `json:"rounds"`
	PlayerId int                 `json:"player_id"`
	Name     string              `json:"name"`
}

type SkirmishMatch struct {
	Players []SkirmishPlayer `json:"players"`
	Rounds  int              `json:"rounds"`
}
