package game

import (
	"context"
	"log"
	"strings"

	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/UltimateForm/mh-gobot/internal/rcon_client"
	"github.com/bwmarrin/discordgo"
)

type GameRouter struct {
	pool       *rcon_client.ConnectionPool
	skirmish   *SkirmishTracker
	deathmatch *DeathmatchTracker
	active     GameTrackerCompute
	logger     *log.Logger
}

func NewGameRouter(pool *rcon_client.ConnectionPool, skirmish *SkirmishTracker, deathmatch *DeathmatchTracker) *GameRouter {
	return &GameRouter{
		pool:       pool,
		skirmish:   skirmish,
		deathmatch: deathmatch,
		logger:     log.New(log.Default().Writer(), "[GameRouter] ", log.Default().Flags()),
	}
}

func (r *GameRouter) OnMatchState(state string) {
	if state == "In progress" {
		r.active = r.resolveTracker()
	}
	if r.active != nil {
		r.active.OnMatchState(state)
	}
}

func (r *GameRouter) OnPlayerScore(e *parse.ScorefeedPlayerEvent) {
	if r.active != nil {
		r.active.OnPlayerScore(e)
	}
}

func (r *GameRouter) OnTeamScore(ctx context.Context, dc *discordgo.Session, e *parse.ScorefeedTeamEvent) {
	if r.active != nil {
		r.active.OnTeamScore(ctx, dc, e)
	}
}

func (r *GameRouter) OnKill(e *parse.KillfeedEvent) {
	if r.active != nil {
		r.active.OnKill(e)
	}
}

func (r *GameRouter) resolveTracker() GameTrackerCompute {
	var infoRaw string
	err := r.pool.WithClient(context.Background(), func(client *rcon_client.ControlledClient) error {
		var err error
		infoRaw, err = client.Execute("info")
		return err
	})
	if err != nil {
		r.logger.Printf("failed to fetch server info, defaulting to deathmatch: %v", err)
		return r.deathmatch
	}
	info, err := parse.ParseServerInfo(infoRaw)
	if err != nil {
		r.logger.Printf("failed to parse server info, defaulting to deathmatch: %v", err)
		return r.deathmatch
	}
	r.logger.Printf("game mode: %s", info.GameMode)
	if strings.EqualFold(info.GameMode, "skirmish") {
		return r.skirmish
	}
	return r.deathmatch
}
