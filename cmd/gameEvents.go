package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/UltimateForm/ryard/internal/config"
	"github.com/UltimateForm/ryard/internal/parse"
	"github.com/UltimateForm/ryard/internal/rcon_client"
	"github.com/bwmarrin/discordgo"
)

func sendEvent(dc *discordgo.Session, embed *discordgo.MessageEmbed) {
	_, err := dc.ChannelMessageSendEmbed(config.Global.EventsChannel, embed)
	if err != nil {
		log.Printf("failed to send event embed: %v", err)
	}
}

func killfeedEmbed(e *parse.KillfeedEvent) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "💀 Kill",
		Color: 0xE74C3C,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Killer", Value: fmt.Sprintf("%s `%s`", e.UserName, e.KillerID), Inline: true},
			{Name: "Killed", Value: fmt.Sprintf("%s `%s`", e.KilledUserName, e.KilledID), Inline: true},
			{Name: "Time", Value: e.Date, Inline: false},
		},
	}
}

func scorefeedPlayerEmbed(e *parse.ScorefeedPlayerEvent) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "📈 Score Update",
		Color: 0xF1C40F,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Player", Value: fmt.Sprintf("%s `%s`", e.UserName, e.PlayerID), Inline: false},
			{Name: "Change", Value: fmt.Sprintf("+%.0f", e.ScoreChange), Inline: true},
			{Name: "Total", Value: fmt.Sprintf("%.0f", e.NewScore), Inline: true},
		},
	}
}

func scorefeedTeamEmbed(e *parse.ScorefeedTeamEvent) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "🏆 Team Score Update",
		Color: 0x3498DB,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Team", Value: fmt.Sprintf("%d", e.TeamID), Inline: false},
			{Name: "Previous", Value: fmt.Sprintf("%.0f", e.OldScore), Inline: true},
			{Name: "Now", Value: fmt.Sprintf("%.0f", e.NewScore), Inline: true},
		},
	}
}

func matchstateEmbed(state string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "🎮 Match State",
		Color:       0x9B59B6,
		Description: state,
	}
}

func chatEmbed(e *parse.ChatEvent) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "💬 Chat",
		Color: 0x57F287,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Player", Value: fmt.Sprintf("%s `%s`", e.UserName, e.PlayerID), Inline: true},
			{Name: "Channel", Value: e.Channel, Inline: true},
			{Name: "Message", Value: e.Message, Inline: false},
		},
	}
}

func handleEvents(ctx context.Context, dc *discordgo.Session, listener *rcon_client.ListenerClient) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-listener.ScorefeedPlayerEvents:
			sendEvent(dc, scorefeedPlayerEmbed(e))
		case e := <-listener.ScorefeedTeamEvents:
			sendEvent(dc, scorefeedTeamEmbed(e))
		case e := <-listener.KillfeedEvents:
			sendEvent(dc, killfeedEmbed(e))
		case state := <-listener.MatchstateEvents:
			sendEvent(dc, matchstateEmbed(state))
		case e := <-listener.ChatEvents:
			sendEvent(dc, chatEmbed(e))
		}
	}
}
