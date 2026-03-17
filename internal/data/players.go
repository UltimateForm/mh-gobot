package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	write, writeErr := tx.Exec(`INSERT INTO players (player_id, username, kills, deaths, assists, raw_score, score)
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
	rowsAffected, _ := write.RowsAffected()
	logger.Printf("player id %v mutation on %v rows", player.PlayerID, rowsAffected)
	if err := tx.Commit(); err != nil {
		return errors.Join(DbPlayerUpsertError, DbFailedToCommitDbTransaction, err)
	}
	return nil
}

func scanPlayer(row *sql.Row) (*Player, error) {
	p := &Player{}
	err := row.Scan(&p.PlayerID, &p.Username, &p.RawScore, &p.Score, &p.Kills, &p.Deaths, &p.Assists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, DbPlayerNotFound
	}
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	return p, nil
}

func ReadPlayer(ctx context.Context, playerID string) (*Player, error) {
	row := db.QueryRowContext(ctx, `SELECT player_id, username, raw_score, score, kills, deaths, assists FROM players WHERE player_id = ?`, playerID)
	return scanPlayer(row)
}

func ReadPlayerByName(ctx context.Context, name string) (*Player, error) {
	row := db.QueryRowContext(ctx, `SELECT player_id, username, raw_score, score, kills, deaths, assists FROM players WHERE username LIKE ? LIMIT 1`, "%"+name+"%")
	return scanPlayer(row)
}

// TopCategory maps Discord option values to DB column names.
// safe to interpolate into SQL — values are whitelisted here, never from user input directly.
var TopCategory = map[string]string{
	"score":   "raw_score",
	"kills":   "kills",
	"deaths":  "deaths",
	"assists": "assists",
}

func ReadTopPlayers(ctx context.Context, limit int, column string) ([]RankedPlayer, error) {
	query := fmt.Sprintf(`SELECT player_id, username, raw_score, score, kills, deaths, assists, ROW_NUMBER() OVER (ORDER BY %s DESC) as rank FROM players ORDER BY %s DESC LIMIT ?`, column, column)
	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	defer rows.Close()
	players := make([]RankedPlayer, 0, limit)
	for rows.Next() {
		var rp RankedPlayer
		if err := rows.Scan(&rp.PlayerID, &rp.Username, &rp.RawScore, &rp.Score, &rp.Kills, &rp.Deaths, &rp.Assists, &rp.Rank); err != nil {
			return nil, errors.Join(DbPlayerReadError, err)
		}
		players = append(players, rp)
	}
	return players, nil
}

func ReadPlayerPlacement(ctx context.Context, playerID string) (*PlayerPlacement, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	defer tx.Rollback()

	var rank int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) + 1 FROM players WHERE raw_score > (SELECT raw_score FROM players WHERE player_id = ?)`, playerID).Scan(&rank)
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}

	offset := max(0, rank-5)
	rows, err := tx.QueryContext(ctx, `SELECT player_id, username, raw_score, score, kills, deaths, assists, ROW_NUMBER() OVER (ORDER BY raw_score DESC) as rank FROM players ORDER BY raw_score DESC LIMIT 9 OFFSET ?`, offset)
	if err != nil {
		return nil, errors.Join(DbPlayerReadError, err)
	}
	defer rows.Close()

	snippet := make([]RankedPlayer, 0, 9)
	for rows.Next() {
		var rp RankedPlayer
		if err := rows.Scan(&rp.PlayerID, &rp.Username, &rp.RawScore, &rp.Score, &rp.Kills, &rp.Deaths, &rp.Assists, &rp.Rank); err != nil {
			return nil, errors.Join(DbPlayerReadError, err)
		}
		snippet = append(snippet, rp)
	}

	return &PlayerPlacement{Rank: rank, Snippet: snippet}, nil
}
