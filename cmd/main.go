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
	// Register slash command handler
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.ApplicationCommandData().Name == "rconx" {
			handleRconxCommand(s, i)
		}
	})

	persistentPopWatch := discord.NewPersistentEmbed(
		renderPopEmbed,
		"playerlist",
		time.Second*10,
		map[string]string{
			config.Global.PopChannel: "",
		},
	)

	// Register ready handler to register commands
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)

		// Register the rconx command (admin only)
		cmd := &discordgo.ApplicationCommand{
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
		}

		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", cmd)
		if err != nil {
			log.Printf("Cannot create slash command: %v", err)
		} else {
			log.Println("Slash command 'rconx' registered successfully")
		}
		persistentPopWatch.Run(appCtx, dg)
	})

	// Open connection
	err = dg.Open()
	if err != nil {
		log.Fatal("Error opening connection:", err)
	}
	defer dg.Close()

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	// RunPopWatch(dg, appCtx)
	// Wait for interrupt signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	stopApp()
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
		tw.AppendHeader(table.Row{"#", "Player", "Score", "K", "D"})
		for i, e := range entries {
			tw.AppendRow(table.Row{i + 1, e.UserName, e.Score, e.Kills, e.Deaths})
		}
		tw.SetStyle(table.StyleLight)
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
				Name:  fmt.Sprintf("👥 Players (%d)", len(entries)),
				Value: tableValue,
			},
		},
	}, nil
}
