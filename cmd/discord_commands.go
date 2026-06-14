package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/config"
	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/discord"
	"github.com/UltimateForm/mh-gobot/internal/game"
	"github.com/UltimateForm/mh-gobot/internal/img"
	"github.com/UltimateForm/mh-gobot/internal/rcon_client"
	"github.com/UltimateForm/mh-gobot/internal/scribe"
	"github.com/UltimateForm/mh-gobot/internal/util"
	"github.com/bwmarrin/discordgo"
	"github.com/jedib0t/go-pretty/v6/table"
)

var scribeClient = scribe.NewClient()
var avatarCache = img.NewAvatarCache(scribeClient)
var rankTierProvider = game.NewRankTierProvider()
var rankIconCache *img.RankIconCache
var weightProvider = game.NewScoreWeightProvider()
var gameConfig = game.NewGameConfig()

func errorEmbed(msg string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{Title: "Error", Description: msg, Color: 0xFF0000}
}

func notFoundEmbed(query string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "Player Not Found",
		Description: fmt.Sprintf("No stats found for `%s`", query),
		Color:       0xFF0000,
	}
}

func handleRconxCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !config.Global.Debug {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "this command is disabled",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	options := i.ApplicationCommandData().Options
	var cmdString string
	for _, opt := range options {
		if opt.Name == "command" {
			cmdString = opt.StringValue()
			break
		}
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error deferring interaction: %v", err)
		return
	}

	result, err := executeRconCommand(cmdString)

	var embed *discordgo.MessageEmbed
	if err != nil {
		embed = &discordgo.MessageEmbed{
			Title:       "RCON Error ❌",
			Description: fmt.Sprintf("Failed to execute command: `%s`", cmdString),
			Color:       0xff0000,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Error",
					Value: util.TruncateCodeString(fmt.Sprintf("```\n%v\n```", err), 1024),
				},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
	} else {
		outputValue := result
		if outputValue == "" {
			outputValue = "(no output)"
		}
		embed = &discordgo.MessageEmbed{
			Title:       "RCON Response ✅",
			Description: fmt.Sprintf("Command: `%s`", cmdString),
			Color:       0x00ff00,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Output",
					Value: util.TruncateCodeString(fmt.Sprintf("```\n%s\n```", outputValue), 1024),
				},
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
	}

	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		log.Printf("Error editing interaction response: %v", err)
	}
}

func executeRconCommand(cmd string) (string, error) {
	var result string
	err := rconPool.WithClient(context.Background(), func(client *rcon_client.ControlledClient) error {
		var err error
		result, err = client.Execute(cmd)
		return err
	})
	return result, err
}

func handleScoreCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	playerID := i.ApplicationCommandData().Options[0].StringValue()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	player, err := resolvePlayer(playerID)

	var embed *discordgo.MessageEmbed
	if errors.Is(err, data.DbPlayerNotFound) {
		embed = notFoundEmbed(playerID)
	} else if err != nil {
		log.Printf("stats command error: %v", err)
		embed = &discordgo.MessageEmbed{
			Title: "Error",
			Color: 0xFF0000,
		}
	} else {
		scribeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		scribePlayer, _ := scribeClient.GetPlayer(scribeCtx, player.PlayerID)
		cancel()

		matchesPlayed, _ := data.CountMatchesForPlayer(context.Background(), player.PlayerID)
		matchesWon, _ := data.CountMatchesWonByPlayer(context.Background(), player.PlayerID)

		placement, _ := data.ReadPlayerPlacement(context.Background(), player.PlayerID)

		currentTier, hasCurrent := rankTierProvider.Current(player.Score)
		nextTier, hasNext := rankTierProvider.Next(player.Score)
		rankName := "Unranked"
		if hasCurrent {
			rankName = currentTier.Name
		}
		nextValue := "[none]"
		if hasNext {
			nextValue = fmt.Sprintf("%s (%s pts)", nextTier.Name, util.HumanFormat(nextTier.ScoreGate-player.Score))
		}

		placementStr := ""
		if placement != nil {
			placementStr = fmt.Sprintf(" - Placed #%d", placement.Rank)
		}

		decayStr := ""
		if topN, terr := computeTopN(context.Background(), gameConfig); terr == nil && topN > 0 {
			if cutoff, cerr := data.ReadDecayCutoffScore(context.Background(), topN); cerr == nil {
				if game.IsDecaying(*player, gameConfig, cutoff, time.Now()) {
					decayStr = "\n🩸 *Decaying — play a match to preserve*"
				}
			}
		}

		embed = &discordgo.MessageEmbed{
			Title:       "🏆 Score",
			Description: fmt.Sprintf("## 🎖️ %s - [%s](https://mordhau-scribe.com/player/%s)\n%s pts%s%s", rankName, player.Username, player.PlayerID, util.HumanFormat(player.Score), placementStr, decayStr),
			Color:       0xF1C40F,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "📈 Next Rank", Value: fmt.Sprintf("```\n%s\n```", nextValue), Inline: false},
				{Name: "⚔️ Kills", Value: fmt.Sprintf("```ansi\n\u001b[31;1m%d\u001b[0m\n```", player.Kills), Inline: true},
				{Name: "🪦 Deaths", Value: fmt.Sprintf("```ansi\n\u001b[31m%d\u001b[0m\n```", player.Deaths), Inline: true},
				{Name: "🤝 Assists", Value: fmt.Sprintf("```ansi\n\u001b[36m%d\u001b[0m\n```", player.Assists), Inline: true},
				{Name: "🏅 Matches Won", Value: fmt.Sprintf("```ansi\n\u001b[32;1m%d\u001b[0m\n```", matchesWon), Inline: true},
				{Name: "📊 Matches Played", Value: fmt.Sprintf("```\n%d\n```", matchesPlayed), Inline: true},
			},
			Footer: &discordgo.MessageEmbedFooter{
				Text: fmt.Sprintf("Player ID: %s", player.PlayerID),
			},
		}
		if scribePlayer != nil && scribePlayer.AvatarURL != "" {
			embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: scribePlayer.AvatarURL}
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func handleTopCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	category := i.ApplicationCommandData().Options[0].StringValue()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	column, ok := data.TopCategory[category]
	if !ok {
		content := "❌ Unknown category"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &content,
		})
		return
	}

	players, err := data.ReadTopPlayers(context.Background(), 10, column)

	var content string
	if err != nil {
		log.Printf("top command error: %v", err)
		content = "❌ Error"
	} else {
		tw := table.NewWriter()
		tw.AppendHeader(table.Row{"#", "Player", "Score", "K", "D", "A"})
		for _, p := range players {
			tw.AppendRow(table.Row{p.Rank, p.Username, util.HumanFormat(p.Score), p.Kills, p.Deaths, p.Assists})
		}
		tw.SetStyle(table.StyleLight)
		tw.Style().Options.DrawBorder = false
		tw.Style().Options.SeparateRows = false
		content = fmt.Sprintf("🏆 **Top 10 - %s**\n%s", category, util.TruncateCodeString(fmt.Sprintf("```\n%s\n```", tw.Render()), 1024))
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
}

func ledgerListMessage(title string, entries []data.LedgerEntry) string {
	if len(entries) == 0 {
		return fmt.Sprintf("**%s**\nNo data found", title)
	}
	tw := table.NewWriter()
	tw.AppendHeader(table.Row{"#", "Player", "Kills"})
	for i, e := range entries {
		tw.AppendRow(table.Row{i + 1, e.Username, e.Count})
	}
	tw.SetStyle(table.StyleLight)
	tw.Style().Options.DrawBorder = false
	tw.Style().Options.SeparateRows = false
	return fmt.Sprintf("**%s**\n```\n%s\n```", title, tw.Render())
}

func handleNemesisCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query := i.ApplicationCommandData().Options[0].StringValue()
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	player, err := resolvePlayer(query)
	if err != nil {
		var content string
		if errors.Is(err, data.DbPlayerNotFound) {
			content = fmt.Sprintf("❌ Player not found: `%s`", query)
		} else {
			log.Printf("nemesis command error: %v", err)
			content = "❌ Error"
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}
	entries, err := data.ReadTopKillersOf(context.Background(), player.PlayerID, 10)
	if err != nil {
		log.Printf("nemesis command read error: %v", err)
		content := "❌ Error"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}
	content := ledgerListMessage(fmt.Sprintf("☠️ Top killers of %s", player.Username), entries)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
}

func handlePreyCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query := i.ApplicationCommandData().Options[0].StringValue()
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	player, err := resolvePlayer(query)
	if err != nil {
		var content string
		if errors.Is(err, data.DbPlayerNotFound) {
			content = fmt.Sprintf("❌ Player not found: `%s`", query)
		} else {
			log.Printf("prey command error: %v", err)
			content = "❌ Error"
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}
	entries, err := data.ReadTopVictimsOf(context.Background(), player.PlayerID, 10)
	if err != nil {
		log.Printf("prey command read error: %v", err)
		content := "❌ Error"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}
	content := ledgerListMessage(fmt.Sprintf("🎯 Top victims of %s", player.Username), entries)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
}

func handleVersusCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	player1Query := options[0].StringValue()
	player2Query := options[1].StringValue()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	p1, err := resolvePlayer(player1Query)
	if err != nil {
		var embed *discordgo.MessageEmbed
		if errors.Is(err, data.DbPlayerNotFound) {
			embed = notFoundEmbed(player1Query)
		} else {
			log.Printf("versus command p1 error: %v", err)
			embed = errorEmbed("")
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}

	p2, err := resolvePlayer(player2Query)
	if err != nil {
		var embed *discordgo.MessageEmbed
		if errors.Is(err, data.DbPlayerNotFound) {
			embed = notFoundEmbed(player2Query)
		} else {
			log.Printf("versus command p2 error: %v", err)
			embed = errorEmbed("")
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}

	versus, err := data.ReadVersus(context.Background(), p1.PlayerID, p2.PlayerID)
	if err != nil {
		log.Printf("versus command read error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{{Title: "Error", Color: 0xFF0000}}})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "⚔️ Versus",
		Color: 0xE74C3C,
		Fields: []*discordgo.MessageEmbedField{
			{Name: p1.Username, Value: fmt.Sprintf("```ansi\n\u001b[31;1m%d kills\u001b[0m\n```", versus.AKills), Inline: true},
			{Name: p2.Username, Value: fmt.Sprintf("```ansi\n\u001b[31;1m%d kills\u001b[0m\n```", versus.BKills), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%s vs %s", p1.PlayerID, p2.PlayerID),
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

func resolvePlayer(query string) (*data.Player, error) {
	if util.IsPlayfabID(query) {
		return data.ReadPlayer(context.Background(), query)
	}
	return data.ReadPlayerByName(context.Background(), query)
}

func handlePlaceCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query := i.ApplicationCommandData().Options[0].StringValue()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	player, err := resolvePlayer(query)

	var content string
	if errors.Is(err, data.DbPlayerNotFound) {
		content = fmt.Sprintf("❌ Player Not Found\nNo stats found for `%s`", query)
	} else if err != nil {
		log.Printf("place command error: %v", err)
		content = "❌ Error"
	} else {
		placement, err := data.ReadPlayerPlacement(context.Background(), player.PlayerID)
		if err != nil {
			log.Printf("place command placement error: %v", err)
			content = "❌ Error"
		} else {
			var sb strings.Builder
			for _, rp := range placement.Snippet {
				if rp.PlayerID == player.PlayerID {
					sb.WriteString(fmt.Sprintf("► #%-3d %-24s %s pts\n", rp.Rank, rp.Username, util.HumanFormat(rp.Score)))
				} else {
					sb.WriteString(fmt.Sprintf("  #%-3d %-24s %s pts\n", rp.Rank, rp.Username, util.HumanFormat(rp.Score)))
				}
			}
			content = fmt.Sprintf("📊 **Placement - #%d**\n```\n%s```\nPlayer: [%s](https://mordhau-scribe.com/player/%s)", placement.Rank, sb.String(), player.PlayerID, player.PlayerID)
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
}

func handleSetRankCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	var scoreGate int64
	var name, shortName string
	for _, opt := range options {
		switch opt.Name {
		case "score_gate":
			scoreGate = opt.IntValue()
		case "name":
			name = opt.StringValue()
		case "short_name":
			shortName = opt.StringValue()
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	ctx := context.Background()
	tier := data.RankTier{ScoreGate: int(scoreGate), Name: name, ShortName: shortName}
	if err := data.UpsertRankTier(ctx, tier); err != nil {
		log.Printf("set_rank error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{errorEmbed("Failed to save rank tier")},
		})
		return
	}
	rankTierProvider.Refresh(ctx)

	desc := fmt.Sprintf("**%s** at score gate **%d**", name, scoreGate)
	if shortName != "" {
		desc += fmt.Sprintf(" (short: `%s`)", shortName)
	}
	embed := &discordgo.MessageEmbed{
		Title:       "🎖️ Rank tier saved",
		Description: desc,
		Color:       0x57F287,
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func handleDelRankCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	scoreGate := i.ApplicationCommandData().Options[0].IntValue()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	ctx := context.Background()
	deleted, err := data.DeleteRankTier(ctx, int(scoreGate))
	if err != nil {
		log.Printf("del_rank error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{errorEmbed("Failed to delete rank tier")},
		})
		return
	}
	rankTierProvider.Refresh(ctx)

	var embed *discordgo.MessageEmbed
	if deleted {
		embed = &discordgo.MessageEmbed{
			Title:       "🗑️ Rank tier deleted",
			Description: fmt.Sprintf("Removed gate **%d**", scoreGate),
			Color:       0x57F287,
		}
	} else {
		embed = &discordgo.MessageEmbed{
			Title:       "Not Found",
			Description: fmt.Sprintf("No rank tier at gate **%d**", scoreGate),
			Color:       0x95A5A6,
		}
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func handleKCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	weightProvider.Refresh(context.Background())
	avg := weightProvider.AvgScore()
	floor := weightProvider.Floor()
	k := weightProvider.K()

	embed := &discordgo.MessageEmbed{
		Title: "⚖️ Score Weight K",
		Color: 0x3498DB,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "K", Value: fmt.Sprintf("```\n%.0f\n```", k), Inline: true},
			{Name: "Avg Score", Value: fmt.Sprintf("```\n%.0f\n```", avg), Inline: true},
			{Name: "Floor", Value: fmt.Sprintf("```\n%.0f\n```", floor), Inline: true},
		},
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func handleSimLossCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	var query string
	sizeFactor := 1.0
	for _, opt := range options {
		switch opt.Name {
		case "player":
			query = opt.StringValue()
		case "size_factor":
			sizeFactor = opt.FloatValue()
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	if sizeFactor < 1.0 {
		sizeFactor = 1.0
	}

	player, err := resolvePlayer(query)
	var embed *discordgo.MessageEmbed
	if errors.Is(err, data.DbPlayerNotFound) {
		embed = notFoundEmbed(query)
	} else if err != nil {
		log.Printf("sim_loss command error: %v", err)
		embed = errorEmbed("")
	} else {
		k := weightProvider.K()
		lossRatio := gameConfig.Get(game.CfgMatchLossRatio)
		lossFactorCap := gameConfig.Get(game.CfgMatchLossFactorCap)
		calc := game.ComputeMatchLoss(
			player.PlayerID,
			player.Score,
			k,
			sizeFactor,
			lossRatio,
			lossFactorCap,
		)
		const baseKillScore = 100
		minKillsStr := "0"
		if calc.ActualLoss > 0 {
			minKills := int(math.Ceil(float64(calc.ActualLoss) / baseKillScore))
			minKillsStr = fmt.Sprintf("%d", minKills)
		}
		ctx := context.Background()
		rank, _ := data.ReadPlayerRank(ctx, player.PlayerID)
		agg, _ := data.ReadAggregates(ctx)
		placementStr := "?"
		if rank > 0 && agg != nil && agg.TotalPlayers > 0 {
			placementStr = fmt.Sprintf("%d/%d", rank, agg.TotalPlayers)
		}
		embed = &discordgo.MessageEmbed{
			Title: "🔮 Match Loss Simulation",
			Description: fmt.Sprintf("**%s** - %s pts (placement %s)\n**K:** %.0f | **ratio:** %.2f | **max factor:** %.2f | **size÷:** %.2f\n**base kill:** %d (flat, bonuses extra)",
				player.Username, util.HumanFormat(player.Score), placementStr, k, lossRatio, lossFactorCap, sizeFactor,
				baseKillScore),
			Color: 0xE67E22,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Base", Value: fmt.Sprintf("```\n%d\n```", calc.BaseAmount), Inline: true},
				{Name: "Factor", Value: fmt.Sprintf("```\n%.2f\n```", calc.LossFactor), Inline: true},
				{Name: "Raw Loss", Value: fmt.Sprintf("```\n%d\n```", calc.RawLoss), Inline: true},
				{Name: "Actual Loss", Value: fmt.Sprintf("```ansi\n[31;1m-%d[0m\n```", calc.ActualLoss), Inline: true},
				{Name: "Score After", Value: fmt.Sprintf("```\n%s\n```", util.HumanFormat(player.Score-calc.ActualLoss)), Inline: true},
				{Name: "Min Kills to Cancel", Value: fmt.Sprintf("```\n%s\n```", minKillsStr), Inline: true},
			},
			Footer: &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Player ID: %s", player.PlayerID)},
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func handleRanksCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	tiers := rankTierProvider.All()
	var content string
	if len(tiers) == 0 {
		content = "🎖️ No rank tiers configured."
	} else {
		tw := table.NewWriter()
		tw.AppendHeader(table.Row{"Score", "Rank", "Short"})
		for _, t := range tiers {
			tw.AppendRow(table.Row{util.HumanFormat(t.ScoreGate), t.Name, t.ShortName})
		}
		tw.SetStyle(table.StyleLight)
		tw.Style().Options.DrawBorder = false
		tw.Style().Options.SeparateRows = false
		content = fmt.Sprintf("🎖️ **Rank Tiers**\n```\n%s\n```", tw.Render())
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
}

var configKeys = []string{
	game.CfgSkirmishRoundWinMod,
	game.CfgSkirmishSizeFactorCap,
	game.CfgSkirmishWinCap,
	game.CfgMatchLossRatio,
	game.CfgMatchLossFactorCap,
	game.CfgStartingPoints,
	game.CfgQuitterPenaltyTeamMin,
	game.CfgDecayEnabled,
	game.CfgDecayGraceDays,
	game.CfgDecayPctPerDay,
	game.CfgDecayTopPct,
	game.CfgTeamBalanceMinFactor,
	game.CfgTeamBalanceMaxFactor,
	game.CfgFirstKillBonusFactor,
}

func handleTunersGetCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	fields := make([]*discordgo.MessageEmbedField, 0, len(configKeys))
	for _, k := range configKeys {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  fmt.Sprintf("`%s` = %.4f", k, gameConfig.Get(k)),
			Value: game.GameConfigDescriptions[k],
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:  "⚙️ Game Tuners",
		Color:  0x3498DB,
		Fields: fields,
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func handleTunersSetCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	var key string
	var value float64
	for _, opt := range options {
		switch opt.Name {
		case "key":
			key = opt.StringValue()
		case "value":
			value = opt.FloatValue()
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	oldValue := gameConfig.Get(key)
	if err := gameConfig.Set(context.Background(), key, value); err != nil {
		log.Printf("tuners_set error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{errorEmbed(err.Error())},
		})
		return
	}

	desc := fmt.Sprintf("**%s**: `%.4f` → `%.4f`\n\n%s", key, oldValue, value, game.GameConfigDescriptions[key])
	if key == game.CfgSkirmishWinCap {
		desc += "\n\n_Note: takes effect on next bot restart._"
	}
	embed := &discordgo.MessageEmbed{
		Title:       "⚙️ Tuner Updated",
		Description: desc,
		Color:       0x57F287,
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func handleStatsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	ctx := context.Background()
	agg, err := data.ReadAggregates(ctx)
	if err != nil {
		log.Printf("stats: read aggregates: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{errorEmbed("failed to read aggregates")}})
		return
	}

	totalMatches, err := data.CountAllMatches(ctx)
	if err != nil {
		log.Printf("stats: count matches: %v", err)
	}

	topPlayers, _ := data.ReadTopPlayers(ctx, 1, data.TopCategory["score"])
	bottomPlayer, _ := data.ReadBottomPlayer(ctx)

	topStr := "-"
	if len(topPlayers) > 0 && topPlayers[0].Score > 0 {
		topStr = fmt.Sprintf("**[%s](https://mordhau-scribe.com/player/%s)** - %s pts", topPlayers[0].Username, topPlayers[0].PlayerID, util.HumanFormat(topPlayers[0].Score))
	}
	bottomStr := "-"
	if bottomPlayer != nil {
		bottomStr = fmt.Sprintf("**[%s](https://mordhau-scribe.com/player/%s)** - %s pts", bottomPlayer.Username, bottomPlayer.PlayerID, util.HumanFormat(bottomPlayer.Score))
	}

	k := weightProvider.K()

	decayCutoffStr := "`off`"
	if gameConfig.Get(game.CfgDecayEnabled) != 0 {
		decayCutoffStr = "`-`"
		if topN, terr := computeTopN(ctx, gameConfig); terr == nil && topN > 0 {
			if cutoff, cerr := data.ReadDecayCutoffScore(ctx, topN); cerr == nil && cutoff > 0 {
				decayCutoffStr = fmt.Sprintf("**%s pts**", util.HumanFormat(cutoff))
			}
		}
	}

	embed := &discordgo.MessageEmbed{
		Title: "📊 Server Stats",
		Color: 0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "👥 Total Players", Value: fmt.Sprintf("```ansi\n\u001b[34;1m%d\u001b[0m\n```", agg.TotalPlayers), Inline: true},
			{Name: "🎮 Total Matches", Value: fmt.Sprintf("```ansi\n\u001b[35;1m%s\u001b[0m\n```", util.HumanFormat(totalMatches)), Inline: true},
			{Name: "📈 Avg Score (K)", Value: fmt.Sprintf("```ansi\n\u001b[33;1m%s\u001b[0m\n```", util.HumanFormat(int(k))), Inline: true},
			{Name: "⚔️ Total Kills", Value: fmt.Sprintf("```ansi\n\u001b[31;1m%s\u001b[0m\n```", util.HumanFormat(agg.TotalKills)), Inline: true},
			{Name: "🪦 Total Deaths", Value: fmt.Sprintf("```ansi\n\u001b[31m%s\u001b[0m\n```", util.HumanFormat(agg.TotalDeaths)), Inline: true},
			{Name: "🤝 Total Assists", Value: fmt.Sprintf("```ansi\n\u001b[36m%s\u001b[0m\n```", util.HumanFormat(agg.TotalAssists)), Inline: true},
			{Name: "🏆 Top Player", Value: topStr, Inline: true},
			{Name: "🥄 Bottom Player", Value: bottomStr, Inline: true},
			{Name: "🩸 Decay Threshold", Value: decayCutoffStr, Inline: true},
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func handleToggleRrCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	state := i.ApplicationCommandData().Options[0].StringValue()
	enabled := state == "on"
	value := 0.0
	if enabled {
		value = 1.0
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	if err := gameConfig.Set(context.Background(), game.CfgRrEnabled, value); err != nil {
		log.Printf("toggle_rr error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{errorEmbed(err.Error())},
		})
		return
	}

	content := "🔓 `!rr` command **enabled** - players can pause/resume their own ranking"
	if !enabled {
		content = "🔒 `!rr` command **disabled** - players can no longer pause/resume their own ranking"
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
}

func handleGetRrCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	players, err := data.ReadPausedPlayers(context.Background())
	if err != nil {
		log.Printf("get_rr error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{errorEmbed("failed to read paused players")},
		})
		return
	}

	var content string
	if len(players) == 0 {
		content = "No players currently have ranking paused."
	} else {
		var b strings.Builder
		fmt.Fprintf(&b, "**Players with ranking paused (%d):**\n", len(players))
		for _, p := range players {
			fmt.Fprintf(&b, "- %s (`%s`)\n", p.Username, p.PlayerID)
		}
		content = b.String()
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
}

func handleSetRrCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	var query, state string
	for _, opt := range options {
		switch opt.Name {
		case "player":
			query = opt.StringValue()
		case "state":
			state = opt.StringValue()
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	player, err := resolvePlayer(query)
	if errors.Is(err, data.DbPlayerNotFound) {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{notFoundEmbed(query)},
		})
		return
	}
	if err != nil {
		log.Printf("set_rr resolve error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{errorEmbed("failed to look up player")},
		})
		return
	}

	paused := state == "off"
	if err := data.SetScoringPaused(context.Background(), player.PlayerID, paused); err != nil {
		log.Printf("set_rr error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{errorEmbed(err.Error())},
		})
		return
	}

	verb := "resumed"
	if paused {
		verb = "paused"
	}
	content := fmt.Sprintf("✅ Ranking %s for **%s** (`%s`)", verb, player.Username, player.PlayerID)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
}

func handleDecayNowCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	dryRun := false
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "dry_run" {
			dryRun = opt.BoolValue()
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	ctx := context.Background()
	now := time.Now()

	if gameConfig.Get(game.CfgDecayEnabled) == 0 {
		content := "⚠️ Decay is currently disabled (`decay_enabled = 0`)"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}

	topN, err := computeTopN(ctx, gameConfig)
	if err != nil {
		log.Printf("decay_now: computeTopN error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{errorEmbed(err.Error())},
		})
		return
	}
	if topN == 0 {
		content := "No active players in the eligible top slice"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}

	graceDays := gameConfig.Get(game.CfgDecayGraceDays)
	pct := gameConfig.Get(game.CfgDecayPctPerDay)
	inactiveCutoff := now.Add(-time.Duration(graceDays * float64(24*time.Hour)))

	if dryRun {
		candidates, err := data.ReadDecayCandidates(ctx, topN, inactiveCutoff)
		if err != nil {
			log.Printf("decay_now dry_run error: %v", err)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Embeds: &[]*discordgo.MessageEmbed{errorEmbed(err.Error())},
			})
			return
		}
		var b strings.Builder
		fmt.Fprintf(&b, "**Dry run** — would decay %d/%d eligible top players by %.1f%% (inactive since <t:%d:R>):\n", len(candidates), topN, pct*100, inactiveCutoff.Unix())
		if len(candidates) == 0 {
			b.WriteString("_(no one currently meets the criteria)_")
		} else {
			for _, p := range candidates {
				lost := int(float64(p.Score) * pct)
				fmt.Fprintf(&b, "- %s (`%s`) — %s pts → %s pts (-%s)\n", p.Username, p.PlayerID, util.HumanFormat(p.Score), util.HumanFormat(p.Score-lost), util.HumanFormat(lost))
			}
		}
		content := b.String()
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}

	result, err := runDecayCore(ctx, gameConfig, now)
	if err != nil {
		log.Printf("decay_now error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{errorEmbed(err.Error())},
		})
		return
	}
	if result.Skipped != "" {
		content := fmt.Sprintf("Decay run skipped: %s", result.Skipped)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}
	content := fmt.Sprintf("🩸 Applied **%.1f%% decay** to **%d/%d** eligible top players", pct*100, result.RowsAffected, result.TopN)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
}

func handleRestartCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Restarting in 3 seconds...",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	go func() {
		time.Sleep(3 * time.Second)
		log.Println("restart requested via discord command")
		stopApp()
		rconPool.Close()
		os.Exit(1)
	}()
}

func handlePatchPlayerCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	var query string
	patch := data.PlayerPatch{}
	for _, opt := range options {
		switch opt.Name {
		case "player":
			query = opt.StringValue()
		case "username":
			v := opt.StringValue()
			patch.Username = &v
		case "score":
			v := int(opt.IntValue())
			patch.Score = &v
		case "kills":
			v := int(opt.IntValue())
			patch.Kills = &v
		case "deaths":
			v := int(opt.IntValue())
			patch.Deaths = &v
		case "assists":
			v := int(opt.IntValue())
			patch.Assists = &v
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	ctx := context.Background()
	player, err := resolvePlayer(query)
	if err != nil {
		var content string
		if errors.Is(err, data.DbPlayerNotFound) {
			content = fmt.Sprintf("❌ Player not found: `%s`", query)
		} else {
			log.Printf("patch_player resolve error: %v", err)
			content = "❌ Error"
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}

	if err := data.PatchPlayer(ctx, player.PlayerID, patch); err != nil {
		log.Printf("patch_player error: %v", err)
		content := "❌ Failed to patch player"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}

	updated, err := data.ReadPlayer(ctx, player.PlayerID)
	if err != nil {
		content := "✅ Patched (failed to read back)"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🔧 Player Patched",
		Description: fmt.Sprintf("[%s](https://mordhau-scribe.com/player/%s)", updated.PlayerID, updated.PlayerID),
		Color:       0x57F287,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Username", Value: fmt.Sprintf("`%s` → `%s`", player.Username, updated.Username), Inline: true},
			{Name: "Score", Value: fmt.Sprintf("`%d` → `%d`", player.Score, updated.Score), Inline: true},
			{Name: "Kills", Value: fmt.Sprintf("`%d` → `%d`", player.Kills, updated.Kills), Inline: true},
			{Name: "Deaths", Value: fmt.Sprintf("`%d` → `%d`", player.Deaths, updated.Deaths), Inline: true},
			{Name: "Assists", Value: fmt.Sprintf("`%d` → `%d`", player.Assists, updated.Assists), Inline: true},
		},
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

func handleReactionsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	link := i.ApplicationCommandData().Options[0].StringValue()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	parts := strings.Split(strings.TrimSpace(link), "/")
	if len(parts) < 2 {
		content := "❌ Invalid message link"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}
	messageID := parts[len(parts)-1]
	channelID := parts[len(parts)-2]

	msg, err := s.ChannelMessage(channelID, messageID)
	if err != nil {
		log.Printf("reactions command: fetch message: %v", err)
		content := "❌ Failed to fetch message"
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}

	if len(msg.Reactions) == 0 {
		content := "No reactions on that message."
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
		return
	}

	var sb strings.Builder
	for _, reaction := range msg.Reactions {
		users, err := s.MessageReactions(channelID, messageID, reaction.Emoji.APIName(), 100, "", "")
		if err != nil {
			log.Printf("reactions command: fetch users for %s: %v", reaction.Emoji.Name, err)
			continue
		}
		sb.WriteString(fmt.Sprintf("**%s** (%d)\n", reaction.Emoji.Name, reaction.Count))
		for _, u := range users {
			sb.WriteString(fmt.Sprintf("- %s\n", u.Username))
		}
	}

	body := sb.String()
	if body == "" {
		body = "No reactions found."
	}
	content := fmt.Sprintf("%s\n%s", link, body)
	if len(content) > 2000 {
		content = content[:2000]
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content})
}

var commandRegistry = discord.NewCommandRegistry([]discord.Command{
	{
		Definition: &discordgo.ApplicationCommand{
			Name:        "stats",
			Description: "Get general server stats",
		},
		Handler: handleStatsCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:        "score",
			Description: "Get lifetime score for a player",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "player",
					Description: "PlayFab ID or player name",
					Required:    true,
				},
			},
		},
		Handler: handleScoreCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:        "place",
			Description: "Get leaderboard placement for a player",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "player",
					Description: "PlayFab ID or player name",
					Required:    true,
				},
			},
		},
		Handler: handlePlaceCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:        "top",
			Description: "Get top 10 players by a stat",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "category",
					Description: "Stat to rank by",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Score", Value: "score"},
						{Name: "Kills", Value: "kills"},
						{Name: "Deaths", Value: "deaths"},
						{Name: "Assists", Value: "assists"},
					},
				},
			},
		},
		Handler: handleTopCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:        "nemesis",
			Description: "Top 10 players who killed a player the most",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "player", Description: "PlayFab ID or player name", Required: true},
			},
		},
		Handler: handleNemesisCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:        "prey",
			Description: "Top 10 players a player has killed the most",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "player", Description: "PlayFab ID or player name", Required: true},
			},
		},
		Handler: handlePreyCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:        "versus",
			Description: "Show kill tally between two players",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "player1",
					Description: "PlayFab ID or player name",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "player2",
					Description: "PlayFab ID or player name",
					Required:    true,
				},
			},
		},
		Handler: handleVersusCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "restart",
			Description:              "Gracefully restart the bot",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
		},
		Handler: handleRestartCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "decay_now",
			Description:              "Manually trigger a score decay tick (bypasses the 24h gap)",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionBoolean,
					Name:        "dry_run",
					Description: "If true, list who would be decayed without writing",
					Required:    false,
				},
			},
		},
		Handler: handleDecayNowCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "rconx",
			Description:              "Execute an RCON command",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "command",
					Description: "The RCON command to execute",
					Required:    true,
				},
			},
		},
		Handler: handleRconxCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "set_rank",
			Description:              "Set or update a rank tier at a score gate",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "score_gate",
					Description: "Minimum score required to reach this tier",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Rank name (e.g. Knight)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "short_name",
					Description: "Optional abbreviation",
					Required:    false,
				},
			},
		},
		Handler: handleSetRankCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "del_rank",
			Description:              "Delete a rank tier by its score gate",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "score_gate",
					Description: "Score gate of the tier to remove",
					Required:    true,
				},
			},
		},
		Handler: handleDelRankCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:        "ranks",
			Description: "List all configured rank tiers",
		},
		Handler: handleRanksCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "k",
			Description:              "Show current score weight K (avg score floored)",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
		},
		Handler: handleKCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "tuners_get",
			Description:              "Show current game tuner values with descriptions",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
		},
		Handler: handleTunersGetCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "tuners_set",
			Description:              "Update a game tuner value (live, persisted in DB)",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "key",
					Description: "Tuner key to update",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: game.CfgSkirmishRoundWinMod, Value: game.CfgSkirmishRoundWinMod},
						{Name: game.CfgSkirmishSizeFactorCap, Value: game.CfgSkirmishSizeFactorCap},
						{Name: game.CfgSkirmishWinCap, Value: game.CfgSkirmishWinCap},
						{Name: game.CfgMatchLossRatio, Value: game.CfgMatchLossRatio},
						{Name: game.CfgMatchLossFactorCap, Value: game.CfgMatchLossFactorCap},
						{Name: game.CfgStartingPoints, Value: game.CfgStartingPoints},
						{Name: game.CfgQuitterPenaltyTeamMin, Value: game.CfgQuitterPenaltyTeamMin},
						{Name: game.CfgDecayEnabled, Value: game.CfgDecayEnabled},
						{Name: game.CfgDecayGraceDays, Value: game.CfgDecayGraceDays},
						{Name: game.CfgDecayPctPerDay, Value: game.CfgDecayPctPerDay},
						{Name: game.CfgDecayTopPct, Value: game.CfgDecayTopPct},
						{Name: game.CfgTeamBalanceMinFactor, Value: game.CfgTeamBalanceMinFactor},
						{Name: game.CfgTeamBalanceMaxFactor, Value: game.CfgTeamBalanceMaxFactor},
						{Name: game.CfgFirstKillBonusFactor, Value: game.CfgFirstKillBonusFactor},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionNumber,
					Name:        "value",
					Description: "New numeric value",
					Required:    true,
				},
			},
		},
		Handler: handleTunersSetCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "sim_loss",
			Description:              "Simulate match loss for a player",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "player",
					Description: "PlayFab ID or player name",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionNumber,
					Name:        "size_factor",
					Description: "Team imbalance divisor (>=1.0, default 1.0)",
					Required:    false,
				},
			},
		},
		Handler: handleSimLossCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "toggle_rr",
			Description:              "Enable or disable players from using the in-game !rr command",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "state",
					Description: "on = players can use !rr, off = command is disabled",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "on", Value: "on"},
						{Name: "off", Value: "off"},
					},
				},
			},
		},
		Handler: handleToggleRrCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "get_rr",
			Description:              "List all players who currently have ranking paused",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
		},
		Handler: handleGetRrCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "set_rr",
			Description:              "Pause or resume ranking for a specific player (admin override)",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "player",
					Description: "PlayFab ID or player name",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "state",
					Description: "on = ranking active, off = ranking paused",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "on", Value: "on"},
						{Name: "off", Value: "off"},
					},
				},
			},
		},
		Handler: handleSetRrCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "patch_player",
			Description:              "Patch a player's stats in the database",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "player", Description: "PlayFab ID or player name", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "username", Description: "New username", Required: false},
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "score", Description: "New score", Required: false},
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "kills", Description: "New kills", Required: false},
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "deaths", Description: "New deaths", Required: false},
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "assists", Description: "New assists", Required: false},
			},
		},
		Handler: handlePatchPlayerCommand,
	},
	{
		Definition: &discordgo.ApplicationCommand{
			Name:                     "reactions",
			Description:              "List everyone who reacted to a message, grouped by emoji",
			DefaultMemberPermissions: &[]int64{discordgo.PermissionAdministrator}[0],
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "link", Description: "Discord message link", Required: true},
			},
		},
		Handler: handleReactionsCommand,
	},
})
