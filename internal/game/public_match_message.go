package game

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/parse"
	"github.com/bwmarrin/discordgo"
	"github.com/jedib0t/go-pretty/v6/table"
)

type playerTableRow struct {
	playerID     string
	name         string
	kills        int
	deaths       int
	assists      int
	partPct      int
	total        int
	initialScore int
}

func (t *SkirmishTracker) sendPublicMatchEndMessage(
	dc *discordgo.Session,
	winningTeam int,
	totalRounds int,
	players map[string]*SkirmishPlayer,
	quitters []quitterRecord,
	scoreboardByID map[string]*parse.ScoreboardEntry,
) {
	if dc == nil || t.publicEventsChannel == "" {
		return
	}

	// Build per-team rows
	var team1, team2 []playerTableRow
	for id, p := range players {
		if p.Team == 0 {
			continue
		}
		kills := p.GetTotalKills()
		deaths := p.GetTotalDeaths()
		assists := p.GetTotalAssists()
		if sb, ok := scoreboardByID[id]; ok {
			kills = sb.Kills
			deaths = sb.Deaths
			assists = sb.Assists
		}
		row := playerTableRow{
			playerID:     id,
			name:         p.Name,
			kills:        kills,
			deaths:       deaths,
			assists:      assists,
			partPct:      int(math.Round(100.0 * p.GetParticipationRatio(totalRounds))),
			total:        p.GetTotalScore(),
			initialScore: p.InitialScore,
		}
		if p.Team == 1 {
			team1 = append(team1, row)
		} else if p.Team == 2 {
			team2 = append(team2, row)
		}
	}

	sort.Slice(team1, func(i, j int) bool { return team1[i].total > team1[j].total })
	sort.Slice(team2, func(i, j int) bool { return team2[i].total > team2[j].total })

	// MVP (highest score) across both teams
	all := slices.Concat(team1, team2)
	sort.Slice(all, func(i, j int) bool { return all[i].total > all[j].total })

	mvpName, mvpID := "-", ""
	if len(all) > 0 {
		mvpName = all[0].name
		mvpID = all[0].playerID
	}

	// SVP (most assists) across both teams, excluding MVP, requiring at least 2 assists
	svpName := ""
	maxAssists := -1
	for _, row := range all {
		if row.playerID == mvpID {
			continue
		}
		if row.assists >= 2 && row.assists > maxAssists {
			maxAssists = row.assists
			svpName = row.name
		}
	}

	timeStr := fmt.Sprintf("<t:%d:f>", time.Now().Unix())

	var msg strings.Builder
	fmt.Fprintf(&msg, "## MATCH OVER %s · TEAM %d WINS!\n", timeStr, winningTeam)
	if svpName != "" {
		fmt.Fprintf(&msg, "### MVP: %s · SVP: %s\n\n", mvpName, svpName)
	} else {
		fmt.Fprintf(&msg, "### MVP: %s\n\n", mvpName)
	}

	fmt.Fprintf(&msg, "### TEAM 1\n%s\n", buildTeamTable(team1))
	fmt.Fprintf(&msg, "### TEAM 2\n%s\n", buildTeamTable(team2))

	penalizedQuitters := make([]quitterRecord, 0, len(quitters))
	for _, q := range quitters {
		if q.penalty > 0 {
			penalizedQuitters = append(penalizedQuitters, q)
		}
	}
	if len(penalizedQuitters) > 0 {
		fmt.Fprintf(&msg, "### Hall of Shame\n*(Players that abandoned their losing teams, either by logging off or switching sides)*\n%s\n", buildHallOfShameTable(penalizedQuitters))
	}

	if _, err := dc.ChannelMessageSend(t.publicEventsChannel, msg.String()); err != nil {
		t.logger.Printf("failed to send public match end message: %v", err)
	}
}

func buildTeamTable(rows []playerTableRow) string {
	tw := table.NewWriter()
	tw.AppendHeader(table.Row{"Name", "K", "D", "A", "Part", "Pts", "Total"})
	for _, row := range rows {
		name := row.name
		if len(name) > 16 {
			name = name[:16]
		}
		partPctStr := fmt.Sprintf("%d%%", row.partPct)
		tw.AppendRow(table.Row{name, row.kills, row.deaths, row.assists, partPctStr, row.initialScore, fmt.Sprintf("%+d", row.total)})
	}
	tw.SetStyle(table.StyleLight)
	tw.Style().Options.DrawBorder = false
	tw.Style().Options.SeparateRows = false
	return fmt.Sprintf("```\n%s\n```", tw.Render())
}

func buildHallOfShameTable(quitters []quitterRecord) string {
	tw := table.NewWriter()
	tw.AppendHeader(table.Row{"Name", "Team", "Quit@", "Penalty"})
	for _, q := range quitters {
		name := q.username
		if len(name) > 16 {
			name = name[:16]
		}
		penalty := "-"
		if q.penalty > 0 {
			penalty = fmt.Sprintf("-%d", q.penalty)
		}
		quitRoundStr := fmt.Sprintf("Rnd%d", q.quitRound)
		tw.AppendRow(table.Row{name, q.team, quitRoundStr, penalty})
	}
	tw.SetStyle(table.StyleLight)
	tw.Style().Options.DrawBorder = false
	tw.Style().Options.SeparateRows = false
	return fmt.Sprintf("```\n%s\n```", tw.Render())
}
