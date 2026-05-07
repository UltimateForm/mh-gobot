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

		embed = &discordgo.MessageEmbed{
			Title:       "🏆 Score",
			Description: fmt.Sprintf("## 🎖️ %s - [%s](https://mordhau-scribe.com/player/%s)\n%s pts%s", rankName, player.Username, player.PlayerID, util.HumanFormat(player.Score), placementStr),
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

	topStr := "—"
	if len(topPlayers) > 0 && topPlayers[0].Score > 0 {
		topStr = fmt.Sprintf("**[%s](https://mordhau-scribe.com/player/%s)** — %s pts", topPlayers[0].Username, topPlayers[0].PlayerID, util.HumanFormat(topPlayers[0].Score))
	}
	bottomStr := "—"
	if bottomPlayer != nil {
		bottomStr = fmt.Sprintf("**[%s](https://mordhau-scribe.com/player/%s)** — %s pts", bottomPlayer.Username, bottomPlayer.PlayerID, util.HumanFormat(bottomPlayer.Score))
	}

	k := weightProvider.K()

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
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
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
})
