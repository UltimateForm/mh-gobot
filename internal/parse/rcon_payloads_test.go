package parse

import (
	"testing"
)

func TestParseKillfeedEvent_Kill(t *testing.T) {
	raw := "Killfeed: 2026.03.15-20.32.13: AAAAAAAAAAAAAAAA (TestKiller) killed BBBBBBBBBBBBBBBB (TestKilled)"
	e, err := ParseKillfeedEvent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e == nil {
		t.Fatal("expected event, got nil")
	}
	if e.KillerID != "AAAAAAAAAAAAAAAA" {
		t.Errorf("KillerID = %q, want %q", e.KillerID, "AAAAAAAAAAAAAAAA")
	}
	if e.UserName != "TestKiller" {
		t.Errorf("UserName = %q, want %q", e.UserName, "TestKiller")
	}
	if e.KilledID != "BBBBBBBBBBBBBBBB" {
		t.Errorf("KilledID = %q, want %q", e.KilledID, "BBBBBBBBBBBBBBBB")
	}
	if e.KilledUserName != "TestKilled" {
		t.Errorf("KilledUserName = %q, want %q", e.KilledUserName, "TestKilled")
	}
	if e.IsAssist {
		t.Error("IsAssist = true, want false")
	}
	if e.Date != "2026.03.15-20.32.13" {
		t.Errorf("Date = %q, want %q", e.Date, "2026.03.15-20.32.13")
	}
}

func TestParseKillfeedEvent_Assist(t *testing.T) {
	raw := "Killfeed: 2026.03.15-20.32.13: AAAAAAAAAAAAAAAA (TestKiller) got an assist kill for the death of BBBBBBBBBBBBBBBB (TestKilled)"
	e, err := ParseKillfeedEvent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e == nil {
		t.Fatal("expected event, got nil")
	}
	if e.KillerID != "AAAAAAAAAAAAAAAA" {
		t.Errorf("KillerID = %q, want %q", e.KillerID, "AAAAAAAAAAAAAAAA")
	}
	if e.KilledID != "BBBBBBBBBBBBBBBB" {
		t.Errorf("KilledID = %q, want %q", e.KilledID, "BBBBBBBBBBBBBBBB")
	}
	if !e.IsAssist {
		t.Error("IsAssist = false, want true")
	}
}

func TestParseKillfeedEvent_Invalid(t *testing.T) {
	e, err := ParseKillfeedEvent("not a killfeed event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e != nil {
		t.Errorf("expected nil, got %+v", e)
	}
}

// i dont like these table test but i dont really wanna worry about unit test opinions right now
// so im letting claude take care of them however he wants
func TestParseLoginEvent(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantNil  bool
		wantType string
		wantUser string
		wantID   string
		wantInst string
	}{
		{
			name:     "login",
			raw:      "Login: 2026.03.15-20.32.13: TestPlayer (CCCCCCCCCCCCCC) logged in",
			wantType: "Login",
			wantUser: "TestPlayer",
			wantID:   "CCCCCCCCCCCCCC",
			wantInst: "in",
		},
		{
			name:     "logout",
			raw:      "Logout: 2026.03.15-20.32.13: TestPlayer (CCCCCCCCCCCCCC) logged out",
			wantType: "Logout",
			wantUser: "TestPlayer",
			wantID:   "CCCCCCCCCCCCCC",
			wantInst: "out",
		},
		{
			name:    "invalid",
			raw:     "garbage input",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := ParseLoginEvent(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if e != nil {
					t.Errorf("expected nil, got %+v", e)
				}
				return
			}
			if e.EventType != tt.wantType {
				t.Errorf("EventType = %q, want %q", e.EventType, tt.wantType)
			}
			if e.UserName != tt.wantUser {
				t.Errorf("UserName = %q, want %q", e.UserName, tt.wantUser)
			}
			if e.PlayerID != tt.wantID {
				t.Errorf("PlayerID = %q, want %q", e.PlayerID, tt.wantID)
			}
			if e.Instance != tt.wantInst {
				t.Errorf("Instance = %q, want %q", e.Instance, tt.wantInst)
			}
		})
	}
}

func TestParseChatEvent(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantNil     bool
		wantUser    string
		wantChannel string
		wantMessage string
	}{
		{
			name:        "all chat",
			raw:         "Chat: CCCCCCCCCCCCCC, TestPlayer, (All) hello world",
			wantUser:    "TestPlayer",
			wantChannel: "All",
			wantMessage: "hello world",
		},
		{
			name:        "multiline message flattened",
			raw:         "Chat: CCCCCCCCCCCCCC, TestPlayer, (All) line one\nline two",
			wantUser:    "TestPlayer",
			wantChannel: "All",
			wantMessage: `line one \ line two`,
		},
		{
			name:    "invalid",
			raw:     "not a chat event",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := ParseChatEvent(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if e != nil {
					t.Errorf("expected nil, got %+v", e)
				}
				return
			}
			if e.UserName != tt.wantUser {
				t.Errorf("UserName = %q, want %q", e.UserName, tt.wantUser)
			}
			if e.Channel != tt.wantChannel {
				t.Errorf("Channel = %q, want %q", e.Channel, tt.wantChannel)
			}
			if e.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", e.Message, tt.wantMessage)
			}
		})
	}
}

func TestParseScoreboardRow(t *testing.T) {
	raw := "CCCCCCCCCCCCCC, TestPlayer, 0, 0, 1500, 12, 3, 2"
	e, err := ParseScoreboardRow(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e == nil {
		t.Fatal("expected entry, got nil")
	}
	if e.PlayerID != "CCCCCCCCCCCCCC" {
		t.Errorf("PlayerID = %q, want %q", e.PlayerID, "CCCCCCCCCCCCCC")
	}
	if e.UserName != "TestPlayer" {
		t.Errorf("UserName = %q, want %q", e.UserName, "TestPlayer")
	}
	if e.Score != 1500 {
		t.Errorf("Score = %d, want 1500", e.Score)
	}
	if e.Kills != 12 {
		t.Errorf("Kills = %d, want 12", e.Kills)
	}
	if e.Deaths != 3 {
		t.Errorf("Deaths = %d, want 3", e.Deaths)
	}
	if e.Assists != 2 {
		t.Errorf("Assists = %d, want 2", e.Assists)
	}
}

func TestParseScoreboard(t *testing.T) {
	raw := "CCCCCCCCCCCCCC, TestPlayer, 0, 0, 1500, 12, 3, 2\nDDDDDDDDDDDDDDDD, TestPlayer2, 0, 0, 800, 5, 7, 1"
	entries, err := ParseScoreboard(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	if entries[0].PlayerID != "CCCCCCCCCCCCCC" {
		t.Errorf("entries[0].PlayerID = %q, want %q", entries[0].PlayerID, "CCCCCCCCCCCCCC")
	}
	if entries[1].Score != 800 {
		t.Errorf("entries[1].Score = %d, want 800", entries[1].Score)
	}
}

func TestParseScorefeedEvent_Player(t *testing.T) {
	raw := "Scorefeed: 2026.03.15-20.32.13: CCCCCCCCCCCCCC (TestPlayer)'s score changed by 100 points and is now 1500 points"
	player, team, err := ParseScorefeedEvent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if player == nil {
		t.Fatal("expected player event, got nil")
	}
	if team != nil {
		t.Errorf("expected nil team event, got %+v", team)
	}
	if player.PlayerID != "CCCCCCCCCCCCCC" {
		t.Errorf("PlayerID = %q, want %q", player.PlayerID, "CCCCCCCCCCCCCC")
	}
	if player.ScoreChange != 100 {
		t.Errorf("ScoreChange = %v, want 100", player.ScoreChange)
	}
	if player.NewScore != 1500 {
		t.Errorf("NewScore = %v, want 1500", player.NewScore)
	}
}

func TestParseScorefeedEvent_Team(t *testing.T) {
	raw := "Scorefeed: 2026.03.15-20.32.13: Team 0's is now 500 points from 400 points"
	player, team, err := ParseScorefeedEvent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if team == nil {
		t.Fatal("expected team event, got nil")
	}
	if player != nil {
		t.Errorf("expected nil player event, got %+v", player)
	}
	if team.TeamID != 0 {
		t.Errorf("TeamID = %d, want 0", team.TeamID)
	}
	if team.NewScore != 500 {
		t.Errorf("NewScore = %v, want 500", team.NewScore)
	}
	if team.OldScore != 400 {
		t.Errorf("OldScore = %v, want 400", team.OldScore)
	}
}

func TestParseMatchstate(t *testing.T) {
	tests := []struct {
		raw   string
		want  string
		isNil bool
	}{
		{"MatchState: WaitingForPlayers", "WaitingForPlayers", false},
		{"MatchState: InProgress", "InProgress", false},
		{"not a matchstate", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseMatchstate(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
