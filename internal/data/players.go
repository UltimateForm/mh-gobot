package data

import (
	"context"
	"database/sql"
	"errors"
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

func ReadPlayer(ctx context.Context, playerID string) (*Player, error) {
	row := db.QueryRowContext(ctx, `SELECT player_id, username, raw_score, score, kills, deaths, assists FROM players WHERE player_id = ?`, playerID)
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
