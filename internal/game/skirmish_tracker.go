package game

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/data"
	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/UltimateForm/mh-gobot/internal/rcon_client"
	"github.com/bwmarrin/discordgo"
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

type SkirmishTracker struct {
	mu             sync.Mutex
	state          skirmishState
	currentRound   int
	players        map[string]*SkirmishPlayer
	teamScores     map[int]float64
	matchRounds    []SkirmishMatchRound
	matchStartedAt time.Time
	matchMap       string
	winCap         float64
	pool           *rcon_client.ConnectionPool
	eventsChannel  string
	weightProvider *ScoreWeightProvider
	gameConfig     *GameConfig
	logger         *log.Logger
}

func NewSkirmishTracker(pool *rcon_client.ConnectionPool, eventsChannel string, winCap float64, wp *ScoreWeightProvider, gc *GameConfig) *SkirmishTracker {
	return &SkirmishTracker{
		state:          skirmishIdle,
		currentRound:   0,
		players:        make(map[string]*SkirmishPlayer),
		teamScores:     make(map[int]float64),
		matchRounds:    make([]SkirmishMatchRound, 0),
		winCap:         winCap,
		pool:           pool,
		eventsChannel:  eventsChannel,
		weightProvider: wp,
		gameConfig:     gc,
		logger:         log.New(log.Default().Writer(), "[SkirmishTracker] ", log.Default().Flags()),
	}
}

func (t *SkirmishTracker) clearMatch() {
	t.currentRound = 0
	t.players = make(map[string]*SkirmishPlayer)
	t.teamScores = make(map[int]float64)
	t.matchRounds = make([]SkirmishMatchRound, 0)
	t.matchStartedAt = time.Time{}
	t.matchMap = ""
}

func (t *SkirmishTracker) getOrInitPlayer(playerID, username string) *SkirmishPlayer {
	if p, ok := t.players[playerID]; ok {
		return p
	}
	p := &SkirmishPlayer{
		PlayerId: playerID,
		Name:     username,
		Rounds:   make(map[int]SkirmishPlayerPerformance),
	}
	t.players[playerID] = p
	return p
}

func (t *SkirmishTracker) ensureRoundEntry(playerID string, round int) {
	p := t.getOrInitPlayer(playerID, "")
	if _, ok := p.Rounds[round]; !ok {
		p.Rounds[round] = SkirmishPlayerPerformance{}
	}
}

func (t *SkirmishTracker) OnMatchState(state string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch state {
	case "In progress":
		t.logger.Println("match started, resetting state")
		t.clearMatch()
		t.state = skirmishInProgress
		t.matchStartedAt = time.Now()
		go t.captureMatchMap()
	case "Leaving map":
		t.logger.Println("leaving map, resetting state")
		t.clearMatch()
		t.state = skirmishIdle
	}
}

func (t *SkirmishTracker) captureMatchMap() {
	var mapName string
	err := t.pool.WithClient(context.Background(), func(client *rcon_client.ControlledClient) error {
		var err error
		mapName, err = client.Execute("info")
		return err
	})
	if err != nil {
		t.logger.Printf("failed to fetch server info: %v", err)
		return
	}

	info, err := parse.ParseServerInfo(mapName)
	if err != nil {
		t.logger.Printf("failed to parse server info: %v", err)
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

	p := t.getOrInitPlayer(e.PlayerID, e.UserName)
	perf := p.Rounds[t.currentRound]
	perf.Score += int(e.ScoreChange)
	p.Rounds[t.currentRound] = perf

	go func() {
		if err := data.AddPlayerScore(context.Background(), e.PlayerID, int(e.ScoreChange)); err != nil {
			t.logger.Printf("failed to add score for %s: %v", e.PlayerID, err)
		}
	}()
}

func (t *SkirmishTracker) OnKill(e *parse.KillfeedEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != skirmishInProgress {
		return
	}

	if e.IsAssist {
		p := t.getOrInitPlayer(e.KillerID, e.UserName)
		perf := p.Rounds[t.currentRound]
		perf.Assists++
		p.Rounds[t.currentRound] = perf
	} else {
		p := t.getOrInitPlayer(e.KillerID, e.UserName)
		perf := p.Rounds[t.currentRound]
		perf.Kills++
		p.Rounds[t.currentRound] = perf
	}

	p := t.getOrInitPlayer(e.KilledID, e.KilledUserName)
	perf := p.Rounds[t.currentRound]
	perf.Deaths++
	p.Rounds[t.currentRound] = perf
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

	isMatchOver := e.NewScore >= t.winCap
	winningTeam := e.TeamID
	roundNum := t.currentRound + 1
	t.mu.Unlock()

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

	t.mu.Lock()

	// Update team assignments and ensure round entries for all present players
	for _, entry := range entries {
		p := t.getOrInitPlayer(entry.PlayerID, entry.UserName)
		// Team change detection: reset rounds if team changed
		if p.Team != 0 && p.Team != entry.TeamID {
			t.logger.Printf("player %s switched teams (was %d, now %d), resetting rounds", entry.PlayerID, p.Team, entry.TeamID)
			p.Rounds = make(map[int]SkirmishPlayerPerformance)
		}
		p.Team = entry.TeamID
		t.ensureRoundEntry(entry.PlayerID, t.currentRound)
	}

	// Snapshot match state before modifications
	teamScoresCopy := make(map[int]float64, len(t.teamScores))
	for k, v := range t.teamScores {
		teamScoresCopy[k] = v
	}

	t.teamScores[e.TeamID] = e.NewScore

	// Batch read player scores for weighted bonus calc
	allIDs := make([]string, 0, len(entries))
	idSeen := make(map[string]bool)
	for _, entry := range entries {
		if !idSeen[entry.PlayerID] {
			allIDs = append(allIDs, entry.PlayerID)
			idSeen[entry.PlayerID] = true
		}
	}

	t.mu.Unlock()

	playerScores, err := data.ReadPlayerScores(ctx, allIDs)
	if err != nil {
		t.logger.Printf("failed to read player scores: %v", err)
		playerScores = make(map[string]int)
	}

	avgK := math.Max(t.weightProvider.AvgScore(), scoreWeightFloor)

	// Win bonuses
	winResults := make([]roundResult, 0, len(winEntries))
	for _, entry := range winEntries {
		t.mu.Lock()
		p := t.players[entry.PlayerID]
		rd := float64(p.Rounds[t.currentRound].Score)
		t.mu.Unlock()

		// Kill factor from elimination data
		kf := 1.0
		if p.Rounds[t.currentRound].Kills > 0 {
			// Simplified: use kills as indicator of contribution
			kf = math.Max(float64(p.Rounds[t.currentRound].Kills)*1.0/float64(max(len(loseEntries), 1)), killFactorFloor)
			if kf > 4.0 {
				kf = 4.0
			}
		}

		winMod := t.gameConfig.Get(CfgSkirmishRoundWinMod)
		matchWinMod := t.gameConfig.Get(CfgSkirmishMatchWinMod)
		bonus := rd * winMod * (float64(len(loseEntries)) / float64(len(winEntries))) * kf
		if isMatchOver {
			md := float64(p.MatchResultScore)
			bonus += md * matchWinMod
		}

		w := t.weightProvider.Weight(playerScores[entry.PlayerID])
		b := int(math.Round(bonus * w))

		if b > 0 {
			matchesWon := 0
			if isMatchOver {
				matchesWon = 1
			}
			if err := data.UpsertSkirmishWin(ctx, entry.PlayerID, b, 1, matchesWon); err != nil {
				t.logger.Printf("upsert win failed for %s: %v", entry.PlayerID, err)
			}
			winResults = append(winResults, roundResult{entry.PlayerID, entry.UserName, rd, b, w, kf})
		}
	}

	t.mu.Lock()
	t.currentRound++
	t.mu.Unlock()

	// Loss calculation and match-end logic
	losses := make([]MatchLossCalc, 0)
	if isMatchOver {
		t.mu.Lock()
		totalRounds := t.currentRound
		persistPlayers := t.players
		persistMap := t.matchMap
		persistStart := t.matchStartedAt
		t.mu.Unlock()

		for _, entry := range loseEntries {
			playerScore := playerScores[entry.PlayerID]
			calc := ComputeMatchLoss(
				entry.PlayerID,
				playerScore,
				avgK,
				float64(len(winEntries))/float64(len(loseEntries)),
				t.gameConfig.Get(CfgMatchLossRatio),
				t.gameConfig.Get(CfgMatchLossFactorCap),
			)
			calc.Username = entry.UserName

			// Participation modifier
			t.mu.Lock()
			p := t.players[entry.PlayerID]
			roundsPlayed := len(p.Rounds)
			t.mu.Unlock()

			participationRatio := float64(roundsPlayed) / float64(totalRounds)
			calc.ParticipationRatio = participationRatio
			adjustedLoss := int(math.Round(float64(calc.ActualLoss) * participationRatio))
			adjustedLoss = max(min(adjustedLoss, playerScore), 0)
			calc.ActualLoss = adjustedLoss

			losses = append(losses, calc)

			if calc.ActualLoss > 0 {
				if err := data.UpsertSkirmishWin(ctx, entry.PlayerID, -calc.ActualLoss, 0, 0); err != nil {
					t.logger.Printf("upsert loss failed for %s: %v", entry.PlayerID, err)
				}
			}
		}

		// Persist match (goroutine)
		go func() {
			matchCtx := context.Background()
			participants := make([]data.MatchParticipant, 0)
			for playerID, p := range persistPlayers {
				if p.Team > 0 {
					roundsWon := 0
					if p.Team == winningTeam {
						roundsWon = len(p.Rounds)
					}
					participants = append(participants, data.MatchParticipant{
						PlayerID:  playerID,
						Team:      p.Team,
						RoundsWon: roundsWon,
					})
				}
			}
			match := data.Match{
				GameMode:   "skirmish",
				Map:        persistMap,
				StartedAt:  persistStart,
				EndedAt:    time.Now(),
				Team1Score: int(teamScoresCopy[1]),
				Team2Score: int(teamScoresCopy[2]),
			}
			if _, err := data.InsertMatch(matchCtx, match, participants); err != nil {
				t.logger.Printf("failed to insert match: %v", err)
			}
		}()

		t.weightProvider.Refresh(ctx)
	}

	t.logger.Printf("round %d: %d win bonuses, %d losses (K=%.0f)", roundNum, len(winResults), len(losses), avgK)

	if dc != nil && t.eventsChannel != "" {
		go t.sendEmbed(dc, roundNum, winningTeam, len(winEntries), len(loseEntries), winResults, losses, isMatchOver, teamScoresCopy)
	}
}

func (t *SkirmishTracker) OnPlayerDisconnect(playerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != skirmishInProgress {
		return
	}

	if p, ok := t.players[playerID]; ok && p.QuitAtRound == 0 {
		p.QuitAtRound = t.currentRound
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

func formatLossesTable(losses []MatchLossCalc) string {
	if len(losses) == 0 {
		return "No losses"
	}
	var sb strings.Builder
	sb.WriteString("```\n")
	for _, loss := range losses {
		name := loss.Username
		if len(name) > 16 {
			name = name[:16]
		}
		pct := int(math.Round(loss.ParticipationRatio * 100))
		fmt.Fprintf(&sb, "%-16s factor=%.2f size÷%.2f p=%d%% → -%d\n", name, loss.LossFactor, loss.SizeFactor, pct, loss.ActualLoss)
	}
	sb.WriteString("```")
	return sb.String()
}

func (t *SkirmishTracker) sendEmbed(dc *discordgo.Session, roundNum int, winningTeam int, winSize int, loseSize int, winResults []roundResult, losses []MatchLossCalc, isMatchOver bool, teamScores map[int]float64) {
	color := 0x57F287
	avgK := math.Max(t.weightProvider.AvgScore(), scoreWeightFloor)
	winMod := t.gameConfig.Get(CfgSkirmishRoundWinMod)
	matchWinMod := t.gameConfig.Get(CfgSkirmishMatchWinMod)
	maxSizeFactor := t.gameConfig.Get(CfgSkirmishSizeFactorCap)
	lossRatio := t.gameConfig.Get(CfgMatchLossRatio)
	lossFactorCap := t.gameConfig.Get(CfgMatchLossFactorCap)

	var title, description string
	if isMatchOver {
		var losingScore float64
		for teamID, score := range teamScores {
			if teamID != winningTeam {
				losingScore = score
				break
			}
		}
		winSizeFactor := float64(loseSize) / float64(winSize)
		if winSizeFactor > maxSizeFactor {
			winSizeFactor = maxSizeFactor
		}
		loseSizeFactor := float64(winSize) / float64(loseSize)
		if loseSizeFactor > maxSizeFactor {
			loseSizeFactor = maxSizeFactor
		}
		scoreFactor := 0.5 + 0.5*(teamScores[winningTeam]-losingScore)/math.Max(teamScores[winningTeam], 1.0)

		title = fmt.Sprintf("🏆 Match over — Team %d wins!", winningTeam)
		description = fmt.Sprintf("**Score:** %d – %.0f | **Size:** %.2f/%.2f | **Margin:** %.2f\n**Mods:** win=%.2f | loss_ratio=%.2f | max_factor=%.2f | **K:** %.0f",
			roundNum, losingScore, winSizeFactor, loseSizeFactor, scoreFactor, matchWinMod,
			lossRatio, lossFactorCap, avgK)
	} else {
		winSizeFactor := float64(loseSize) / float64(winSize)
		if winSizeFactor > maxSizeFactor {
			winSizeFactor = maxSizeFactor
		}
		title = fmt.Sprintf("⚔️ Round %d — Team %d wins", roundNum, winningTeam)
		description = fmt.Sprintf("**Mod:** %.2f | **Team balance:** %.2f | **K:** %.0f", winMod, winSizeFactor, avgK)
	}

	fields := []*discordgo.MessageEmbedField{
		{
			Name:  fmt.Sprintf("🏅 Team %d bonuses", winningTeam),
			Value: formatResultsTable(winResults),
		},
	}

	if isMatchOver && len(losses) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  "📉 Match Losses",
			Value: formatLossesTable(losses),
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
