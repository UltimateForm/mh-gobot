package parse

type KillfeedEvent struct {
	EventType      string
	Date           string
	KillerID       string
	UserName       string
	KilledID       string
	KilledUserName string
	IsAssist       bool
}

type LoginEvent struct {
	EventType string
	Date      string
	UserName  string
	PlayerID  string
	Instance  string
}

type ChatEvent struct {
	EventType string
	PlayerID  string
	UserName  string
	Channel   string
	Message   string
}

type ServerInfo struct {
	Host       string
	ServerName string
	Version    string
	GameMode   string
	Map        string
}

type ScoreboardEntry struct {
	PlayerID string
	UserName string
	Score    int
	Kills    int
	Deaths   int
	Assists  int
}

type ScorefeedPlayerEvent struct {
	Date        string
	PlayerID    string
	UserName    string
	ScoreChange float64
	NewScore    float64
}

type ScorefeedTeamEvent struct {
	Date     string
	TeamID   int
	NewScore float64
	OldScore float64
}
