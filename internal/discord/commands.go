package discord

import (
	"log"
	"slices"

	"github.com/UltimateForm/mh-gobot/internal/config"
	"github.com/bwmarrin/discordgo"
)

type Command struct {
	Definition *discordgo.ApplicationCommand
	Handler    func(s *discordgo.Session, i *discordgo.InteractionCreate)
}

type CommandRegistry struct {
	commands []Command
	handlers map[string]func(*discordgo.Session, *discordgo.InteractionCreate)
}

func NewCommandRegistry(commands []Command) *CommandRegistry {
	handlers := make(map[string]func(*discordgo.Session, *discordgo.InteractionCreate))
	for _, cmd := range commands {
		handlers[cmd.Definition.Name] = cmd.Handler
	}
	return &CommandRegistry{commands: commands, handlers: handlers}
}

func (r *CommandRegistry) Register(dc *discordgo.Session) {
	for _, cmd := range r.commands {
		_, err := dc.ApplicationCommandCreate(dc.State.User.ID, "", cmd.Definition)
		if err != nil {
			log.Printf("failed to register slash command %q: %v", cmd.Definition.Name, err)
		} else {
			log.Printf("slash command %q registered", cmd.Definition.Name)
		}
	}
}

func (r *CommandRegistry) Handler() func(*discordgo.Session, *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		cmdName := i.ApplicationCommandData().Name

		userID := ""
		if i.Member != nil && i.Member.User != nil {
			userID = i.Member.User.ID
		} else if i.User != nil {
			userID = i.User.ID
		}

		// Reject DMs entirely
		if i.GuildID == "" {
			log.Printf("[CMD] rejected DM for command %q from user %s", cmdName, userID)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "This command is not available in DMs",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		// Check if guild is in allowed list
		if len(config.Global.KnownServers) > 0 && !slices.Contains(config.Global.KnownServers, i.GuildID) {
			log.Printf("[CMD] rejected unauthorized server %s for command %q from user %s", i.GuildID, cmdName, userID)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "This command is not available on this server",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}

		if handler, ok := r.handlers[cmdName]; ok {
			handler(s, i)
		}
	}
}
