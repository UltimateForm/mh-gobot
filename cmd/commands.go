package cmd

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/UltimateForm/ryard/internal/config"
	"github.com/UltimateForm/ryard/internal/rcon_client"
	"github.com/UltimateForm/ryard/internal/util"
	"github.com/bwmarrin/discordgo"
)

func registerCommands(dc *discordgo.Session) {
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

	_, err := dc.ApplicationCommandCreate(dc.State.User.ID, "", cmd)
	if err != nil {
		log.Printf("Cannot create slash command: %v", err)
	} else {
		log.Println("Slash command 'rconx' registered successfully")
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
