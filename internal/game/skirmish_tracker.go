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

var matchSizeMultipliers = map[int]float64{
	1: 0.00,
	2: 0.10,
	3: 0.45,
	4: 0.80,
	5: 1.00,
	6: 0.90,
	7: 0.50,
	8: 0.50,
}

func matchSizeMult(teamSize int) float64 {
	if m, ok := matchSizeMultipliers[min(teamSize, 8)]; ok {
		return m
	}
	return matchSizeMultipliers[8]
}

type roundResult struct {
	playerID     string
	username     string
	delta        float64
	bonus        int
	comebackMult float64
}

type matchWinResult struct {
	playerID string
	username string
	bonus    int
	share    float64
}

type quitterRecord struct {
	playerID  string
	username  string
	team      int
	quitRound int
	penalty   int
}

type SkirmishTracker struct {
	mu                        sync.Mutex
	state                     skirmishState
	currentRound              int
	players                   map[string]*SkirmishPlayer
	firstKillOfMatchApplied   bool
	teamScores          map[int]float64
	roundAliveCounts    map[int]int
	roundPeakDeficit    map[int]int
	matchRounds         []SkirmishMatchRound
	matchStartedAt      time.Time
	matchMap            string
	winCap              float64
	pool                *rcon_client.ConnectionPool
	eventsChannel       string
	publicEventsChannel string
	weightProvider      *ScoreWeightProvider
	gameConfig          *GameConfig
	logger              *log.Logger
	quitters            []quitterRecord
}

func NewSkirmishTracker(pool *rcon_client.ConnectionPool, eventsChannel string, publicEventsChannel string, winCap float64, wp *ScoreWeightProvider, gc *GameConfig) *SkirmishTracker {
	return &SkirmishTracker{
		state:               skirmishIdle,
		currentRound:        0,
		players:             make(map[string]*SkirmishPlayer),
		teamScores:          make(map[int]float64),
		roundAliveCounts:    make(map[int]int),
		roundPeakDeficit:    make(map[int]int),
		matchRounds:         make([]SkirmishMatchRound, 0),
		winCap:              winCap,
		pool:                pool,
		eventsChannel:       eventsChannel,
		publicEventsChannel: publicEventsChannel,
		weightProvider:      wp,
		gameConfig:          gc,
		logger:              log.New(log.Default().Writer(), "[SkirmishTracker] ", log.Default().Flags()),
		quitters:            make([]quitterRecord, 0),
	}
}

func (t *SkirmishTracker) clearMatch() {
	t.currentRound = 0
	t.players = make(map[string]*SkirmishPlayer)
	t.teamScores = make(map[int]float64)
	t.roundAliveCounts = make(map[int]int)
	t.roundPeakDeficit = make(map[int]int)
	t.matchRounds = make([]SkirmishMatchRound, 0)
	t.matchStartedAt = time.Time{}
	t.matchMap = ""
	t.quitters = make([]quitterRecord, 0)
	t.firstKillOfMatchApplied = false
}

func (t *SkirmishTracker) TeamScores() map[int]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[int]int, len(t.teamScores))
	for k, v := range t.teamScores {
		out[k] = int(v)
	}
	return out
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

// stampIfNeeded seeds a player's InitialScore/LiveScore from the DB once, asynchronously,
// the first time they're seen in a match. No-op if already stamped.
func (t *SkirmishTracker) stampIfNeeded(playerID string, alreadyStamped bool) {
	if alreadyStamped {
		return
	}
	go func() {
		ctx := context.Background()
		if dbPlayer, err := data.ReadPlayer(ctx, playerID); err == nil {
			t.mu.Lock()
			if pp, ok := t.players[playerID]; ok {
				pp.StampInitialScore(dbPlayer.Score)
			}
			t.mu.Unlock()
		}
	}()
}

func (t *SkirmishTracker) applyScoreDelta(playerID string, delta int, label string) {
	go func() {
		if err := data.AddPlayerScore(context.Background(), playerID, delta); err != nil {
			t.logger.Printf("%s failed for %s: %v", label, playerID, err)
		}
	}()
}

func (t *SkirmishTracker) OnPlayerScore(e *parse.ScorefeedPlayerEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != skirmishInProgress {
		return
	}

	p := t.getOrInitPlayer(e.PlayerID, e.UserName)
	delta := int(e.ScoreChange)
	if delta > 0 {
		weight := ScoreWeight(p.LiveScore, t.weightProvider.AvgScore())
		delta = int(math.Round(float64(e.ScoreChange) * weight))
	}

	perf := p.Rounds[t.currentRound]
	perf.Score += delta
	p.Rounds[t.currentRound] = perf
	p.LiveScore += delta

	t.stampIfNeeded(e.PlayerID, p.initialScoreStamped)
	t.applyScoreDelta(e.PlayerID, delta, "score update")
}

func (t *SkirmishTracker) OnKill(e *parse.KillfeedEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != skirmishInProgress {
		return
	}

	killer := t.getOrInitPlayer(e.KillerID, e.UserName)
	victim := t.getOrInitPlayer(e.KilledID, e.KilledUserName)
	t.stampIfNeeded(e.KillerID, killer.initialScoreStamped)
	t.stampIfNeeded(e.KilledID, victim.initialScoreStamped)

	baseline := 100.0
	if e.IsAssist {
		baseline = 50.0
	}
	
	avgScore := t.weightProvider.AvgScore()
	kf := KillFactor(victim.LiveScore, killer.LiveScore, math.Max(avgScore, scoreWeightFloor))
	weight := ScoreWeight(killer.LiveScore, avgScore)
	topUp := int(math.Round(baseline * weight * (kf - 1)))

	perf := killer.Rounds[t.currentRound]
	if e.IsAssist {
		perf.Assists++
	} else {
		perf.Kills++
		perf.KilledIds = append(perf.KilledIds, e.KilledID)
	}
	perf.Score += topUp
	killer.Rounds[t.currentRound] = perf
	killer.LiveScore += topUp
	if topUp != 0 {
		t.applyScoreDelta(e.KillerID, topUp, "kill factor adjustment")
	}

	if e.IsAssist {
		return
	}

	if !t.firstKillOfMatchApplied {
		t.firstKillOfMatchApplied = true
		bonus := int((t.gameConfig.Get(CfgFirstKillBonusFactor) - 1) * 100)
		if bonus > 0 {
			t.applyScoreDelta(e.KillerID, bonus, "first kill bonus")
		}
	}

	victimPerf := victim.Rounds[t.currentRound]
	victimPerf.Deaths++
	victim.Rounds[t.currentRound] = victimPerf

	victimTeam := victim.Team
	if victimTeam > 0 {
		t.roundAliveCounts[victimTeam]--
		for teamID := 1; teamID <= 2; teamID++ {
			deficit := t.roundAliveCounts[3-teamID] - t.roundAliveCounts[teamID]
			if deficit > t.roundPeakDeficit[teamID] {
				t.roundPeakDeficit[teamID] = deficit
			}
		}
	}
}

func (t *SkirmishTracker) OnTeamScore(ctx context.Context, dc *discordgo.Session, e *parse.ScorefeedTeamEvent) {
	t.mu.Lock()
	if t.state != skirmishInProgress {
		// Bot isn't actively tracking this match (e.g. mid-match restart).
		// Capture the score so the pop embed can show it; skip round-end processing.
		t.teamScores[e.TeamID] = e.NewScore
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

	validEntries := make([]*parse.ScoreboardEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.TeamID > 0 {
			validEntries = append(validEntries, entry)
		} else {
			// Check for players in spectator mode (TeamID==0 but still tracked)
			t.mu.Lock()
			if p, tracked := t.players[entry.PlayerID]; tracked && p.Team != 0 {
				t.mu.Unlock()
				t.logger.Printf("player %s entered spectator mode (was team %d)", entry.PlayerID, p.Team)
				t.OnPlayerLogout(ctx, &parse.LoginEvent{PlayerID: entry.PlayerID})
			} else {
				t.mu.Unlock()
			}
		}
	}

	var winEntries, loseEntries []*parse.ScoreboardEntry
	for _, entry := range validEntries {
		if entry.TeamID == winningTeam {
			winEntries = append(winEntries, entry)
		} else {
			loseEntries = append(loseEntries, entry)
		}
	}
	t.logger.Printf("round end: team %d wins, %d winners, %d losers on scoreboard", winningTeam, len(winEntries), len(loseEntries))

	t.mu.Lock()

	// Update team assignments and ensure round entries for all present players
	for _, entry := range validEntries {
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

	// Batch read player scores for loss calc and end-of-round stamp sync
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

	t.mu.Lock()
	for id, p := range t.players {
		if s, ok := playerScores[id]; ok {
			p.StampInitialScore(s)
		}
	}
	t.mu.Unlock()

	avgK := math.Max(t.weightProvider.AvgScore(), scoreWeightFloor)

	// Win bonuses — weight and kf are now applied live in OnPlayerScore/OnKill,
	// so rd already reflects fairness-adjusted round contribution.
	peakDeficit := t.roundPeakDeficit[winningTeam]
	comebackMult := 1.0
	if peakDeficit > 0 {
		comebackMult = float64(peakDeficit) * t.gameConfig.Get(CfgComebackBonusFactor)
	}

	winResults := make([]roundResult, 0, len(winEntries))
	for _, entry := range winEntries {
		t.mu.Lock()
		p := t.players[entry.PlayerID]
		rd := float64(p.Rounds[t.currentRound].Score)
		isSurvivor := p.Rounds[t.currentRound].Deaths == 0
		t.mu.Unlock()

		winMod := t.gameConfig.Get(CfgSkirmishRoundWinMod)
		cm := 1.0
		if isSurvivor && peakDeficit > 0 {
			cm = comebackMult
		}
		b := int(math.Round(rd * winMod * cm))

		if b > 0 {
			matchesWon := 0
			if isMatchOver {
				matchesWon = 1
			}
			if err := data.UpsertSkirmishWin(ctx, entry.PlayerID, b, 1, matchesWon); err != nil {
				t.logger.Printf("upsert win failed for %s: %v", entry.PlayerID, err)
			}
			winResults = append(winResults, roundResult{entry.PlayerID, entry.UserName, rd, b, cm})
		}
	}

	t.mu.Lock()
	t.currentRound++
	t.roundAliveCounts = make(map[int]int)
	t.roundPeakDeficit = make(map[int]int)
	for _, p := range t.players {
		if p.Team > 0 {
			t.roundAliveCounts[p.Team]++
		}
	}
	t.mu.Unlock()

	// Loss calculation and match-end logic
	losses := make([]MatchLossCalc, 0)
	winBonuses := make([]matchWinResult, 0)
	sizeMult := 1.0
	teamBalanceFactor := 1.0
	if isMatchOver {
		t.mu.Lock()
		totalRounds := t.currentRound
		persistPlayers := t.players
		persistMap := t.matchMap
		persistStart := t.matchStartedAt
		quittersCopy := make([]quitterRecord, len(t.quitters))
		copy(quittersCopy, t.quitters)
		t.mu.Unlock()

		matchSize := min(len(winEntries), len(loseEntries))
		sizeMult = matchSizeMult(matchSize)

		avgTeamScore := func(entries []*parse.ScoreboardEntry) float64 {
			sum, n := 0, 0
			for _, e := range entries {
				if s := playerScores[e.PlayerID]; s > 0 {
					sum += s
					n++
				}
			}
			if n == 0 {
				return 0
			}
			return float64(sum) / float64(n)
		}
		avgWin := avgTeamScore(winEntries)
		avgLose := avgTeamScore(loseEntries)
		if avgWin > 0 {
			minF := t.gameConfig.Get(CfgTeamBalanceMinFactor)
			maxF := t.gameConfig.Get(CfgTeamBalanceMaxFactor)
			teamBalanceFactor = math.Min(math.Max(avgLose/avgWin, minF), maxF)
		}


		for _, entry := range loseEntries {
			playerScore := playerScores[entry.PlayerID]
			calc := ComputeMatchLoss(
				entry.PlayerID,
				playerScore,
				avgK,
				MatchLossSizeFactor(len(winEntries), len(loseEntries)),
				t.gameConfig.Get(CfgMatchLossRatio),
				t.gameConfig.Get(CfgMatchLossFactorCap),
			)
			calc.Username = entry.UserName

			// Participation modifier
			t.mu.Lock() // potentially overkill adding lock here
			p := t.players[entry.PlayerID]
			roundsPlayed := len(p.Rounds)
			t.mu.Unlock()

			participationRatio := float64(roundsPlayed) / float64(totalRounds)
			calc.ParticipationRatio = participationRatio
			adjustedLoss := int(math.Round(float64(calc.ActualLoss) * participationRatio * sizeMult * teamBalanceFactor))
			adjustedLoss = max(min(adjustedLoss, playerScore), 0) // forgot why min(adjustedLoss, playerScore)
			calc.ActualLoss = adjustedLoss

			losses = append(losses, calc)

			// Set MatchResultScore for losers
			t.mu.Lock()
			if p, ok := t.players[entry.PlayerID]; ok {
				p.MatchResultScore = -adjustedLoss
			}
			t.mu.Unlock()

			if calc.ActualLoss > 0 {
				if err := data.UpsertSkirmishWin(ctx, entry.PlayerID, -calc.ActualLoss, 0, 0); err != nil {
					t.logger.Printf("upsert loss failed for %s: %v", entry.PlayerID, err)
				}
			}
		}

		// Build match win bonus pool from loser losses
		totalPool := 0
		for _, calc := range losses {
			totalPool += calc.ActualLoss
		}

		if totalPool > 0 {
			// Compute participation weights for winners
			type winnerWeight struct {
				entry               *parse.ScoreboardEntry
				participationWeight float64
			}
			weights := make([]winnerWeight, 0, len(winEntries))
			weightSum := 0.0
			for _, entry := range winEntries {
				t.mu.Lock()
				p := t.players[entry.PlayerID]
				roundsPlayed := len(p.Rounds)
				t.mu.Unlock()
				w := float64(roundsPlayed) / float64(totalRounds)
				weights = append(weights, winnerWeight{entry, w})
				weightSum += w
			}

			for _, ww := range weights {
				if weightSum == 0 {
					break
				}
				normalizedShare := ww.participationWeight / weightSum
				bonus := int(math.Round(normalizedShare * float64(totalPool)))
				if bonus > 0 {
					if err := data.UpsertSkirmishWin(ctx, ww.entry.PlayerID, bonus, 0, 0); err != nil {
						t.logger.Printf("match win bonus upsert failed for %s: %v", ww.entry.PlayerID, err)
					}
				}
				winBonuses = append(winBonuses, matchWinResult{
					playerID: ww.entry.PlayerID,
					username: ww.entry.UserName,
					bonus:    bonus,
					share:    normalizedShare,
				})

				// Set MatchResultScore for winners
				t.mu.Lock()
				if p, ok := t.players[ww.entry.PlayerID]; ok {
					p.MatchResultScore = bonus
				}
				t.mu.Unlock()
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

		if dc != nil && t.publicEventsChannel != "" {
			scoreboardByID := make(map[string]*parse.ScoreboardEntry, len(winEntries)+len(loseEntries))
			for _, e := range winEntries {
				scoreboardByID[e.PlayerID] = e
			}
			for _, e := range loseEntries {
				scoreboardByID[e.PlayerID] = e
			}
			go t.sendPublicMatchEndMessage(dc, winningTeam, totalRounds, persistPlayers, quittersCopy, scoreboardByID)
		}

		t.mu.Lock()
		t.clearMatch()
		t.state = skirmishIdle
		t.mu.Unlock()
	}

	t.logger.Printf("round %d: %d win bonuses, %d losses (K=%.0f)", roundNum, len(winResults), len(losses), avgK)

	if dc != nil && t.eventsChannel != "" {
		go t.sendRoundEmbed(dc, roundNum, winningTeam, len(winEntries), len(loseEntries), winResults, peakDeficit)
		if isMatchOver {
			go t.sendMatchEndEmbed(dc, winningTeam, len(winEntries), len(loseEntries), losses, winBonuses, teamScoresCopy, sizeMult, teamBalanceFactor)
		}
	}
}

func (t *SkirmishTracker) OnPlayerLogout(ctx context.Context, e *parse.LoginEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != skirmishInProgress {
		return
	}

	player, ok := t.players[e.PlayerID]
	if !ok {
		return
	}

	// Record when the player quit
	if player.QuitAtRound == 0 {
		player.QuitAtRound = t.currentRound
	}

	// Check if team is losing
	team1Score := t.teamScores[1]
	team2Score := t.teamScores[2]
	losingTeamID := 0

	if team1Score < team2Score {
		losingTeamID = 1
	} else if team2Score < team1Score {
		losingTeamID = 2
	}

	penalty := 0

	if losingTeamID > 0 && player.Team == losingTeamID {
		dbPlayer, err := data.ReadPlayer(ctx, e.PlayerID)
		if err != nil {
			t.logger.Printf("failed to read player for logout penalty: %v", err)
		} else {
			winTeamID := 1
			if losingTeamID == 1 {
				winTeamID = 2
			}
			winSize, loseSize := 0, 0
			for _, p := range t.players {
				if p.Team == winTeamID {
					winSize++
				} else if p.Team == losingTeamID {
					loseSize++
				}
			}

			minTeamSize := int(t.gameConfig.Get(CfgQuitterPenaltyTeamMin))
			if loseSize < minTeamSize {
				t.logger.Printf("player logout: %s from losing team %d skipped penalty (team size %d < %d)", e.PlayerID, losingTeamID, loseSize, minTeamSize)
			} else {
				loss := ComputeMatchLoss(
					e.PlayerID,
					dbPlayer.Score,
					t.weightProvider.K(),
					MatchLossSizeFactor(winSize, loseSize),
					t.gameConfig.Get(CfgMatchLossRatio),
					t.gameConfig.Get(CfgMatchLossFactorCap),
				)

				sizeMult := matchSizeMult(min(winSize, loseSize))

				allIDs := make([]string, 0, winSize+loseSize)
				for pid, p := range t.players {
					if p.Team == winTeamID || p.Team == losingTeamID {
						allIDs = append(allIDs, pid)
					}
				}
				teamScores, _ := data.ReadPlayerScores(ctx, allIDs)
				avgTeam := func(teamID int) float64 {
					sum, n := 0, 0
					for pid, p := range t.players {
						if p.Team != teamID {
							continue
						}
						if s := teamScores[pid]; s > 0 {
							sum += s
							n++
						}
					}
					if n == 0 {
						return 0
					}
					return float64(sum) / float64(n)
				}
				avgWin := avgTeam(winTeamID)
				avgLose := avgTeam(losingTeamID)
				teamBalanceFactor := 1.0
				if avgWin > 0 {
					minF := t.gameConfig.Get(CfgTeamBalanceMinFactor)
					maxF := t.gameConfig.Get(CfgTeamBalanceMaxFactor)
					teamBalanceFactor = math.Min(math.Max(avgLose/avgWin, minF), maxF)
				}

				matchProgressFactor := math.Min(float64(t.currentRound)/t.winCap, 1.0)

				penalty = max(min(int(math.Round(float64(loss.ActualLoss)*sizeMult*teamBalanceFactor*matchProgressFactor)), dbPlayer.Score), 0)

				if penalty != 0 {
					if err := data.AddPlayerScore(ctx, e.PlayerID, -penalty); err != nil {
						t.logger.Printf("failed to apply match loss to %s: %v", e.PlayerID, err)
					} else {
						t.logger.Printf("player logout penalty: %s lost %d points (losing team %d, size_mult=%.2f, balance=%.2f, progress=%.2f)", e.PlayerID, penalty, losingTeamID, sizeMult, teamBalanceFactor, matchProgressFactor)
					}
				} else {
					t.logger.Printf("player logout: %s from losing team %d (no points to lose)", e.PlayerID, losingTeamID)
				}
			}
		}
	} else if losingTeamID > 0 {
		t.logger.Printf("player logout: %s from winning team %d (no penalty)", e.PlayerID, 3-losingTeamID)
	} else {
		t.logger.Printf("player logout: %s (teams tied)", e.PlayerID)
	}

	t.quitters = append(t.quitters, quitterRecord{
		playerID:  e.PlayerID,
		username:  player.Name,
		team:      player.Team,
		quitRound: player.QuitAtRound,
		penalty:   penalty,
	})

	delete(t.players, e.PlayerID)
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
		line := fmt.Sprintf("%-16s Δ%+.0f", name, r.delta)
		if r.comebackMult > 1.0 {
			line += fmt.Sprintf(" cm=%.2f", r.comebackMult)
		}
		line += fmt.Sprintf(" → +%d\n", r.bonus)
		sb.WriteString(line)
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

func formatMatchWinTable(results []matchWinResult) string {
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
		fmt.Fprintf(&sb, "%-16s share=%.0f%% → +%d\n", name, r.share*100, r.bonus)
	}
	sb.WriteString("```")
	return sb.String()
}

func (t *SkirmishTracker) sendRoundEmbed(dc *discordgo.Session, roundNum int, winningTeam int, winSize int, loseSize int, winResults []roundResult, peakDeficit int) {
	color := 0x57F287
	winMod := t.gameConfig.Get(CfgSkirmishRoundWinMod)
	maxSizeFactor := t.gameConfig.Get(CfgSkirmishSizeFactorCap)

	winSizeFactor := float64(loseSize) / float64(winSize)
	if winSizeFactor > maxSizeFactor {
		winSizeFactor = maxSizeFactor
	}

	title := fmt.Sprintf("⚔️ Round %d - Team %d wins", roundNum, winningTeam)
	description := fmt.Sprintf("**Round Win Mod:** %.2f | **Team Balance Mod:** %.2f | **Peak Deficit:** %d", winMod, winSizeFactor, peakDeficit)

	fields := []*discordgo.MessageEmbedField{
		{
			Name:  fmt.Sprintf("🏅 Team %d win bonuses", winningTeam),
			Value: formatResultsTable(winResults),
		},
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

func (t *SkirmishTracker) sendMatchEndEmbed(dc *discordgo.Session, winningTeam int, winSize int, loseSize int, losses []MatchLossCalc, winBonuses []matchWinResult, teamScores map[int]float64, sizeMult float64, teamBalanceFactor float64) {
	color := 0x57F287
	avgK := math.Max(t.weightProvider.AvgScore(), scoreWeightFloor)
	maxSizeFactor := t.gameConfig.Get(CfgSkirmishSizeFactorCap)
	lossRatio := t.gameConfig.Get(CfgMatchLossRatio)
	lossFactorCap := t.gameConfig.Get(CfgMatchLossFactorCap)

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

	title := fmt.Sprintf("🏆 Match over - Team %d wins!", winningTeam)
	description := fmt.Sprintf("**Score:** %.0f – %.0f | **Team size:** %.2f/%.2f (loss reduction) | **Margin:** %.2f\n**Mods:** loss_ratio=%.2f | max_factor=%.2f | **K:** %.0f | **Match size:** %.2f | **Rank balance:** %.2f",
		teamScores[winningTeam], losingScore, winSizeFactor, loseSizeFactor, scoreFactor,
		lossRatio, lossFactorCap, avgK, sizeMult, teamBalanceFactor)

	fields := []*discordgo.MessageEmbedField{}

	if len(losses) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  "📉 Match Losses",
			Value: formatLossesTable(losses),
		})
	}

	if len(winBonuses) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  "🏅 Match Win Bonuses",
			Value: formatMatchWinTable(winBonuses),
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Fields:      fields,
	}

	if _, err := dc.ChannelMessageSendEmbed(t.eventsChannel, embed); err != nil {
		t.logger.Printf("failed to send match end embed: %v", err)
	}
}
