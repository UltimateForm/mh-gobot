package data

import "context"

func ReadRankTiers(ctx context.Context) ([]RankTier, error) {
	rows, err := db.QueryContext(ctx, `SELECT score_gate, name, COALESCE(short_name, '') FROM rank_tiers ORDER BY score_gate ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tiers []RankTier
	for rows.Next() {
		var t RankTier
		if err := rows.Scan(&t.ScoreGate, &t.Name, &t.ShortName); err != nil {
			return nil, err
		}
		tiers = append(tiers, t)
	}
	return tiers, rows.Err()
}

func UpsertRankTier(ctx context.Context, tier RankTier) error {
	var shortName any
	if tier.ShortName != "" {
		shortName = tier.ShortName
	}
	_, err := db.ExecContext(ctx, `INSERT INTO rank_tiers (score_gate, name, short_name)
		VALUES (?, ?, ?)
		ON CONFLICT(score_gate) DO UPDATE SET name = excluded.name, short_name = excluded.short_name`,
		tier.ScoreGate, tier.Name, shortName)
	return err
}

func DeleteRankTier(ctx context.Context, scoreGate int) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM rank_tiers WHERE score_gate = ?`, scoreGate)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

