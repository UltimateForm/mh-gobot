package game

import (
	"context"
	"fmt"
	"log"
	"maps"
	"math"
	"strings"
	"sync"

	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/UltimateForm/mh-gobot/internal/rcon_client"
	"github.com/UltimateForm/mh-gobot/internal/util"
	"github.com/bwmarrin/discordgo"
)

// mostly vibecoded, cuz i dont care about this that much tbh

const (
	winMod        = 0.5
	matchWinMod   = 0.25
	matchLoseMod  = 0.10
	maxSizeFactor = 2.0
)

type skirmishState int

const (
	skirmishIdle skirmishState = iota
	skirmishInProgress
)

type roundResult struct {
	playerID string
	username string
	delta    float64
	bonus    int
}

type SkirmishTracker struct {
	mu            sync.Mutex
	state         skirmishState
	roundDeltas   map[string]float64
	matchDeltas   map[string]float64
	teamScores    map[int]float64
	winCap        float64
	pool          *rcon_client.ConnectionPool
	eventsChannel string
	logger        *log.Logger
}

func NewSkirmishTracker(pool *rcon_client.ConnectionPool, eventsChannel string, winCap float64) *SkirmishTracker {
	return &SkirmishTracker{
		state:         skirmishIdle,
		roundDeltas:   make(map[string]float64),
		matchDeltas:   make(map[string]float64),
		teamScores:    make(map[int]float64),
		winCap:        winCap,
		pool:          pool,
		eventsChannel: eventsChannel,
		logger:        log.New(log.Default().Writer(), "[SkirmishTracker] ", log.Default().Flags()),
	}
}

func (t *SkirmishTracker) clearRound() {
	t.roundDeltas = make(map[string]float64)
}

func (t *SkirmishTracker) clearMatch() {
	t.roundDeltas = make(map[string]float64)
	t.matchDeltas = make(map[string]float64)
	t.teamScores = make(map[int]float64)
}

func (t *SkirmishTracker) OnMatchState(state string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch state {
	case "In progress":
		t.logger.Println("match started, resetting state")
		t.clearMatch()
		t.state = skirmishInProgress
	case "Leaving map":
		t.logger.Println("leaving map, resetting state")
		t.clearMatch()
		t.state = skirmishIdle
	}
}

func (t *SkirmishTracker) OnPlayerScore(e *parse.ScorefeedPlayerEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != skirmishInProgress {
		return
	}
	go func() {
		if err := data.AddPlayerScore(context.Background(), e.PlayerID, int(e.ScoreChange)); err != nil {
			t.logger.Printf("failed to add raw score for %s: %v", e.PlayerID, err)
		}
	}()
	if e.ScoreChange <= 0 {
		return // skip teamkill penalties for bonus accumulation
	}
	t.roundDeltas[e.PlayerID] += e.ScoreChange
	t.matchDeltas[e.PlayerID] += e.ScoreChange
}

func (t *SkirmishTracker) OnTeamScore(ctx context.Context, dc *discordgo.Session, e *parse.ScorefeedTeamEvent) {
	t.mu.Lock()
	if t.state != skirmishInProgress {
		t.mu.Unlock()
		return
	}
	if e.NewScore <= e.OldScore {
		t.mu.Unlock()
		return
	}

	// snapshot and reset round state
	roundSnapshot := t.roundDeltas
	t.clearRound()
	t.teamScores[e.TeamID] = e.NewScore

	isMatchOver := e.NewScore >= t.winCap
	var matchSnapshot map[string]float64
	teamScoresCopy := make(map[int]float64, len(t.teamScores))
	maps.Copy(teamScoresCopy, t.teamScores)
	if isMatchOver {
		matchSnapshot = t.matchDeltas
		t.clearMatch()
		t.state = skirmishIdle
	}

	winCap := t.winCap
	winningTeam := e.TeamID
	t.mu.Unlock()

	// fetch fresh scoreboard for team assignments
	var scoreboardRaw string
	err := t.pool.WithClient(ctx, func(client *rcon_client.ControlledClient) error {
		var err error
		scoreboardRaw, err = client.Execute("scoreboard")
		return err
	})
	if err != nil {
		t.logger.Printf("failed to fetch scoreboard: %v", err)
		return
	}
	entries, err := parse.ParseScoreboard(scoreboardRaw)
	if err != nil {
		t.logger.Printf("failed to parse scoreboard: %v", err)
		return
	}

	var winEntries, loseEntries []*parse.ScoreboardEntry
	for _, entry := range entries {
		if entry.TeamID == winningTeam {
			winEntries = append(winEntries, entry)
		} else {
			loseEntries = append(loseEntries, entry)
		}
	}
	t.logger.Printf("round end: team %d wins, %d winners, %d losers on scoreboard", winningTeam, len(winEntries), len(loseEntries))

	winSize := float64(len(winEntries))
	loseSize := float64(len(loseEntries))
	winSizeFactor, loseSizeFactor := 1.0, 1.0
	if winSize > 0 && loseSize > 0 {
		winSizeFactor = math.Min(loseSize/winSize, maxSizeFactor)
		loseSizeFactor = math.Min(winSize/loseSize, maxSizeFactor)
	}

	scoreFactor := 1.0
	if isMatchOver {
		var losingScore float64
		for teamID, score := range teamScoresCopy {
			if teamID != winningTeam {
				losingScore = score
				break
			}
		}
		diff := e.NewScore - losingScore
		scoreFactor = 0.5 + 0.5*(diff/winCap)
	}

	roundNum := int(e.NewScore)

	winResults := make([]roundResult, 0, len(winEntries))
	for _, entry := range winEntries {
		rd := roundSnapshot[entry.PlayerID]
		bonus := rd * winMod * winSizeFactor
		if isMatchOver {
			md := matchSnapshot[entry.PlayerID]
			bonus += md * matchWinMod * winSizeFactor * scoreFactor
		}
		b := int(math.Round(bonus))
		if b <= 0 {
			continue
		}
		matchesWon := 0
		if isMatchOver {
			matchesWon = 1
		}
		if err := data.UpsertSkirmishWin(ctx, entry.PlayerID, b, 1, matchesWon); err != nil {
			t.logger.Printf("upsert win failed for %s: %v", entry.PlayerID, err)
		}
		displayDelta := rd
		if isMatchOver {
			displayDelta = matchSnapshot[entry.PlayerID]
		}
		winResults = append(winResults, roundResult{entry.PlayerID, entry.UserName, displayDelta, b})
	}

	loseResults := make([]roundResult, 0)
	if isMatchOver {
		for _, entry := range loseEntries {
			md := matchSnapshot[entry.PlayerID]
			b := int(math.Round(md * matchLoseMod * loseSizeFactor))
			if b <= 0 {
				continue
			}
			if err := data.UpsertSkirmishWin(ctx, entry.PlayerID, b, 0, 0); err != nil {
				t.logger.Printf("upsert consolation failed for %s: %v", entry.PlayerID, err)
			}
			loseResults = append(loseResults, roundResult{entry.PlayerID, entry.UserName, md, b})
		}
	}

	t.logger.Printf("round %d: %d win bonuses, %d consolations (win_sf=%.2f, lose_sf=%.2f, score_f=%.2f)",
		roundNum, len(winResults), len(loseResults), winSizeFactor, loseSizeFactor, scoreFactor)

	// go t.sayResults(ctx, winningTeam, isMatchOver, winResults, loseResults)

	if dc != nil && t.eventsChannel != "" {
		go t.sendEmbed(
			dc,
			roundNum,
			winningTeam,
			winSizeFactor,
			loseSizeFactor,
			scoreFactor,
			winResults,
			loseResults,
			isMatchOver,
			teamScoresCopy,
		)
	}
}

func (t *SkirmishTracker) sayResults(ctx context.Context, winningTeam int, isMatchOver bool, winResults []roundResult, loseResults []roundResult) {
	if t.pool == nil {
		return
	}
	label := "round"
	if isMatchOver {
		label = "match"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Team %d wins %s!", winningTeam, label))
	for _, r := range winResults {
		sb.WriteString(fmt.Sprintf("\n> +%d to %s", r.bonus, r.username))
	}
	if isMatchOver {
		for _, r := range loseResults {
			sb.WriteString(fmt.Sprintf("\n> +%d to %s (L)", r.bonus, r.username))
		}
	}
	chunks := util.SplitChunks(sb.String(), 300)
	t.pool.WithClient(ctx, func(c *rcon_client.ControlledClient) error {
		for _, chunk := range chunks {
			if _, err := c.Execute("say " + chunk); err != nil {
				t.logger.Printf("say failed: %v", err)
			}
		}
		return nil
	})
}

func formatResultsTable(results []roundResult) string {
	if len(results) == 0 {
		return "No bonuses awarded"
	}
	var sb strings.Builder
	sb.WriteString("```\n")
	for _, r := range results {
		name := r.username
		if len(name) > 18 {
			name = name[:18]
		}
		fmt.Fprintf(&sb, "%-18s Δ%+.0f → +%d\n", name, r.delta, r.bonus)
	}
	sb.WriteString("```")
	return sb.String()
}

func (t *SkirmishTracker) sendEmbed(
	dc *discordgo.Session,
	roundNum int,
	winningTeam int,
	winSizeFactor float64,
	loseSizeFactor float64,
	scoreFactor float64,
	winResults []roundResult,
	loseResults []roundResult,
	isMatchOver bool,
	teamScores map[int]float64,
) {
	var title, description string
	color := 0x57F287

	if isMatchOver {
		var losingScore float64
		for teamID, score := range teamScores {
			if teamID != winningTeam {
				losingScore = score
				break
			}
		}
		title = fmt.Sprintf("🏆 Match over — Team %d wins!", winningTeam)
		description = fmt.Sprintf("**Score:** %d – %.0f | **Size:** %.2f/%.2f | **Margin:** %.2f\n**Mods:** win=%.2f | consolation=%.2f",
			roundNum, losingScore, winSizeFactor, loseSizeFactor, scoreFactor, matchWinMod, matchLoseMod)
	} else {
		title = fmt.Sprintf("⚔️ Round %d — Team %d wins", roundNum, winningTeam)
		description = fmt.Sprintf("**Mod:** %.2f | **Team balance:** %.2f", winMod, winSizeFactor)
	}

	fields := []*discordgo.MessageEmbedField{
		{
			Name:  fmt.Sprintf("🏅 Team %d bonuses", winningTeam),
			Value: formatResultsTable(winResults),
		},
	}
	if isMatchOver && len(loseResults) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  "🤝 Consolation bonuses",
			Value: formatResultsTable(loseResults),
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Fields:      fields,
	}
	if _, err := dc.ChannelMessageSendEmbed(t.eventsChannel, embed); err != nil {
		t.logger.Printf("failed to send embed: %v", err)
	}
}
