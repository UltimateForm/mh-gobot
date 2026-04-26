package game

import (
	"context"
	"fmt"
	"log"
	"maps"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/UltimateForm/mh-gobot/internal/rcon_client"
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

const killFactorFloor = 0.05

type roundResult struct {
	playerID   string
	username   string
	delta      float64
	bonus      int
	weight     float64
	killFactor float64
}

type matchParticipant struct {
	team      int
	roundsWon int
}

type SkirmishTracker struct {
	mu                sync.Mutex
	state             skirmishState
	roundDeltas       map[string]float64
	matchDeltas       map[string]float64
	roundEliminations map[string][]string
	teamScores        map[int]float64
	matchParticipants map[string]*matchParticipant
	matchStartedAt    time.Time
	matchMap          string
	winCap            float64
	pool              *rcon_client.ConnectionPool
	eventsChannel     string
	weightProvider    *ScoreWeightProvider
	logger            *log.Logger
}

func NewSkirmishTracker(pool *rcon_client.ConnectionPool, eventsChannel string, winCap float64, wp *ScoreWeightProvider) *SkirmishTracker {
	return &SkirmishTracker{
		state:             skirmishIdle,
		roundDeltas:       make(map[string]float64),
		matchDeltas:       make(map[string]float64),
		roundEliminations: make(map[string][]string),
		teamScores:        make(map[int]float64),
		matchParticipants: make(map[string]*matchParticipant),
		winCap:            winCap,
		pool:              pool,
		eventsChannel:     eventsChannel,
		weightProvider:    wp,
		logger:            log.New(log.Default().Writer(), "[SkirmishTracker] ", log.Default().Flags()),
	}
}

func (t *SkirmishTracker) clearRound() {
	t.roundDeltas = make(map[string]float64)
	t.roundEliminations = make(map[string][]string)
}

func (t *SkirmishTracker) clearMatch() {
	t.roundDeltas = make(map[string]float64)
	t.matchDeltas = make(map[string]float64)
	t.roundEliminations = make(map[string][]string)
	t.teamScores = make(map[int]float64)
	t.matchParticipants = make(map[string]*matchParticipant)
	t.matchStartedAt = time.Time{}
	t.matchMap = ""
}

func (t *SkirmishTracker) OnMatchState(state string) {
	t.mu.Lock()
	switch state {
	case "In progress":
		t.logger.Println("match started, resetting state")
		t.clearMatch()
		t.matchStartedAt = time.Now()
		t.state = skirmishInProgress
		t.mu.Unlock()
		go t.captureMatchMap()
		return
	case "Leaving map":
		t.logger.Println("leaving map, resetting state")
		t.clearMatch()
		t.state = skirmishIdle
	}
	t.mu.Unlock()
}

func (t *SkirmishTracker) captureMatchMap() {
	var infoRaw string
	err := t.pool.WithClient(context.Background(), func(client *rcon_client.ControlledClient) error {
		var err error
		infoRaw, err = client.Execute("info")
		return err
	})
	if err != nil {
		t.logger.Printf("failed to fetch info for map capture: %v", err)
		return
	}
	info, err := parse.ParseServerInfo(infoRaw)
	if err != nil {
		t.logger.Printf("failed to parse server info for map capture: %v", err)
		return
	}
	t.mu.Lock()
	t.matchMap = info.Map
	t.mu.Unlock()
}

func (t *SkirmishTracker) OnPlayerScore(e *parse.ScorefeedPlayerEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != skirmishInProgress {
		return
	}
	go func() {
		ctx := context.Background()
		player, err := data.ReadPlayer(ctx, e.PlayerID)
		currentScore := 0
		if err == nil {
			currentScore = player.Score
		}
		weight := t.weightProvider.Weight(currentScore)
		weightedDelta := int(math.Round(float64(e.ScoreChange) * weight))
		if err := data.AddPlayerScore(ctx, e.PlayerID, weightedDelta); err != nil {
			t.logger.Printf("failed to add raw score for %s: %v", e.PlayerID, err)
		}
	}()
	if e.ScoreChange <= 0 {
		return // skip teamkill penalties for bonus accumulation
	}
	t.roundDeltas[e.PlayerID] += e.ScoreChange
	t.matchDeltas[e.PlayerID] += e.ScoreChange
}

func (t *SkirmishTracker) OnKill(e *parse.KillfeedEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != skirmishInProgress {
		return
	}
	t.roundEliminations[e.KillerID] = append(t.roundEliminations[e.KillerID], e.KilledID)
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
	elimSnapshot := t.roundEliminations
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

	// track participants and increment round wins for winners
	for _, entry := range winEntries {
		if t.matchParticipants[entry.PlayerID] == nil {
			t.matchParticipants[entry.PlayerID] = &matchParticipant{team: entry.TeamID}
		}
		t.matchParticipants[entry.PlayerID].roundsWon++
	}
	for _, entry := range loseEntries {
		if t.matchParticipants[entry.PlayerID] == nil {
			t.matchParticipants[entry.PlayerID] = &matchParticipant{team: entry.TeamID}
		}
	}

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

	// batch-read current scores for weight and kill factor calculation
	// include both scoreboard players and elimination victims
	idSet := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		idSet[entry.PlayerID] = struct{}{}
	}
	for _, victims := range elimSnapshot {
		for _, vid := range victims {
			idSet[vid] = struct{}{}
		}
	}
	allIDs := make([]string, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}
	playerScores, err := data.ReadPlayerScores(ctx, allIDs)
	if err != nil {
		t.logger.Printf("failed to read player scores for weighting: %v", err)
		playerScores = make(map[string]int)
	}

	avgK := math.Max(t.weightProvider.AvgScore(), scoreWeightFloor)

	winResults := make([]roundResult, 0, len(winEntries))
	for _, entry := range winEntries {
		rd := roundSnapshot[entry.PlayerID]
		// kill factor: based on rank differential of eliminations
		kf := 1.0
		if victims := elimSnapshot[entry.PlayerID]; len(victims) > 0 {
			sum := 0.0
			for _, vid := range victims {
				sum += (float64(playerScores[vid]) + avgK) / (float64(playerScores[entry.PlayerID]) + avgK)
			}
			kf = max(sum/float64(len(victims)), killFactorFloor)
		}
		bonus := rd * winMod * winSizeFactor * kf
		if isMatchOver {
			md := matchSnapshot[entry.PlayerID]
			bonus += md * matchWinMod * winSizeFactor * scoreFactor
		}
		w := t.weightProvider.Weight(playerScores[entry.PlayerID])
		b := int(math.Round(bonus * w))
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
		winResults = append(winResults, roundResult{entry.PlayerID, entry.UserName, displayDelta, b, w, kf})
	}

	loseResults := make([]roundResult, 0)
	if isMatchOver {
		for _, entry := range loseEntries {
			md := matchSnapshot[entry.PlayerID]
			w := t.weightProvider.Weight(playerScores[entry.PlayerID])
			b := int(math.Round(md * matchLoseMod * loseSizeFactor * w))
			if b <= 0 {
				continue
			}
			if err := data.UpsertSkirmishWin(ctx, entry.PlayerID, b, 0, 0); err != nil {
				t.logger.Printf("upsert consolation failed for %s: %v", entry.PlayerID, err)
			}
			loseResults = append(loseResults, roundResult{entry.PlayerID, entry.UserName, md, b, w, 1.0})
		}

		// persist match to database
		go func() {
			matchCtx := context.Background()
			participants := make([]data.MatchParticipant, 0, len(t.matchParticipants))
			for playerID, mp := range t.matchParticipants {
				participants = append(participants, data.MatchParticipant{
					PlayerID:  playerID,
					Team:      mp.team,
					RoundsWon: mp.roundsWon,
				})
			}
			match := data.Match{
				GameMode:   "skirmish",
				Map:        t.matchMap,
				StartedAt:  t.matchStartedAt,
				EndedAt:    time.Now(),
				Team1Score: int(teamScoresCopy[1]),
				Team2Score: int(teamScoresCopy[2]),
			}
			if _, err := data.InsertMatch(matchCtx, match, participants); err != nil {
				t.logger.Printf("failed to insert match: %v", err)
			}
		}()
	}

	t.logger.Printf("round %d: %d win bonuses, %d consolations (win_sf=%.2f, lose_sf=%.2f, score_f=%.2f, K=%.0f)",
		roundNum, len(winResults), len(loseResults), winSizeFactor, loseSizeFactor, scoreFactor, math.Max(t.weightProvider.AvgScore(), scoreWeightFloor))

	if isMatchOver {
		t.weightProvider.Refresh(ctx)
	}

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

func formatResultsTable(results []roundResult) string {
	if len(results) == 0 {
		return "No bonuses awarded"
	}
	var sb strings.Builder
	sb.WriteString("```\n")
	for _, r := range results {
		name := r.username
		if len(name) > 16 {
			name = name[:16]
		}
		fmt.Fprintf(&sb, "%-16s Δ%+.0f w=%.2f kf=%.2f → +%d\n", name, r.delta, r.weight, r.killFactor, r.bonus)
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

	avgK := math.Max(t.weightProvider.AvgScore(), scoreWeightFloor)

	if isMatchOver {
		var losingScore float64
		for teamID, score := range teamScores {
			if teamID != winningTeam {
				losingScore = score
				break
			}
		}
		title = fmt.Sprintf("🏆 Match over — Team %d wins!", winningTeam)
		description = fmt.Sprintf("**Score:** %d – %.0f | **Size:** %.2f/%.2f | **Margin:** %.2f\n**Mods:** win=%.2f | consolation=%.2f | **K:** %.0f",
			roundNum, losingScore, winSizeFactor, loseSizeFactor, scoreFactor, matchWinMod, matchLoseMod, avgK)
	} else {
		title = fmt.Sprintf("⚔️ Round %d — Team %d wins", roundNum, winningTeam)
		description = fmt.Sprintf("**Mod:** %.2f | **Team balance:** %.2f | **K:** %.0f", winMod, winSizeFactor, avgK)
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
