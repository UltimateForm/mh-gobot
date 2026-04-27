package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func UpsertPlayer(ctx context.Context, player Player) error {
	if player.PlayerID == "" {
		return DbInvalidPlayer
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Join(DbPlayerUpsertError, err)
	}
	defer tx.Rollback()
	_, writeErr := tx.Exec(`INSERT INTO players (player_id, username, kills, deaths, assists, raw_score, score)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(player_id) DO UPDATE SET
    username  = excluded.username,
    kills     = players.kills + excluded.kills,
    deaths    = players.deaths + excluded.deaths,
    assists   = players.assists + excluded.assists,
    raw_score = players.raw_score + excluded.raw_score,
    score     = players.score + excluded.score
`, player.PlayerID, player.Username, player.Kills, player.Deaths, player.Assists, player.RawScore, player.Score)
	if writeErr != nil {
		return errors.Join(DbPlayerUpsertError, writeErr)
	}
	// rowsAffected, _ := write.RowsAffected()
	// logger.Printf("player id %v mutation on %v rows", player.PlayerID, rowsAffected)
	if err := tx.Commit(); err != nil {
		return errors.Join(DbPlayerUpsertError, DbFailedToCommitDbTransaction, err)
	}
	return nil
}

func scanPlayer(row *sql.Row) (*Player, error) {
	p := &Player{}
	err := row.Scan(&p.PlayerID, &p.Username, &p.RawScore, &p.Score, &p.Kills, &p.Deaths, &p.Assists, &p.RoundsWon, &p.MatchesWon)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, DbPlayerNotFound
	}
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	return p, nil
}

func ReadPlayer(ctx context.Context, playerID string) (*Player, error) {
	row := db.QueryRowContext(ctx, `SELECT player_id, username, raw_score, score, kills, deaths, assists, rounds_won, matches_won FROM players WHERE player_id = ?`, playerID)
	return scanPlayer(row)
}

func ReadPlayerByName(ctx context.Context, name string) (*Player, error) {
	row := db.QueryRowContext(ctx, `SELECT player_id, username, raw_score, score, kills, deaths, assists, rounds_won, matches_won FROM players WHERE username LIKE ? LIMIT 1`, "%"+name+"%")
	return scanPlayer(row)
}

// TopCategory maps Discord option values to DB column names.
// safe to interpolate into SQL — values are whitelisted here, never from user input directly.
var TopCategory = map[string]string{
	"score":   "score",
	"kills":   "kills",
	"deaths":  "deaths",
	"assists": "assists",
}

func ReadTopPlayers(ctx context.Context, limit int, column string) ([]RankedPlayer, error) {
	query := fmt.Sprintf(`SELECT player_id, username, raw_score, score, kills, deaths, assists, rounds_won, matches_won, ROW_NUMBER() OVER (ORDER BY %s DESC) as rank FROM players ORDER BY %s DESC LIMIT ?`, column, column)
	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	defer rows.Close()
	players := make([]RankedPlayer, 0, limit)
	for rows.Next() {
		var rp RankedPlayer
		if err := rows.Scan(&rp.PlayerID, &rp.Username, &rp.RawScore, &rp.Score, &rp.Kills, &rp.Deaths, &rp.Assists, &rp.RoundsWon, &rp.MatchesWon, &rp.Rank); err != nil {
			return nil, errors.Join(DbPlayerReadError, err)
		}
		players = append(players, rp)
	}
	return players, nil
}

func ReadPlayerRank(ctx context.Context, playerID string) (int, error) {
	var rank int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) + 1 FROM players WHERE score > (SELECT score FROM players WHERE player_id = ?)`, playerID).Scan(&rank)
	if err != nil {
		return 0, errors.Join(DbPlayerReadError, err)
	}
	return rank, nil
}

func ReadPlayerPlacement(ctx context.Context, playerID string) (*PlayerPlacement, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	defer tx.Rollback()

	var rank int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) + 1 FROM players WHERE score > (SELECT score FROM players WHERE player_id = ?)`, playerID).Scan(&rank)
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}

	offset := max(0, rank-5)
	rows, err := tx.QueryContext(ctx, `SELECT player_id, username, raw_score, score, kills, deaths, assists, rounds_won, matches_won, ROW_NUMBER() OVER (ORDER BY score DESC) as rank FROM players ORDER BY score DESC LIMIT 9 OFFSET ?`, offset)
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	defer rows.Close()

	snippet := make([]RankedPlayer, 0, 9)
	for rows.Next() {
		var rp RankedPlayer
		if err := rows.Scan(&rp.PlayerID, &rp.Username, &rp.RawScore, &rp.Score, &rp.Kills, &rp.Deaths, &rp.Assists, &rp.RoundsWon, &rp.MatchesWon, &rp.Rank); err != nil {
			return nil, errors.Join(DbPlayerReadError, err)
		}
		snippet = append(snippet, rp)
	}

	return &PlayerPlacement{Rank: rank, Snippet: snippet}, nil
}

func UpsertSkirmishWin(ctx context.Context, playerID string, scoreBonus int, roundsWon int, matchesWon int) error {
	res, err := db.ExecContext(ctx, `
UPDATE players SET
    score       = score + ?,
    rounds_won  = rounds_won + ?,
    matches_won = matches_won + ?
WHERE player_id = ?
`, scoreBonus, roundsWon, matchesWon, playerID)
	if err != nil {
		return errors.Join(DbPlayerUpsertError, err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		logger.Printf("UpsertSkirmishWin: player %s not found, skipping", playerID)
	}
	return nil
}

func AddPlayerScore(ctx context.Context, playerID string, delta int) error {
	res, err := db.ExecContext(ctx, `UPDATE players SET score = score + ? WHERE player_id = ?`, delta, playerID)
	if err != nil {
		return errors.Join(DbPlayerUpsertError, err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		logger.Printf("AddPlayerScore: player %s not found, skipping", playerID)
	}
	return nil
}

func ReadPlayerScores(ctx context.Context, playerIDs []string) (map[string]int, error) {
	if len(playerIDs) == 0 {
		return make(map[string]int), nil
	}
	placeholders := make([]string, len(playerIDs))
	args := make([]interface{}, len(playerIDs))
	for i, id := range playerIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`SELECT player_id, score FROM players WHERE player_id IN (%s)`,
		strings.Join(placeholders, ","))
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	defer rows.Close()
	result := make(map[string]int, len(playerIDs))
	for rows.Next() {
		var id string
		var score int
		if err := rows.Scan(&id, &score); err != nil {
			return nil, errors.Join(DbPlayerReadError, err)
		}
		result[id] = score
	}
	return result, nil
}

func ReadAggregates(ctx context.Context) (*PlayerAggregates, error) {
	var agg PlayerAggregates
	err := db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(kills), 0), COALESCE(SUM(deaths), 0), COALESCE(AVG(score), 0) FROM players`).
		Scan(&agg.TotalPlayers, &agg.TotalKills, &agg.TotalDeaths, &agg.AvgScore)
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	return &agg, nil
}
