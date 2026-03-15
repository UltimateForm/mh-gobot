package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/UltimateForm/ryard/internal/config"
	"github.com/bwmarrin/discordgo"
)

func watchPop(s *discordgo.Session, interval time.Duration, ctx context.Context) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case t := <-ticker.C:
			_, err := s.ChannelMessageSend(config.Global.PopChannel, fmt.Sprintf("hello my boy, it is %v o'clock", t))
			if err != nil {
				log.Print(err)
			}
			// tbd
		case <-ctx.Done():
			log.Print("watch pop received signal to stop")
			return
		}
	}
}

func RunPopWatch(dsess *discordgo.Session, ctx context.Context) {
	go watchPop(dsess, time.Second*5, ctx)
}
