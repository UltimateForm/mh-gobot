package discord

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/bwmarrin/discordgo"
)

type RenderResult struct {
	Content   string                  // optional plain-text content; used when Embed is nil
	Embed     *discordgo.MessageEmbed // optional, when nil the message is sent without an embed wrapper
	Image     io.Reader               // optional, nil if no image
	ImageName string                  // optional attachment filename; defaults to "embed_image.png"
}

// TODO: consider different name as we are no longer doing just embeds
type PersistentEmbed struct {
	name     string
	render   func(time.Time) (RenderResult, error)
	interval time.Duration
	channels map[string]string
	logger   *log.Logger
}

// channels is a map of channels and id for existing embed ids, if existing embed does not exist just pass empty id
func NewPersistentEmbed(renderFunc func(time.Time) (RenderResult, error), name string, interval time.Duration, channels map[string]string) *PersistentEmbed {
	return &PersistentEmbed{
		name:     name,
		render:   renderFunc,
		interval: interval,
		channels: channels,
		logger: log.New(
			log.Default().Writer(),
			fmt.Sprintf("[PersistentEmbed:%v] ", name),
			log.Default().Flags(),
		),
	}
}

func metaKey(name, channelID string) string {
	return fmt.Sprintf("persistent_embed:%s:%s", name, channelID)
}

func (src *PersistentEmbed) tick(ctx context.Context, dc *discordgo.Session, t time.Time) {
	result, err := src.render(t)
	if err != nil {
		src.logger.Printf("failed to render: %v", err)
		return
	}
	embed := result.Embed
	imageName := result.ImageName
	if imageName == "" {
		imageName = "embed_image.png"
	}
	if result.Image != nil && embed != nil {
		// embed images need a special placeholder wrapper so we prepare it here
		embed.Image = &discordgo.MessageEmbedImage{URL: "attachment://" + imageName}
	}
	for k, v := range src.channels {
		if v != "" {
			edit := &discordgo.MessageEdit{Channel: k, ID: v, Attachments: &[]*discordgo.MessageAttachment{}}
			if embed != nil {
				edit.Embed = embed
			} else {
				edit.Embeds = &[]*discordgo.MessageEmbed{}
				content := result.Content
				edit.Content = &content
			}
			if result.Image != nil {
				edit.Files = []*discordgo.File{{Name: imageName, Reader: result.Image}}
			}
			if _, err := dc.ChannelMessageEditComplex(edit); err != nil {
				src.logger.Printf("failed to edit message: %v", err)
			}
		} else {
			storedID, err := data.GetMeta(ctx, metaKey(src.name, k))
			if err != nil && !errors.Is(err, data.DbMetaNotFound) {
				src.logger.Printf("failed to read meta: %v", err)
			}
			if storedID != "" {
				if err := dc.ChannelMessageDelete(k, storedID); err != nil {
					src.logger.Printf("failed to delete old message: %v", err)
				}
			}
			send := &discordgo.MessageSend{}
			if embed != nil {
				send.Embed = embed
			} else {
				// kinda hoping we always got a content
				send.Content = result.Content
			}
			if result.Image != nil {
				send.Files = []*discordgo.File{{Name: imageName, Reader: result.Image}}
			}
			msg, err := dc.ChannelMessageSendComplex(k, send)
			if err != nil {
				src.logger.Printf("failed to send message: %v", err)
				continue
			}
			src.channels[k] = msg.ID
			if err := data.SetMeta(ctx, metaKey(src.name, k), msg.ID); err != nil {
				src.logger.Printf("failed to store meta: %v", err)
			}
		}
	}
}

func (src *PersistentEmbed) routine(ctx context.Context, dc *discordgo.Session) {
	ticker := time.NewTicker(src.interval)
	defer ticker.Stop()
	src.tick(ctx, dc, time.Now())
	for {
		select {
		case t := <-ticker.C:
			src.tick(ctx, dc, t)
		case <-ctx.Done():
			src.logger.Println("exiting due to context done")
			return
		}
	}
}

func (src *PersistentEmbed) Run(ctx context.Context, dc *discordgo.Session) {
	go src.routine(ctx, dc)
}
