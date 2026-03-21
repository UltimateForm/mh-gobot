package data

import (
	"context"
	"database/sql"
	"errors"
)

type VersusResult struct {
	AKills int // how many times A killed B
	BKills int // how many times B killed A
}

type LedgerEntry struct {
	PlayerID string
	Username string
	Count    int
}

func UpsertKillLedger(ctx context.Context, killerID, killedID string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO kill_ledger (killer_id, killed_id, count)
		VALUES (?, ?, 1)
		ON CONFLICT(killer_id, killed_id) DO UPDATE SET
			count = kill_ledger.count + 1`,
		killerID, killedID,
	)
	if err != nil {
		return errors.Join(DbKillLedgerError, err)
	}
	return nil
}

func ReadVersus(ctx context.Context, playerIDA, playerIDB string) (*VersusResult, error) {
	result := &VersusResult{}

	scanCount := func(killer, killed string, dest *int) error {
		err := db.QueryRowContext(ctx,
			`SELECT count FROM kill_ledger WHERE killer_id = ? AND killed_id = ?`,
			killer, killed,
		).Scan(dest)
		if errors.Is(err, sql.ErrNoRows) {
			*dest = 0
			return nil
		}
		return err
	}

	if err := scanCount(playerIDA, playerIDB, &result.AKills); err != nil {
		return nil, errors.Join(DbKillLedgerError, err)
	}
	if err := scanCount(playerIDB, playerIDA, &result.BKills); err != nil {
		return nil, errors.Join(DbKillLedgerError, err)
	}

	return result, nil
}

func ReadTopKillersOf(ctx context.Context, playerID string, limit int) ([]LedgerEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT k.killer_id, p.username, k.count
		FROM kill_ledger k
		JOIN players p ON p.player_id = k.killer_id
		WHERE k.killed_id = ?
		ORDER BY k.count DESC LIMIT ?`,
		playerID, limit,
	)
	if err != nil {
		return nil, errors.Join(DbKillLedgerError, err)
	}
	defer rows.Close()
	var entries []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.PlayerID, &e.Username, &e.Count); err != nil {
			return nil, errors.Join(DbKillLedgerError, err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func ReadTopVictimsOf(ctx context.Context, playerID string, limit int) ([]LedgerEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT k.killed_id, p.username, k.count
		FROM kill_ledger k
		JOIN players p ON p.player_id = k.killed_id
		WHERE k.killer_id = ?
		ORDER BY k.count DESC LIMIT ?`,
		playerID, limit,
	)
	if err != nil {
		return nil, errors.Join(DbKillLedgerError, err)
	}
	defer rows.Close()
	var entries []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.PlayerID, &e.Username, &e.Count); err != nil {
			return nil, errors.Join(DbKillLedgerError, err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}
