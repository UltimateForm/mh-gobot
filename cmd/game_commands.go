package cmd

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

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
	msg := fmt.Sprintf("%s: Score %d | K %d | D %d | A %d",
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

	return rconSay(ctx, fmt.Sprintf("%s vs %s: %d - %d", event.UserName, p2.Username, versus.AKills, versus.BKills))
}

func handleRollGameCommand(ctx context.Context, event *parse.ChatEvent, args []string) error {
	n := rand.Intn(100)
	return rconSay(ctx, fmt.Sprintf("%s rolled a %02d", event.UserName, n))
}

var gameCommandRegistry = game.NewGameCommandRegistry(
	config.Global.GameCommandPrefix,
	[]game.GameCommand{
		{Name: "score", Handler: handleScoreGameCommand},
		{Name: "roll", Handler: handleRollGameCommand},
		{Name: "versus", Aliases: []string{"vs"}, Handler: handleVersusGameCommand},
	},
)
