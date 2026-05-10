package cmd

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/UltimateForm/mh-gobot/internal/config"
	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/game"
	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/UltimateForm/mh-gobot/internal/rcon_client"
)

func rconSay(ctx context.Context, msg string) error {
	return rconPool.WithClient(ctx, func(c *rcon_client.ControlledClient) error {
		_, err := c.Execute("say " + msg)
		return err
	})
}

func handleScoreGameCommand(ctx context.Context, event *parse.ChatEvent, args []string) error {
	var player *data.Player
	var err error
	if len(args) == 0 {
		player, err = data.ReadPlayer(ctx, event.PlayerID)
	} else {
		player, err = resolvePlayer(args[0])
	}
	if errors.Is(err, data.DbPlayerNotFound) {
		return rconSay(ctx, "Score: player not found")
	}
	if err != nil {
		return err
	}
	rank, err := data.ReadPlayerRank(ctx, player.PlayerID)
	if err != nil {
		rank = 0
	}
	rankName := "Unranked"
	if tier, ok := rankTierProvider.Current(player.Score); ok {
		rankName = tier.Name
	}
	msg := fmt.Sprintf("%s - %s: Score %d | K %d | D %d | A %d",
		rankName,
		player.Username,
		player.Score,
		player.Kills,
		player.Deaths,
		player.Assists,
	)
	if rank > 0 {
		msg += fmt.Sprintf("\nPlaced #%d", rank)
	}
	return rconSay(ctx, msg)
}

func handleVersusGameCommand(ctx context.Context, event *parse.ChatEvent, args []string) error {
	if len(args) == 0 {
		return rconSay(ctx, "Versus: provide a player name or ID")
	}

	p2, err := resolvePlayer(args[0])
	if errors.Is(err, data.DbPlayerNotFound) {
		return rconSay(ctx, fmt.Sprintf("Versus: player '%s' not found", args[0]))
	}
	if err != nil {
		return err
	}

	versus, err := data.ReadVersus(ctx, event.PlayerID, p2.PlayerID)
	if err != nil {
		return err
	}

	return rconSay(ctx, fmt.Sprintf("%s %d - %d %s", event.UserName, versus.AKills, versus.BKills, p2.Username))
}

func handleRollGameCommand(ctx context.Context, event *parse.ChatEvent, args []string) error {
	n := rand.Intn(100)
	return rconSay(ctx, fmt.Sprintf("%s rolled a %02d", event.UserName, n))
}

func handleHelpGameCommand(ctx context.Context, event *parse.ChatEvent, args []string) error {
	return rconSay(ctx, "commands: !score, !roll, !versus (!vs), !rr, !help")
}

func handleRrGameCommand(ctx context.Context, event *parse.ChatEvent, args []string) error {
	if gameConfig.Get(game.CfgRrEnabled) == 0 {
		return rconSay(ctx, fmt.Sprintf("%s: !rr is currently disabled by the admin", event.UserName))
	}
	if len(args) == 0 {
		player, err := data.ReadPlayer(ctx, event.PlayerID)
		if errors.Is(err, data.DbPlayerNotFound) {
			return rconSay(ctx, fmt.Sprintf("%s: no score record yet | Usage: !rr <on|off>", event.UserName))
		}
		if err != nil {
			return err
		}
		status := "active"
		if player.ScoringPaused {
			status = "paused"
		}
		return rconSay(ctx, fmt.Sprintf("%s: scoring is %s | Usage: !rr <on|off>", event.UserName, status))
	}
	subCmd := strings.ToLower(args[0])
	if subCmd != "on" && subCmd != "off" {
		return rconSay(ctx, "Usage: !rr <on|off>")
	}
	paused := subCmd == "off"
	if err := data.SetScoringPaused(ctx, event.PlayerID, paused); err != nil {
		if errors.Is(err, data.DbPlayerNotFound) {
			return rconSay(ctx, fmt.Sprintf("%s: no score record yet - play a round first", event.UserName))
		}
		return err
	}
	if paused {
		return rconSay(ctx, fmt.Sprintf("%s: scoring paused - your rank is locked", event.UserName))
	}
	return rconSay(ctx, fmt.Sprintf("%s: scoring resumed - rank changes are active again", event.UserName))
}

var gameCommandRegistry = game.NewGameCommandRegistry(
	config.Global.GameCommandPrefix,
	[]game.GameCommand{
		{Name: "score", Handler: handleScoreGameCommand},
		{Name: "roll", Handler: handleRollGameCommand},
		{Name: "versus", Aliases: []string{"vs"}, Handler: handleVersusGameCommand},
		{Name: "rr", Handler: handleRrGameCommand},
		{Name: "help", Handler: handleHelpGameCommand},
	},
)
