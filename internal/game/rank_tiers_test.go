package game

import (
	"testing"

	"github.com/UltimateForm/mh-gobot/internal/data"
)

func providerWithTiers(tiers []data.RankTier) *RankTierProvider {
	p := NewRankTierProvider()
	p.tiers = tiers
	return p
}

var standardTiers = []data.RankTier{
	{ScoreGate: 0, Name: "Recruit"},
	{ScoreGate: 5000, Name: "Soldier", ShortName: "SLD"},
	{ScoreGate: 15000, Name: "Knight"},
	{ScoreGate: 30000, Name: "Lord"},
}

func TestCurrent(t *testing.T) {
	tests := []struct {
		name     string
		tiers    []data.RankTier
		score    int
		expected string // empty string means nil
	}{
		{"empty provider returns nil", nil, 1000, ""},
		{"score below all gates returns nil", []data.RankTier{{ScoreGate: 100, Name: "X"}}, 50, ""},
		{"score exactly at gate", standardTiers, 5000, "Soldier"},
		{"score between gates returns lower", standardTiers, 7500, "Soldier"},
		{"score below first gate (with 0 floor)", standardTiers, 0, "Recruit"},
		{"score at top gate", standardTiers, 30000, "Lord"},
		{"score above top gate stays at top", standardTiers, 999999, "Lord"},
		{"score one below next gate", standardTiers, 14999, "Soldier"},
		{"score one above gate", standardTiers, 15001, "Knight"},
		{"negative score with no negative gate returns nil", []data.RankTier{{ScoreGate: 100, Name: "X"}}, -50, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := providerWithTiers(tt.tiers)
			got, ok := p.Current(tt.score)
			if tt.expected == "" {
				if ok {
					t.Errorf("Current(%d) = %+v, want not found", tt.score, got)
				}
				return
			}
			if !ok {
				t.Errorf("Current(%d) not found, want %q", tt.score, tt.expected)
				return
			}
			if got.Name != tt.expected {
				t.Errorf("Current(%d) = %q, want %q", tt.score, got.Name, tt.expected)
			}
		})
	}
}

func TestNext(t *testing.T) {
	tests := []struct {
		name     string
		tiers    []data.RankTier
		score    int
		expected string // empty string means nil
	}{
		{"empty provider returns nil", nil, 1000, ""},
		{"score below all gates returns first", []data.RankTier{{ScoreGate: 100, Name: "X"}}, 50, "X"},
		{"score between gates returns next", standardTiers, 7500, "Knight"},
		{"score exactly at gate returns next gate (not current)", standardTiers, 5000, "Knight"},
		{"score at top gate returns nil", standardTiers, 30000, ""},
		{"score above top gate returns nil", standardTiers, 999999, ""},
		{"score one below first gate", standardTiers, -1, "Recruit"},
		{"score one below mid gate returns that gate", standardTiers, 14999, "Knight"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := providerWithTiers(tt.tiers)
			got, ok := p.Next(tt.score)
			if tt.expected == "" {
				if ok {
					t.Errorf("Next(%d) = %+v, want not found", tt.score, got)
				}
				return
			}
			if !ok {
				t.Errorf("Next(%d) not found, want %q", tt.score, tt.expected)
				return
			}
			if got.Name != tt.expected {
				t.Errorf("Next(%d) = %q, want %q", tt.score, got.Name, tt.expected)
			}
		})
	}
}

func TestAllReturnsCopy(t *testing.T) {
	p := providerWithTiers(standardTiers)
	out := p.All()
	if len(out) != len(standardTiers) {
		t.Fatalf("All() len = %d, want %d", len(out), len(standardTiers))
	}
	out[0].Name = "MUTATED"
	again := p.All()
	if again[0].Name != "Recruit" {
		t.Errorf("mutating returned slice leaked into provider state: got %q", again[0].Name)
	}
}

func TestCurrentReturnsCopy(t *testing.T) {
	p := providerWithTiers(standardTiers)
	got, ok := p.Current(20000)
	if !ok {
		t.Fatal("Current(20000) not found, want Knight")
	}
	got.Name = "MUTATED"
	again, _ := p.Current(20000)
	if again.Name != "Knight" {
		t.Errorf("mutating Current() result leaked into provider state: got %q", again.Name)
	}
}
