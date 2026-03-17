package parse

import (
	"testing"
)

func TestMapKillerFromKillfeed_Kill(t *testing.T) {
	e := KillfeedEvent{
		KillerID: "AAAAAAAAAAAAAAAA",
		UserName: "TestKiller",
		KilledID: "BBBBBBBBBBBBBBBB",
		IsAssist: false,
	}
	p := MapKillerFromKillfeed(e)
	if p.PlayerID != "AAAAAAAAAAAAAAAA" {
		t.Errorf("PlayerID = %q, want %q", p.PlayerID, "AAAAAAAAAAAAAAAA")
	}
	if p.Username != "TestKiller" {
		t.Errorf("Username = %q, want %q", p.Username, "TestKiller")
	}
	if p.Kills != 1 {
		t.Errorf("Kills = %d, want 1", p.Kills)
	}
	if p.Assists != 0 {
		t.Errorf("Assists = %d, want 0", p.Assists)
	}
}

func TestMapKillerFromKillfeed_Assist(t *testing.T) {
	e := KillfeedEvent{
		KillerID: "AAAAAAAAAAAAAAAA",
		UserName: "TestKiller",
		KilledID: "BBBBBBBBBBBBBBBB",
		IsAssist: true,
	}
	p := MapKillerFromKillfeed(e)
	if p.Assists != 1 {
		t.Errorf("Assists = %d, want 1", p.Assists)
	}
	if p.Kills != 0 {
		t.Errorf("Kills = %d, want 0", p.Kills)
	}
}

func TestMapKilledFromKillfeed(t *testing.T) {
	e := KillfeedEvent{
		KilledID:       "BBBBBBBBBBBBBBBB",
		KilledUserName: "TestKilled",
	}
	p := MapKilledFromKillfeed(e)
	if p.PlayerID != "BBBBBBBBBBBBBBBB" {
		t.Errorf("PlayerID = %q, want %q", p.PlayerID, "BBBBBBBBBBBBBBBB")
	}
	if p.Username != "TestKilled" {
		t.Errorf("Username = %q, want %q", p.Username, "TestKilled")
	}
	if p.Deaths != 1 {
		t.Errorf("Deaths = %d, want 1", p.Deaths)
	}
}

func TestMapPlayerScore(t *testing.T) {
	e := ScorefeedPlayerEvent{
		PlayerID:    "CCCCCCCCCCCCCC",
		UserName:    "TestPlayer",
		ScoreChange: 150,
		NewScore:    1500,
	}
	p := MapPlayerScore(e)
	if p.PlayerID != "CCCCCCCCCCCCCC" {
		t.Errorf("PlayerID = %q, want %q", p.PlayerID, "CCCCCCCCCCCCCC")
	}
	if p.Username != "TestPlayer" {
		t.Errorf("Username = %q, want %q", p.Username, "TestPlayer")
	}
	if p.RawScore != 150 {
		t.Errorf("RawScore = %d, want 150", p.RawScore)
	}
}
