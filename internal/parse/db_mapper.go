package parse

import "github.com/UltimateForm/mh-gobot/internal/data"

func MapPlayerScore(e ScorefeedPlayerEvent) data.Player {
	return data.Player{
		PlayerID: e.PlayerID,
		Username: e.UserName,
		RawScore: int(e.ScoreChange),
	}
}

func MapKillerFromKillfeed(e KillfeedEvent) data.Player {
	p := data.Player{
		PlayerID: e.KillerID,
		Username: e.UserName,
	}
	if e.IsAssist {
		p.Assists = 1
	} else {
		p.Kills = 1
	}
	return p
}

func MapKilledFromKillfeed(e KillfeedEvent) data.Player {
	return data.Player{
		PlayerID: e.KilledID,
		Username: e.KilledUserName,
		Deaths:   1,
	}
}
