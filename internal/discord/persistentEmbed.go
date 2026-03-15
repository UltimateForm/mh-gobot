package discord

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

// TODO: expand to support normal messages too, no reason to bind this forever to just embeds

type PersistentEmbed struct {
	render   func(time.Time) (*discordgo.MessageEmbed, error)
	interval time.Duration
	channels map[string]string
	logger   *log.Logger
}

// channels is a map of channels and id for existing embed ids, if existing embed does not exist just pass empty id
func NewPersistentEmbed(renderFunc func(time.Time) (*discordgo.MessageEmbed, error), name string, interval time.Duration, channels map[string]string) *PersistentEmbed {
	return &PersistentEmbed{
		render:   renderFunc,
		interval: interval,
		channels: channels,
		logger: log.New(
			log.Default().Writer(),
			fmt.Sprintf("[PersistentEmbed:%v]", name),
			log.Default().Flags(),
		),
	}
}

func (src *PersistentEmbed) routine(ctx context.Context, dc *discordgo.Session) {
	ticker := time.NewTicker(src.interval)
	defer ticker.Stop()
	for {
		select {
		case t := <-ticker.C:
			src.logger.Println("rendering...")
			embed, err := src.render(t)
			if err != nil {
				src.logger.Printf("failed to render: %v", err)
				continue
			}
			for k, v := range src.channels {
				if v != "" {
					_, err := dc.ChannelMessageEditEmbed(k, v, embed)
					if err != nil {
						src.logger.Printf("failed to edit message: %v", err)
					}
				} else {
					msg, err := dc.ChannelMessageSendEmbed(k, embed)
					if err != nil {
						src.logger.Printf("failed to send message: %v", err)
						continue
					}
					src.channels[k] = msg.ID
				}
			}
		case <-ctx.Done():
			src.logger.Println("exiting due to context done")
			return
		}
	}
}

func (src *PersistentEmbed) Run(ctx context.Context, dc *discordgo.Session) {
	go src.routine(ctx, dc)
}
