package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func InsertMatch(ctx context.Context, m Match, participants []MatchParticipant) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var mapVal any
	if m.Map != "" {
		mapVal = m.Map
	} else {
		mapVal = nil
	}

	res, err := tx.ExecContext(ctx, `INSERT INTO matches
		(game_mode, map, started_at, ended_at, team1_score, team2_score)
		VALUES (?, ?, ?, ?, ?, ?)`,
		m.GameMode, mapVal, m.StartedAt, m.EndedAt, m.Team1Score, m.Team2Score,
	)
	if err != nil {
		return 0, fmt.Errorf("insert match: %w", err)
	}
	matchID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO match_participants
		(match_id, player_id, team, rounds_won) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare participants: %w", err)
	}
	defer stmt.Close()

	for _, p := range participants {
		if _, err := stmt.ExecContext(ctx, matchID, p.PlayerID, p.Team, p.RoundsWon); err != nil {
			return 0, fmt.Errorf("insert participant %s: %w", p.PlayerID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, errors.Join(DbFailedToCommitDbTransaction, err)
	}
	return matchID, nil
}

func ReadMatchesForPlayer(ctx context.Context, playerID string, limit int) ([]Match, error) {
	rows, err := db.QueryContext(ctx, `SELECT m.id, m.game_mode, COALESCE(m.map, ''), m.started_at, m.ended_at, m.team1_score, m.team2_score
		FROM matches m
		INNER JOIN match_participants mp ON mp.match_id = m.id
		WHERE mp.player_id = ?
		ORDER BY m.ended_at DESC
		LIMIT ?`, playerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.ID, &m.GameMode, &m.Map, &m.StartedAt, &m.EndedAt, &m.Team1Score, &m.Team2Score); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

func CountRoundsWonForPlayer(ctx context.Context, playerID string) (int, error) {
	var total sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT SUM(rounds_won) FROM match_participants WHERE player_id = ?`, playerID).Scan(&total)
	if err != nil {
		return 0, err
	}
	if !total.Valid {
		return 0, nil
	}
	return int(total.Int64), nil
}

func CountMatchesForPlayer(ctx context.Context, playerID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT match_id) FROM match_participants WHERE player_id = ?`, playerID).Scan(&count)
	return count, err
}

func CountMatchesWonByPlayer(ctx context.Context, playerID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT mp.match_id)
		FROM match_participants mp
		INNER JOIN matches m ON m.id = mp.match_id
		WHERE mp.player_id = ?
		AND (
			(mp.team = 1 AND m.team1_score > m.team2_score) OR
			(mp.team = 2 AND m.team2_score > m.team1_score)
		)
	`, playerID).Scan(&count)
	return count, err
}
