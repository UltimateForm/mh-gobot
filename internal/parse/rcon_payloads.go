package parse

import (
	"strconv"
	"strings"
	"time"

	"github.com/vjeantet/grok"
)

const (
	GrokKillfeedEvent = `%{WORD:event_type}: %{NOTSPACE:date}: (?:%{NOTSPACE:killer_id})? \(%{GREEDYDATA:user_name}\) killed (?:%{NOTSPACE:killed_id})? \(%{GREEDYDATA:killed_user_name}\)`
	GrokLoginEvent    = `%{WORD:event_type}: %{NOTSPACE:date}: %{GREEDYDATA:user_name} \(%{WORD:player_id}\) logged %{WORD:instance}`
	DateFormat        = "2006.01.02-15.04.05"
	GrokChatEvent     = `%{WORD:event_type}: %{NOTSPACE:player_id}, %{GREEDYDATA:user_name}, \(%{WORD:channel}\) %{GREEDYDATA:message}`
	GrokServerInfo    = `HostName: %{GREEDYDATA:host}\nServerName: %{GREEDYDATA:server_name}\nVersion: %{GREEDYDATA:version}\nGameMode: %{GREEDYDATA:game_mode}\nMap: %{GREEDYDATA:map}`
	GrokPlayerlistRow = `%{NOTSPACE:player_id}, %{GREEDYDATA:user_name}, %{GREEDYDATA}, %{GREEDYDATA}`
	GrokMatchstate    = `MatchState: %{GREEDYDATA:state}`
	GrokScoreboardRow = `%{NOTSPACE:player_id}, %{DATA:user_name}, %{NUMBER}, %{NUMBER}, %{NUMBER:score}, %{NUMBER:kills}, %{NUMBER:deaths}, %{NUMBER}`
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
	values, err := parseEvent(event, GrokKillfeedEvent)
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
		GameMode:   values["game_mode"],
		Map:        values["map"],
	}, nil
}

func ParseScoreboardRow(raw string) (*ScoreboardEntry, error) {
	values, err := parseEvent(raw, GrokScoreboardRow)
	if err != nil || values == nil {
		return nil, err
	}
	score, _ := strconv.Atoi(values["score"])
	kills, _ := strconv.Atoi(values["kills"])
	deaths, _ := strconv.Atoi(values["deaths"])
	return &ScoreboardEntry{
		PlayerID: values["player_id"],
		UserName: values["user_name"],
		Score:    score,
		Kills:    kills,
		Deaths:   deaths,
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

func ParseMatchstate(raw string) (string, error) {
	values, err := parseEvent(raw, GrokMatchstate)
	if err != nil || values == nil {
		return "", err
	}
	return values["state"], nil
}
