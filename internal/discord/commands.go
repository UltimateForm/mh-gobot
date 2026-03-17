package discord

import (
	"log"

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
		if handler, ok := r.handlers[i.ApplicationCommandData().Name]; ok {
			handler(s, i)
		}
	}
}
