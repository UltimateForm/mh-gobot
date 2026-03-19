package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/UltimateForm/mh-gobot/internal/config"
	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/UltimateForm/mh-gobot/internal/rcon_client"
	"github.com/bwmarrin/discordgo"
)

func logEvent(dc *discordgo.Session, msg string) {
	if config.Global.EventsChannel == "" {
		return
	}
	_, err := dc.ChannelMessageSend(config.Global.EventsChannel, msg)
	if err != nil {
		log.Printf("failed to send event: %v", err)
	}
}

func persistPlayer(p data.Player) {
	if err := data.UpsertPlayer(context.Background(), p); err != nil {
		log.Printf("failed to upsert player: %v", err)
	}
}

func killfeedMsg(e *parse.KillfeedEvent) string {
	if e.IsAssist {
		return fmt.Sprintf("🤝 **assist** `%s` (%s) → `%s` (%s)", e.UserName, e.KillerID, e.KilledUserName, e.KilledID)
	}
	return fmt.Sprintf("💀 **kill** `%s` (%s) → `%s` (%s)", e.UserName, e.KillerID, e.KilledUserName, e.KilledID)
}

func scorefeedPlayerMsg(e *parse.ScorefeedPlayerEvent) string {
	return fmt.Sprintf("📈 **score** `%s` (%s) +%.0f → %.0f", e.UserName, e.PlayerID, e.ScoreChange, e.NewScore)
}

func scorefeedTeamMsg(e *parse.ScorefeedTeamEvent) string {
	return fmt.Sprintf("🏆 **team %d** %.0f → %.0f", e.TeamID, e.OldScore, e.NewScore)
}

func matchstateMsg(state string) string {
	return fmt.Sprintf("🎮 **matchstate** `%s`", state)
}

func chatMsg(e *parse.ChatEvent) string {
	return fmt.Sprintf("💬 **[%s]** `%s` (%s): %s", e.Channel, e.UserName, e.PlayerID, e.Message)
}

func handleEvents(ctx context.Context, dc *discordgo.Session, listener *rcon_client.ListenerClient) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-listener.ScorefeedPlayerEvents:
			go logEvent(dc, scorefeedPlayerMsg(e))
			go persistPlayer(parse.MapPlayerScore(*e))
		case e := <-listener.ScorefeedTeamEvents:
			go logEvent(dc, scorefeedTeamMsg(e))
		case e := <-listener.KillfeedEvents:
			go logEvent(dc, killfeedMsg(e))
			go persistPlayer(parse.MapKilledFromKillfeed(*e))
			go persistPlayer(parse.MapKillerFromKillfeed(*e))
		case state := <-listener.MatchstateEvents:
			go logEvent(dc, matchstateMsg(state))
		case e := <-listener.ChatEvents:
			go logEvent(dc, chatMsg(e))
		}
	}
}
