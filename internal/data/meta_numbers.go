package data

import (
	"context"
	"database/sql"
	"errors"
)

func GetMetaNumber(ctx context.Context, key string) (float64, error) {
	var value float64
	err := db.QueryRowContext(ctx, `SELECT value FROM meta_numbers WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, DbMetaNumberNotFound
	}
	if err != nil {
		return 0, err
	}
	return value, nil
}

func SetMetaNumber(ctx context.Context, key string, value float64) error {
	_, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO meta_numbers (key, value) VALUES (?, ?)`, key, value)
	return err
}

func GetAllMetaNumbers(ctx context.Context) (map[string]float64, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM meta_numbers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]float64)
	for rows.Next() {
		var k string
		var v float64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
