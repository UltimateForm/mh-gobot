package data

import (
	"context"
	"database/sql"
	"errors"
)

func GetMeta(ctx context.Context, key string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", DbMetaNotFound
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func SetMeta(ctx context.Context, key, value string) error {
	_, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, key, value)
	return err
}
