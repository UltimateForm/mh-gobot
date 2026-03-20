package game

import (
	"context"
	"log"
	"strings"

	"github.com/UltimateForm/mh-gobot/internal/parse"
)

type GameCommand struct {
	Name    string
	Handler func(ctx context.Context, event *parse.ChatEvent, args []string) error
}

type GameCommandRegistry struct {
	prefix   string
	handlers map[string]func(ctx context.Context, event *parse.ChatEvent, args []string) error
	logger   *log.Logger
}

func NewGameCommandRegistry(prefix string, commands []GameCommand) *GameCommandRegistry {
	handlers := make(map[string]func(ctx context.Context, event *parse.ChatEvent, args []string) error, len(commands))
	for _, c := range commands {
		handlers[c.Name] = c.Handler
	}
	return &GameCommandRegistry{
		prefix:   prefix,
		handlers: handlers,
		logger:   log.New(log.Default().Writer(), "[GameCommandRegistry] ", log.Default().Flags()),
	}
}

// Dispatch checks if the chat message is a command, parses it, and calls the handler.
// Returns true if the message started with the prefix (was a command attempt), false otherwise.
func (r *GameCommandRegistry) Dispatch(ctx context.Context, event *parse.ChatEvent) bool {
	msg := strings.TrimSpace(event.Message)
	if !strings.HasPrefix(msg, r.prefix) {
		return false
	}
	rest := strings.TrimPrefix(msg, r.prefix)
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return true
	}
	name := strings.ToLower(parts[0])
	args := parts[1:]
	handler, ok := r.handlers[name]
	if !ok {
		r.logger.Printf("unknown command %q from %s (%s)", name, event.UserName, event.PlayerID)
		return true
	}
	if err := handler(ctx, event, args); err != nil {
		r.logger.Printf("command %q failed for %s (%s): %v", name, event.UserName, event.PlayerID, err)
	}
	return true
}
