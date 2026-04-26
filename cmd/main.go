package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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

	weightProvider := game.NewScoreWeightProvider()
	weightProvider.Refresh(appCtx)

	skirmishTracker := game.NewSkirmishTracker(rconPool, config.Global.EventsChannel, config.Global.SkirmishWinCap, weightProvider)
	deathmatchTracker := game.NewDeathmatchTracker(weightProvider)
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
	ctx := context.Background()
	players, err := data.ReadTopPlayers(ctx, 20, data.TopCategory["score"])
	if err != nil {
		return discord.RenderResult{}, err
	}

	podiumIDs := make([]string, 0, 3)
	for i := range min(len(players), 3) {
		podiumIDs = append(podiumIDs, players[i].PlayerID)
	}
	avatars := avatarCache.GetMany(ctx, podiumIDs)

	imgReader, err := img.RenderLeaderboardImage(players, avatars)
	if err != nil {
		return discord.RenderResult{}, err
	}

	return discord.RenderResult{
		Content:   fmt.Sprintf("🏆 **Top 20** — updated <t:%d:R>", t.Unix()),
		Image:     imgReader,
		ImageName: "leaderboard.png",
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

	if config.Global.SkirmishAltPopType == 1 && strings.EqualFold(serverInfo.GameMode, "skirmish") {
		return renderAltSkirmishPopEmbed(t, serverInfo, entries)
	}

	title := serverInfo.ServerName
	if config.Global.ServerNameOverride != "" {
		title = config.Global.ServerNameOverride
	}
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

func loadMapImage(mapName string) (io.Reader, string) {
	if mapName == "" {
		return nil, ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, ""
	}
	matches, err := filepath.Glob(filepath.Join(home, ".mh-gobot", "imgmap", mapName+".*"))
	if err != nil || len(matches) == 0 {
		return nil, ""
	}
	path := matches[0]
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ""
	}
	return bytes.NewReader(data), filepath.Base(path)
}

func renderAltSkirmishPopEmbed(t time.Time, serverInfo *parse.ServerInfo, entries []*parse.ScoreboardEntry) (discord.RenderResult, error) {
	var durationRaw string
	err := rconPool.WithClient(context.Background(), func(client *rcon_client.ControlledClient) error {
		var err error
		durationRaw, err = client.Execute("getmatchduration")
		return err
	})
	if err != nil {
		return discord.RenderResult{}, errors.Join(errors.New("rcon exec getmatchduration err"), err)
	}

	seconds, err := parse.ParseMatchDuration(durationRaw)
	if err != nil {
		return discord.RenderResult{}, errors.Join(errors.New("failed to parse match duration"), err)
	}

	var statusValue string
	if seconds < 0 {
		startUnix := t.Unix() + int64(seconds)
		statusValue = fmt.Sprintf("Started <t:%d:R>", startUnix)
	} else {
		statusValue = "waiting..."
	}

	var team1, team2 []*parse.ScoreboardEntry
	for _, e := range entries {
		switch e.TeamID {
		case 1:
			team1 = append(team1, e)
		case 2:
			team2 = append(team2, e)
		}
	}

	formatTeam := func(team []*parse.ScoreboardEntry) string {
		if len(team) == 0 {
			return "> *empty*"
		}
		var b strings.Builder
		for _, p := range team {
			fmt.Fprintf(&b, "> [%s](https://mordhau-scribe.com/player/%s) — K %d · D %d · A %d\n", p.UserName, p.PlayerID, p.Kills, p.Deaths, p.Assists)
		}
		return strings.TrimRight(b.String(), "\n")
	}

	serverName := serverInfo.ServerName
	if config.Global.ServerNameOverride != "" {
		serverName = config.Global.ServerNameOverride
	}
	description := fmt.Sprintf("```\n%s\n```\n🕒 Last updated: <t:%d:R>", serverName, t.Unix())

	fields := []*discordgo.MessageEmbedField{
		{Name: "🎮 Type", Value: "8vs8", Inline: true},
		{Name: "⏱️ Status", Value: statusValue, Inline: true},
		{Name: "🗺️ Map", Value: serverInfo.Map, Inline: true},
		{Name: fmt.Sprintf("🔴 Team 1 (%d)", len(team1)), Value: formatTeam(team1), Inline: false},
		{Name: fmt.Sprintf("🔵 Team 2 (%d)", len(team2)), Value: formatTeam(team2), Inline: false},
	}

	mapImg, imgName := loadMapImage(serverInfo.Map)

	return discord.RenderResult{
		Embed: &discordgo.MessageEmbed{
			Description: description,
			Color:       0x5865F2,
			Fields:      fields,
			Footer:      &discordgo.MessageEmbedFooter{Text: "https://github.com/UltimateForm/mh-gobot"},
		},
		Image:     mapImg,
		ImageName: imgName,
	}, nil
}
