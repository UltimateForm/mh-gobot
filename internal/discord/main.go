package discord

import (
	"errors"

	"github.com/UltimateForm/mh-gobot/internal/config"
	"github.com/bwmarrin/discordgo"
)

func Create() (*discordgo.Session, error) {
	dg, err := discordgo.New("Bot " + config.Global.DcToken)
	if err != nil {
		return nil, errors.Join(errors.New("failed to create dc bot"), err)
	}
	return dg, err
}
