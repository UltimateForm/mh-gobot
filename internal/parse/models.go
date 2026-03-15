package parse

type KillfeedEvent struct {
	EventType      string
	Date           string
	KillerID       string
	UserName       string
	KilledID       string
	KilledUserName string
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
}
