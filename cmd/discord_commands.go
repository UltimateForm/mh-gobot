package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/UltimateForm/ryard/internal/config"
	"github.com/UltimateForm/ryard/internal/data"
	"github.com/UltimateForm/ryard/internal/discord"
	"github.com/UltimateForm/ryard/internal/rcon_client"
	"github.com/UltimateForm/ryard/internal/util"
	"github.com/bwmarrin/discordgo"
	"github.com/jedib0t/go-pretty/v6/table"
)


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
	client, err := rcon_client.New(config.Global.RconUri)
	if err != nil {
		return "", err
	}
	success, err := client.Authenticate(config.Global.RconPassword)
	if err != nil {
		return "", errors.Join(errors.New("authentication error"), err)
	}
	if !success {
		return "", errors.New("authentication failed")
	}
	return client.Execute(cmd)
}

func handleScoreCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	playerID := i.ApplicationCommandData().Options[0].StringValue()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	player, err := resolvePlayer(playerID)

	var embed *discordgo.MessageEmbed
	if errors.Is(err, data.DbPlayerNotFound) {
		embed = &discordgo.MessageEmbed{
			Title:       "Player Not Found",
			Description: fmt.Sprintf("No stats found for `%s`", playerID),
			Color:       0xFF0000,
		}
	} else if err != nil {
		log.Printf("stats command error: %v", err)
		embed = &discordgo.MessageEmbed{
			Title: "Error",
			Color: 0xFF0000,
		}
	} else {
		embed = &discordgo.MessageEmbed{
			Title:       "🏆 Lifetime Score",
			Description: fmt.Sprintf("```ansi\n%s — \u001b[33;1m%s pts\u001b[0m\n```", player.Username, util.HumanFormat(player.RawScore)), // TODO: switch to computed score once scoring logic is defined
			Color:       0xF1C40F,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "⚔️ Kills", Value: fmt.Sprintf("```ansi\n\u001b[31;1m%d\u001b[0m\n```", player.Kills), Inline: true},
				{Name: "🪦 Deaths", Value: fmt.Sprintf("```ansi\n\u001b[31m%d\u001b[0m\n```", player.Deaths), Inline: true},
				{Name: "🤝 Assists", Value: fmt.Sprintf("```ansi\n\u001b[36m%d\u001b[0m\n```", player.Assists), Inline: true},
			},
			Footer: &discordgo.MessageEmbedFooter{
				Text: fmt.Sprintf("Player ID: %s", player.PlayerID),
			},
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
		embed = &discordgo.MessageEmbed{Title: "Error", Color: 0xFF0000}
	} else {
		tw := table.NewWriter()
		tw.AppendHeader(table.Row{"#", "Player", "Score", "K", "D", "A"})
		for _, p := range players {
			tw.AppendRow(table.Row{p.Rank, p.Username, util.HumanFormat(p.RawScore), p.Kills, p.Deaths, p.Assists})
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
		embed = &discordgo.MessageEmbed{
			Title:       "Player Not Found",
			Description: fmt.Sprintf("No stats found for `%s`", query),
			Color:       0xFF0000,
		}
	} else if err != nil {
		log.Printf("place command error: %v", err)
		embed = &discordgo.MessageEmbed{Title: "Error", Color: 0xFF0000}
	} else {
		placement, err := data.ReadPlayerPlacement(context.Background(), player.PlayerID)
		if err != nil {
			log.Printf("place command placement error: %v", err)
			embed = &discordgo.MessageEmbed{Title: "Error", Color: 0xFF0000}
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
	discord.Command{
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
