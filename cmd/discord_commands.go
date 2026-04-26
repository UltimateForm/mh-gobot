package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/discord"
	"github.com/UltimateForm/mh-gobot/internal/rcon_client"
	"github.com/UltimateForm/mh-gobot/internal/scribe"
	"github.com/UltimateForm/mh-gobot/internal/util"
	"github.com/bwmarrin/discordgo"
	"github.com/jedib0t/go-pretty/v6/table"
)

var scribeClient = scribe.NewClient()

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
		scribeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		scribePlayer, _ := scribeClient.GetPlayer(scribeCtx, player.PlayerID)
		cancel()

		matchesPlayed, _ := data.CountMatchesForPlayer(context.Background(), player.PlayerID)
		matchesWon, _ := data.CountMatchesWonByPlayer(context.Background(), player.PlayerID)

		embed = &discordgo.MessageEmbed{
			Title:       "🏆 Score",
			URL:         fmt.Sprintf("https://mordhau-scribe.com/player/%s", player.PlayerID),
			Description: fmt.Sprintf("```ansi\n%s — \u001b[33;1m%s pts\u001b[0m\n```", player.Username, util.HumanFormat(player.Score)),
			Color:       0xF1C40F,
			Fields: []*discordgo.MessageEmbedField{
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
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{{Title: "Error", Description: "Unknown category", Color: 0xFF0000}},
		})
		return
	}

	players, err := data.ReadTopPlayers(context.Background(), 10, column)

	var embed *discordgo.MessageEmbed
	if err != nil {
		log.Printf("top command error: %v", err)
		embed = errorEmbed("")
	} else {
		tw := table.NewWriter()
		tw.AppendHeader(table.Row{"#", "Player", "Score", "K", "D", "A"})
		for _, p := range players {
			tw.AppendRow(table.Row{p.Rank, p.Username, util.HumanFormat(p.Score), p.Kills, p.Deaths, p.Assists})
		}
		tw.SetStyle(table.StyleLight)
		tw.Style().Options.DrawBorder = false
		tw.Style().Options.SeparateRows = false
		embed = &discordgo.MessageEmbed{
			Title: fmt.Sprintf("🏆 Top 10 — %s", category),
			Color: 0xF1C40F,
			Fields: []*discordgo.MessageEmbedField{
				{Value: util.TruncateCodeString(fmt.Sprintf("```\n%s\n```", tw.Render()), 1024)},
			},
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func ledgerListEmbed(title string, subject *data.Player, entries []data.LedgerEntry) *discordgo.MessageEmbed {
	if len(entries) == 0 {
		return &discordgo.MessageEmbed{
			Title:       title,
			Description: fmt.Sprintf("No data found for **%s**", subject.Username),
			Color:       0x95A5A6,
		}
	}
	tw := table.NewWriter()
	tw.AppendHeader(table.Row{"#", "Player", "Kills"})
	for i, e := range entries {
		tw.AppendRow(table.Row{i + 1, e.Username, e.Count})
	}
	tw.SetStyle(table.StyleLight)
	tw.Style().Options.DrawBorder = false
	tw.Style().Options.SeparateRows = false
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: fmt.Sprintf("```\n%s\n```", tw.Render()),
		Color:       0xE74C3C,
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Player ID: %s", subject.PlayerID)},
	}
}

func handleNemesisCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query := i.ApplicationCommandData().Options[0].StringValue()
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	player, err := resolvePlayer(query)
	if err != nil {
		var embed *discordgo.MessageEmbed
		if errors.Is(err, data.DbPlayerNotFound) {
			embed = notFoundEmbed(query)
		} else {
			log.Printf("nemesis command error: %v", err)
			embed = errorEmbed("")
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}
	entries, err := data.ReadTopKillersOf(context.Background(), player.PlayerID, 10)
	if err != nil {
		log.Printf("nemesis command read error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{{Title: "Error", Color: 0xFF0000}}})
		return
	}
	embed := ledgerListEmbed(fmt.Sprintf("☠️ Top killers of %s", player.Username), player, entries)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
}

func handlePreyCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query := i.ApplicationCommandData().Options[0].StringValue()
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	player, err := resolvePlayer(query)
	if err != nil {
		var embed *discordgo.MessageEmbed
		if errors.Is(err, data.DbPlayerNotFound) {
			embed = notFoundEmbed(query)
		} else {
			log.Printf("prey command error: %v", err)
			embed = errorEmbed("")
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}
	entries, err := data.ReadTopVictimsOf(context.Background(), player.PlayerID, 10)
	if err != nil {
		log.Printf("prey command read error: %v", err)
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{{Title: "Error", Color: 0xFF0000}}})
		return
	}
	embed := ledgerListEmbed(fmt.Sprintf("🎯 Top victims of %s", player.Username), player, entries)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
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
			{Name: ":vs:", Value: "\u200b", Inline: true},
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

	var embed *discordgo.MessageEmbed
	if errors.Is(err, data.DbPlayerNotFound) {
		embed = notFoundEmbed(query)
	} else if err != nil {
		log.Printf("place command error: %v", err)
		embed = errorEmbed("")
	} else {
		placement, err := data.ReadPlayerPlacement(context.Background(), player.PlayerID)
		if err != nil {
			log.Printf("place command placement error: %v", err)
			embed = errorEmbed("")
		} else {
			var sb strings.Builder
			for _, rp := range placement.Snippet {
				if rp.PlayerID == player.PlayerID {
					sb.WriteString(fmt.Sprintf("► #%-3d %-24s %s pts\n", rp.Rank, rp.Username, util.HumanFormat(rp.RawScore)))
				} else {
					sb.WriteString(fmt.Sprintf("  #%-3d %-24s %s pts\n", rp.Rank, rp.Username, util.HumanFormat(rp.RawScore)))
				}
			}
			embed = &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("📊 Placement — #%d", placement.Rank),
				Description: fmt.Sprintf("```\n%s```", sb.String()),
				Color:       0x3498DB,
				Footer: &discordgo.MessageEmbedFooter{
					Text: fmt.Sprintf("Player ID: %s", player.PlayerID),
				},
			}
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

var commandRegistry = discord.NewCommandRegistry([]discord.Command{
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
})
