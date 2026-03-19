package parse

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vjeantet/grok"
)

var reScorefeedTeam = regexp.MustCompile(`^Scorefeed: \d{4}\.\d{2}\.\d{2}-\d{2}\.\d{2}\.\d{2}: Team`)
var reKillfeedAssist = regexp.MustCompile(`^Killfeed: \d{4}\.\d{2}\.\d{2}-\d{2}\.\d{2}\.\d{2}: \S+ \(.+\) got an assist`)

const (
	GrokKillfeedEvent   = `%{WORD:event_type}: %{NOTSPACE:date}: (?:%{NOTSPACE:killer_id})? \(%{GREEDYDATA:user_name}\) killed (?:%{NOTSPACE:killed_id})? \(%{GREEDYDATA:killed_user_name}\)`
	GrokKillfeedAssist  = `%{WORD:event_type}: %{NOTSPACE:date}: %{NOTSPACE:killer_id} \(%{DATA:user_name}\) got an assist kill for the death of %{NOTSPACE:killed_id} \(%{DATA:killed_user_name}\)`
	GrokLoginEvent      = `%{WORD:event_type}: %{NOTSPACE:date}: %{GREEDYDATA:user_name} \(%{WORD:player_id}\) logged %{WORD:instance}`
	DateFormat          = "2006.01.02-15.04.05"
	GrokChatEvent       = `%{WORD:event_type}: %{NOTSPACE:player_id}, %{GREEDYDATA:user_name}, \(%{WORD:channel}\) %{GREEDYDATA:message}`
	GrokServerInfo      = `HostName: %{GREEDYDATA:host}\nServerName: %{GREEDYDATA:server_name}\nVersion: %{GREEDYDATA:version}\nGameMode: %{GREEDYDATA:game_mode}\nMap: %{GREEDYDATA:map}`
	GrokPlayerlistRow   = `%{NOTSPACE:player_id}, %{GREEDYDATA:user_name}, %{GREEDYDATA}, %{GREEDYDATA}`
	GrokMatchstate      = `MatchState: %{GREEDYDATA:state}`
	GrokScoreboardRow   = `%{NOTSPACE:player_id}, %{DATA:user_name}, %{NUMBER:team_id}, %{NUMBER}, %{NUMBER:score}, %{NUMBER:kills}, %{NUMBER:deaths}, %{NUMBER:assists}`
	GrokScorefeedPlayer = `Scorefeed: %{NOTSPACE:date}: %{NOTSPACE:player_id} \(%{DATA:user_name}\)'s score changed by %{NUMBER:score_change} points and is now %{NUMBER:new_score} points`
	GrokScorefeedTeam   = `Scorefeed: %{NOTSPACE:date}: Team %{NUMBER:team_id}'s is now %{NUMBER:new_score} points from %{NUMBER:old_score} points`
)

var g *grok.Grok

func init() {
	var err error
	g, err = grok.NewWithConfig(&grok.Config{NamedCapturesOnly: true})
	if err != nil {
		panic(err)
	}
}

func parseEvent(event, pattern string) (map[string]string, error) {
	values, err := g.Parse(pattern, event)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}

func ParseKillfeedEvent(event string) (*KillfeedEvent, error) {
	isAssist := reKillfeedAssist.MatchString(event)
	pattern := GrokKillfeedEvent
	if isAssist {
		pattern = GrokKillfeedAssist
	}
	values, err := parseEvent(event, pattern)
	if err != nil || values == nil {
		return nil, err
	}
	return &KillfeedEvent{
		EventType:      values["event_type"],
		Date:           values["date"],
		KillerID:       values["killer_id"],
		UserName:       values["user_name"],
		KilledID:       values["killed_id"],
		KilledUserName: values["killed_user_name"],
		IsAssist:       isAssist,
	}, nil
}

func ParseLoginEvent(event string) (*LoginEvent, error) {
	values, err := parseEvent(event, GrokLoginEvent)
	if err != nil || values == nil {
		return nil, err
	}
	return &LoginEvent{
		EventType: values["event_type"],
		Date:      values["date"],
		UserName:  values["user_name"],
		PlayerID:  values["player_id"],
		Instance:  values["instance"],
	}, nil
}

func ParseChatEvent(event string) (*ChatEvent, error) {
	flat := strings.Join(strings.Split(event, "\n"), ` \ `)
	values, err := parseEvent(flat, GrokChatEvent)
	if err != nil || values == nil {
		return nil, err
	}
	return &ChatEvent{
		EventType: values["event_type"],
		PlayerID:  values["player_id"],
		UserName:  values["user_name"],
		Channel:   values["channel"],
		Message:   values["message"],
	}, nil
}

func ParseDate(dateStr string) (time.Time, error) {
	return time.Parse(DateFormat, dateStr)
}

func ParseServerInfo(raw string) (*ServerInfo, error) {
	values, err := parseEvent(raw, GrokServerInfo)
	if err != nil || values == nil {
		return nil, err
	}
	return &ServerInfo{
		Host:       values["host"],
		ServerName: values["server_name"],
		Version:    values["version"],
		// GameMode:   values["game_mode"],
		GameMode: "Skirmish",
		Map:      values["map"],
	}, nil
}

func ParseScoreboardRow(raw string) (*ScoreboardEntry, error) {
	values, err := parseEvent(raw, GrokScoreboardRow)
	if err != nil || values == nil {
		return nil, err
	}
	teamID, _ := strconv.Atoi(values["team_id"])
	teamID++
	score, _ := strconv.Atoi(values["score"])
	kills, _ := strconv.Atoi(values["kills"])
	deaths, _ := strconv.Atoi(values["deaths"])
	assists, _ := strconv.Atoi(values["assists"])
	return &ScoreboardEntry{
		PlayerID: values["player_id"],
		UserName: values["user_name"],
		TeamID:   teamID,
		Score:    score,
		Kills:    kills,
		Deaths:   deaths,
		Assists:  assists,
	}, nil
}

func ParseScoreboard(raw string) ([]*ScoreboardEntry, error) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	entries := make([]*ScoreboardEntry, 0, len(lines))
	for _, line := range lines {
		entry, err := ParseScoreboardRow(line)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func ParseScorefeedPlayerEvent(raw string) (*ScorefeedPlayerEvent, error) {
	values, err := parseEvent(raw, GrokScorefeedPlayer)
	if err != nil || values == nil {
		return nil, err
	}
	scoreChange, _ := strconv.ParseFloat(values["score_change"], 64)
	newScore, _ := strconv.ParseFloat(values["new_score"], 64)
	return &ScorefeedPlayerEvent{
		Date:        values["date"],
		PlayerID:    values["player_id"],
		UserName:    values["user_name"],
		ScoreChange: scoreChange,
		NewScore:    newScore,
	}, nil
}

func ParseScorefeedTeamEvent(raw string) (*ScorefeedTeamEvent, error) {
	values, err := parseEvent(raw, GrokScorefeedTeam)
	if err != nil || values == nil {
		return nil, err
	}
	teamID, _ := strconv.Atoi(values["team_id"])
	newScore, _ := strconv.ParseFloat(values["new_score"], 64)
	oldScore, _ := strconv.ParseFloat(values["old_score"], 64)
	return &ScorefeedTeamEvent{
		Date:     values["date"],
		TeamID:   teamID,
		NewScore: newScore,
		OldScore: oldScore,
	}, nil
}

func ParseScorefeedEvent(raw string) (*ScorefeedPlayerEvent, *ScorefeedTeamEvent, error) {
	if reScorefeedTeam.MatchString(raw) {
		event, err := ParseScorefeedTeamEvent(raw)
		return nil, event, err
	}
	event, err := ParseScorefeedPlayerEvent(raw)
	return event, nil, err
}

func ParseMatchstate(raw string) (string, error) {
	values, err := parseEvent(raw, GrokMatchstate)
	if err != nil || values == nil {
		return "", err
	}
	return values["state"], nil
}
