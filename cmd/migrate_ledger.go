package cmd

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/UltimateForm/mh-gobot/internal/config"
	"github.com/bwmarrin/discordgo"
)

var reKillMsg = regexp.MustCompile(`💀 \*\*kill\*\* ` + "`[^`]+`" + ` \(([A-Z0-9]{14,16})\) → ` + "`[^`]+`" + ` \(([A-Z0-9]{14,16})\)`)

func afterSnowflake(t time.Time) string {
	const discordEpoch = int64(1420070400000) // ms, Jan 1 2015
	ms := t.UnixMilli()
	return strconv.FormatInt((ms-discordEpoch)<<22, 10)
}

// ExecMigrateLedger fetches kill messages from the events channel starting after
// afterMessageID and generates a SQL migration file for the kill_ledger table.
// Pass an actual Discord message ID as the starting point for reliable results.
// If afterMessageID is empty, falls back to a computed snowflake for the last 30h.
func ExecMigrateLedger(afterMessageID string) {
	if config.Global.EventsChannel == "" {
		log.Fatal("EVENTS_CHANNEL not set")
	}
	if config.Global.DcToken == "" {
		log.Fatal("DC_TOKEN not set")
	}

	dc, err := discordgo.New("Bot " + config.Global.DcToken)
	if err != nil {
		log.Fatalf("failed to create discord session: %v", err)
	}

	afterID := afterMessageID
	if afterID == "" {
		since := time.Now().Add(-30 * time.Hour)
		afterID = afterSnowflake(since)
		log.Printf("no message ID provided, fetching kill messages since %s", since.Format(time.RFC3339))
	} else {
		log.Printf("fetching kill messages after message ID %s", afterID)
	}

	// counts[killerID][killedID] = count
	counts := map[string]map[string]int{}

	total := 0
	beforeID := "" // start from newest, paginate backwards
	for {
		msgs, err := dc.ChannelMessages(config.Global.EventsChannel, 100, beforeID, "", "")
		if err != nil {
			log.Fatalf("failed to fetch messages: %v", err)
		}
		log.Printf("fetched batch of %d messages", len(msgs))

		done := false
		for _, msg := range msgs {
			if msg.ID == afterMessageID {
				log.Printf("found target message ID %s, stopping", afterMessageID)
				done = true
				break
			}
			m := reKillMsg.FindStringSubmatch(msg.Content)
			if m == nil {
				continue
			}
			killerID, killedID := m[1], m[2]
			if counts[killerID] == nil {
				counts[killerID] = map[string]int{}
			}
			counts[killerID][killedID]++
			total++
		}

		if done || len(msgs) < 100 {
			break
		}
		beforeID = msgs[len(msgs)-1].ID
		log.Printf("paginating, next before ID: %s, kills so far: %d", beforeID, total)
	}

	pairs := 0
	for _, v := range counts {
		pairs += len(v)
	}
	log.Printf("found %d kill events across %d unique killer->killed pairs", total, pairs)

	if total == 0 {
		log.Println("nothing to migrate")
		return
	}

	var sb strings.Builder
	sb.WriteString("-- kill_ledger migration generated at ")
	sb.WriteString(time.Now().Format(time.RFC3339))
	sb.WriteString("\n-- source: events channel last 24h\n\n")

	for killerID, victims := range counts {
		for killedID, count := range victims {
			sb.WriteString(fmt.Sprintf(
				"INSERT INTO kill_ledger (killer_id, killed_id, count) VALUES ('%s', '%s', %d) ON CONFLICT(killer_id, killed_id) DO UPDATE SET count = kill_ledger.count + excluded.count;\n",
				killerID, killedID, count,
			))
		}
	}

	outPath := "kill_ledger_migration.sql"
	if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
		log.Fatalf("failed to write migration file: %v", err)
	}
	log.Printf("wrote migration to %s", outPath)
}
