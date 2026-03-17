package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/UltimateForm/ryard/internal/config"
	"github.com/UltimateForm/ryard/internal/data"
	"github.com/UltimateForm/ryard/internal/discord"
	"github.com/UltimateForm/ryard/internal/parse"
	"github.com/UltimateForm/ryard/internal/rcon_client"
	"github.com/UltimateForm/ryard/internal/util"
	"github.com/bwmarrin/discordgo"
	"github.com/jedib0t/go-pretty/v6/table"
)

func Start() {
	config.Global.Validate()
	// Create Discord session
	dg, err := discord.Create()
	if err != nil {
		log.Fatal(errors.Join(errors.New("failed to create dc bot"), err))
	}
	appCtx, stopApp := context.WithCancel(context.Background())

	dg.AddHandler(commandRegistry.Handler())

	var persistentPopWatch *discord.PersistentEmbed
	if config.Global.PopChannel != "" {
		persistentPopWatch = discord.NewPersistentEmbed(
			renderPopEmbed,
			"playerlist",
			time.Second*30,
			map[string]string{config.Global.PopChannel: ""},
		)
	}

	var persistentLeaderboard *discord.PersistentEmbed
	if config.Global.LeaderboardsChannel != "" {
		persistentLeaderboard = discord.NewPersistentEmbed(
			renderLeaderboardEmbed,
			"leaderboard",
			time.Second*60,
			map[string]string{config.Global.LeaderboardsChannel: ""},
		)
	}

	rconlistener, err := rcon_client.NewListener(
		config.Global.RconUri,
		config.Global.RconPassword,
		[]rcon_client.ListenType{
			rcon_client.ListenChat,
			rcon_client.ListenKillfeed,
			rcon_client.ListenScorefeed,
			rcon_client.ListenMatchstate,
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
		rconlistener.Run(appCtx)
		go handleEvents(appCtx, s, rconlistener)
		commandRegistry.Register(s)
		if persistentPopWatch != nil {
			persistentPopWatch.Run(appCtx, dg)
		}
		if persistentLeaderboard != nil {
			persistentLeaderboard.Run(appCtx, dg)
		}
	})

	err = dg.Open()
	if err != nil {
		log.Fatal("error opening connection:", err)
	}
	defer dg.Close()

	log.Println("we up")

	// wait for interrupt signal ie cntrl+c
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	stopApp()
}

func renderLeaderboardEmbed(t time.Time) (*discordgo.MessageEmbed, error) {
	players, err := data.ReadTopPlayers(context.Background(), 10, data.TopCategory["score"])
	if err != nil {
		return nil, err
	}

	tw := table.NewWriter()
	tw.AppendHeader(table.Row{"#", "Player", "Score", "K", "D", "A"})
	for _, p := range players {
		tw.AppendRow(table.Row{p.Rank, p.Username, util.HumanFormat(p.RawScore), p.Kills, p.Deaths, p.Assists})
	}
	tw.SetStyle(table.StyleLight)
	tw.Style().Options.DrawBorder = false
	tw.Style().Options.SeparateRows = false

	return &discordgo.MessageEmbed{
		Title:       "🏆 Score: Top 10",
		Description: fmt.Sprintf("🕒 Last updated: <t:%d:R>", t.Unix()),
		Color:       0xF1C40F,
		Fields: []*discordgo.MessageEmbedField{
			{Value: util.TruncateCodeString(fmt.Sprintf("```\n%s\n```", tw.Render()), 1024)},
		},
	}, nil
}

func renderPopEmbed(t time.Time) (*discordgo.MessageEmbed, error) {
	client, err := rcon_client.New(config.Global.RconUri)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	success, err := client.Authenticate(config.Global.RconPassword)
	if err != nil {
		return nil, errors.Join(errors.New("authentication error"), err)
	}
	if !success {
		return nil, errors.New("authentication failed")
	}

	infoRaw, err := client.Execute("info")
	if err != nil {
		return nil, errors.Join(errors.New("rcon exec info err"), err)
	}
	serverInfo, err := parse.ParseServerInfo(infoRaw)
	if err != nil {
		return nil, errors.Join(errors.New("failed to parse server info"), err)
	}

	scoreboardRaw, err := client.Execute("scoreboard")
	if err != nil {
		return nil, errors.Join(errors.New("rcon exec scoreboard err"), err)
	}
	entries, err := parse.ParseScoreboard(scoreboardRaw)
	if err != nil {
		return nil, errors.Join(errors.New("failed to parse scoreboard"), err)
	}

	var tableValue string
	if len(entries) == 0 {
		tableValue = "No players online"
	} else {
		tw := table.NewWriter()
		tw.AppendHeader(table.Row{"#", "Player", "Score", "K", "D", "A"})
		for i, e := range entries {
			tw.AppendRow(table.Row{i + 1, e.UserName, e.Score, e.Kills, e.Deaths, e.Assists})
		}
		tw.SetStyle(table.StyleLight)
		tw.Style().Options.DrawBorder = false
		tw.Style().Options.SeparateRows = false
		tableValue = util.TruncateCodeString(fmt.Sprintf("```\n%s\n```", tw.Render()), 1024)
	}

	title := serverInfo.ServerName
	if title == "" {
		title = serverInfo.Host
	}

	return &discordgo.MessageEmbed{
		Title:       "🖥️ " + title,
		Description: fmt.Sprintf("🕒 Last updated: <t:%d:R>", t.Unix()),
		Color:       0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "🗺️ Map",
				Value:  serverInfo.Map,
				Inline: true,
			},
			{
				Name:   "⚔️ Mode",
				Value:  serverInfo.GameMode,
				Inline: true,
			},
			{
				Name:  fmt.Sprintf("👥 Players Online (%d)", len(entries)),
				Value: tableValue,
			},
		},
	}, nil
}
