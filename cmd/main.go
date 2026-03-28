package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/config"
	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/discord"
	"github.com/UltimateForm/mh-gobot/internal/game"
	"github.com/UltimateForm/mh-gobot/internal/img"
	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/UltimateForm/mh-gobot/internal/rcon_client"
	"github.com/UltimateForm/mh-gobot/internal/util"
	"github.com/bwmarrin/discordgo"
	"github.com/jedib0t/go-pretty/v6/table"
)

var rconPool *rcon_client.ConnectionPool

func Start() {
	config.Global.Validate()
	rconPool = rcon_client.NewPool(config.Global.RconUri, config.Global.RconPassword, 5, 60*time.Second)

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

	skirmishTracker := game.NewSkirmishTracker(rconPool, config.Global.EventsChannel, config.Global.SkirmishWinCap)
	deathmatchTracker := game.NewDeathmatchTracker()
	gameRouter := game.NewGameRouter(rconPool, skirmishTracker, deathmatchTracker)

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
		rconlistener.Run(appCtx)
		go handleEvents(appCtx, s, rconlistener, gameRouter)
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
	rconPool.Close()
}

func renderLeaderboardEmbed(t time.Time) (discord.RenderResult, error) {
	players, err := data.ReadTopPlayers(context.Background(), 20, data.TopCategory["score"])
	if err != nil {
		return discord.RenderResult{}, err
	}

	tw := table.NewWriter()
	tw.AppendHeader(table.Row{"#", "Player", "Score", "K", "D", "A"})
	for _, p := range players {
		tw.AppendRow(table.Row{p.Rank, p.Username, util.HumanFormat(p.Score), p.Kills, p.Deaths, p.Assists})
	}
	tw.SetStyle(table.StyleLight)
	tw.Style().Options.DrawBorder = false
	tw.Style().Options.SeparateRows = false

	timestamp := fmt.Sprintf("🕒 Last updated: <t:%d:R>\n", t.Unix())
	// len is wrong here, same inside TruncateCodeStringByLine but idc right now
	// TODO: fix this lol
	tableStr := util.TruncateCodeStringByLine(fmt.Sprintf("```\n%s\n```", tw.Render()), 4096-len(timestamp))
	return discord.RenderResult{
		Embed: &discordgo.MessageEmbed{
			Title:       "🏆 Score: Top 20",
			Description: timestamp + tableStr,
			Color:       0xF1C40F,
		},
	}, nil
}

func renderPopEmbed(t time.Time) (discord.RenderResult, error) {
	var infoRaw, scoreboardRaw string
	err := rconPool.WithClient(context.Background(), func(client *rcon_client.ControlledClient) error {
		var err error
		infoRaw, err = client.Execute("info")
		if err != nil {
			return errors.Join(errors.New("rcon exec info err"), err)
		}
		scoreboardRaw, err = client.Execute("scoreboard")
		if err != nil {
			return errors.Join(errors.New("rcon exec scoreboard err"), err)
		}
		return nil
	})
	if err != nil {
		return discord.RenderResult{}, err
	}

	serverInfo, err := parse.ParseServerInfo(infoRaw)
	if err != nil {
		return discord.RenderResult{}, errors.Join(errors.New("failed to parse server info"), err)
	}

	entries, err := parse.ParseScoreboard(scoreboardRaw)
	if err != nil {
		return discord.RenderResult{}, errors.Join(errors.New("failed to parse scoreboard"), err)
	}

	title := serverInfo.ServerName
	if title == "" {
		title = serverInfo.Host
	}

	baseFields := []*discordgo.MessageEmbedField{
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
			Name:   "\u200b",
			Value:  "\u200b",
			Inline: true,
		},
	}

	agg, err := data.ReadAggregates(context.Background())
	if err != nil {
		return discord.RenderResult{}, err
	}
	baseFields = append(baseFields,
		&discordgo.MessageEmbedField{
			// hehe this is actually kind of a lie :D we are counting players who have came in and either killed or died
			Name:   "👥 Total Players",
			Value:  fmt.Sprintf("```ansi\n\u001b[34;1m%d\u001b[0m\n```", agg.TotalPlayers),
			Inline: true,
		},
		&discordgo.MessageEmbedField{
			Name:   "⚔️ Total Kills",
			Value:  fmt.Sprintf("```ansi\n\u001b[31;1m%s\u001b[0m\n```", util.HumanFormat(agg.TotalKills)),
			Inline: true,
		},
		&discordgo.MessageEmbedField{
			Name:   "🪦 Total Deaths",
			Value:  fmt.Sprintf("```ansi\n\u001b[31m%s\u001b[0m\n```", util.HumanFormat(agg.TotalDeaths)),
			Inline: true,
		},
	)

	playersOnlineField := &discordgo.MessageEmbedField{
		Name:  fmt.Sprintf("🎮 Players Online (%d)", len(entries)),
		Value: "No players online",
	}
	baseFields = append(baseFields, playersOnlineField)

	var scoreboardImg io.Reader
	if len(entries) > 0 {
		skirmish := strings.EqualFold(serverInfo.GameMode, "skirmish")
		scoreboardImg, err = img.RenderScoreboardImage(entries, skirmish)
		if err != nil {
			log.Printf("failed to render scoreboard image: %v", err)
		}
		playersOnlineField.Value = ""
	}

	return discord.RenderResult{
		Embed: &discordgo.MessageEmbed{
			Title:       "🖥️ " + title,
			Description: fmt.Sprintf("🕒 Last updated: <t:%d:R>", t.Unix()),
			Color:       0x5865F2,
			Fields:      baseFields,
			Footer:      &discordgo.MessageEmbedFooter{Text: "https://github.com/UltimateForm/mh-gobot"},
		},
		Image: scoreboardImg,
	}, nil
}
