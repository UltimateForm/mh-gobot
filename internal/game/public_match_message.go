package game

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jedib0t/go-pretty/v6/table"
)

type playerTableRow struct {
	name    string
	kills   int
	deaths  int
	assists int
	partPct int
	total   int
}

func (t *SkirmishTracker) sendPublicMatchEndMessage(
	dc *discordgo.Session,
	winningTeam int,
	totalRounds int,
	players map[string]*SkirmishPlayer,
	quitters []quitterRecord,
) {
	if dc == nil || t.publicEventsChannel == "" {
		return
	}

	// Build per-team rows
	var team1, team2 []playerTableRow
	for _, p := range players {
		if p.Team == 0 {
			continue
		}
		row := playerTableRow{
			name:    p.Name,
			kills:   p.GetTotalKills(),
			deaths:  p.GetTotalDeaths(),
			assists: p.GetTotalAssists(),
			partPct: int(math.Round(100.0 * p.GetParticipationRatio(totalRounds))),
			total:   p.GetTotalScore(),
		}
		if p.Team == 1 {
			team1 = append(team1, row)
		} else if p.Team == 2 {
			team2 = append(team2, row)
		}
	}

	sort.Slice(team1, func(i, j int) bool { return team1[i].total > team1[j].total })
	sort.Slice(team2, func(i, j int) bool { return team2[i].total > team2[j].total })

	// MVP/SVP across both teams
	all := slices.Concat(team1, team2)
	sort.Slice(all, func(i, j int) bool { return all[i].total > all[j].total })

	mvpName, svpName := "—", "—"
	if len(all) > 0 {
		mvpName = all[0].name
	}
	if len(all) > 1 {
		svpName = all[1].name
	}

	timeStr := fmt.Sprintf("<t:%d:f>", time.Now().Unix())

	var msg strings.Builder
	fmt.Fprintf(&msg, "## MATCH OVER %s · TEAM %d WINS!\n", timeStr, winningTeam)
	fmt.Fprintf(&msg, "### MVP: %s · SVP: %s\n\n", mvpName, svpName)

	fmt.Fprintf(&msg, "### TEAM 1\n%s\n", buildTeamTable(team1))
	fmt.Fprintf(&msg, "### TEAM 2\n%s\n", buildTeamTable(team2))

	if len(quitters) > 0 {
		fmt.Fprintf(&msg, "#### Hall of Shame\n%s\n", buildHallOfShameTable(quitters))
	}

	if _, err := dc.ChannelMessageSend(t.publicEventsChannel, msg.String()); err != nil {
		t.logger.Printf("failed to send public match end message: %v", err)
	}
}

func buildTeamTable(rows []playerTableRow) string {
	tw := table.NewWriter()
	tw.AppendHeader(table.Row{"Name", "K", "D", "A", "Part", "Total"})
	for _, row := range rows {
		name := row.name
		if len(name) > 16 {
			name = name[:16]
		}
		partPctStr := fmt.Sprintf("%d%%", row.partPct)
		tw.AppendRow(table.Row{name, row.kills, row.deaths, row.assists, partPctStr, fmt.Sprintf("%+d", row.total)})
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
		penalty := "—"
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
