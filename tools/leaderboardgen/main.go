package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/UltimateForm/mh-gobot/internal/config"
	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/img"
	"github.com/UltimateForm/mh-gobot/internal/scribe"
)

func main() {
	outputPath := flag.String("o", "leaderboard.png", "output file path")
	limit := flag.Int("n", 20, "number of players to include")
	flag.Parse()

	config.Global.Validate()
	data.Init()

	ctx := context.Background()

	// Fetch top players
	players, err := data.ReadTopPlayers(ctx, *limit, data.TopCategory["score"])
	if err != nil {
		log.Fatalf("failed to read top players: %v", err)
	}

	// Get avatars for top 3
	podiumIDs := make([]string, 0, 3)
	for i := 0; i < 3 && i < len(players); i++ {
		podiumIDs = append(podiumIDs, players[i].PlayerID)
	}
	scribeClient := scribe.NewClient()
	avatarCache := img.NewAvatarCache(scribeClient)
	avatars := avatarCache.GetMany(ctx, podiumIDs)

	// Get rank tiers from database
	allTiers, err := data.ReadRankTiers(ctx)
	if err != nil {
		log.Fatalf("failed to read rank tiers: %v", err)
	}

	tierMap := make(map[string]data.RankTier)
	for _, p := range players {
		for _, tier := range allTiers {
			if p.Score >= tier.ScoreGate {
				tierMap[p.PlayerID] = tier
			}
		}
	}

	// Load rank icons
	rankIconCache := img.NewRankIconCache()

	// Test harness: hardcode 2nd, 4th, 6th placements as decaying so we can
	// preview the ▼ indicator without trusting last_match_played_at yet.
	decayingIDs := make(map[string]bool)
	for _, pos := range []int{1, 3, 5} {
		if pos < len(players) {
			decayingIDs[players[pos].PlayerID] = true
		}
	}

	// Test harness: give top 3 high K/D/A to preview human-readable formatting.
	for i := 0; i < 3 && i < len(players); i++ {
		players[i].Kills = 1200 + i*300
		players[i].Deaths = 1050 + i*200
		players[i].Assists = 1100 + i*150
	}

	// Render leaderboard
	imgReader, err := img.RenderLeaderboardImage(players, avatars, tierMap, rankIconCache, decayingIDs)
	if err != nil {
		log.Fatalf("failed to render leaderboard: %v", err)
	}

	// Write to file
	outputFile, err := os.Create(*outputPath)
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	_, err = outputFile.ReadFrom(imgReader)
	if err != nil {
		log.Fatalf("failed to write image: %v", err)
	}

	fmt.Printf("✓ Leaderboard image saved to %s\n", *outputPath)
}
