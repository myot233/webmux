package conversation

import (
	"context"
	"database/sql"
)

const budgetSchema = `
ALTER TABLE activity ADD COLUMN json_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE activity ADD COLUMN heading_json_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE provider_thread ADD COLUMN usage_measured INTEGER NOT NULL DEFAULT 0;
PRAGMA user_version=3;
`

// backfillJSONCosts is a one-time, bounded-memory migration. A UTF-8 rune split
// between old chunks may be overestimated, which remains safe for budgeting.
func backfillJSONCosts(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM activity")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	for _, id := range ids {
		var costs [2]int64
		for section, table := range []string{"activity_chunk", "activity_heading_chunk"} {
			rows, err := tx.QueryContext(ctx, "SELECT text FROM "+table+" WHERE activity_id=? ORDER BY byte_offset", id)
			if err != nil {
				return err
			}
			for rows.Next() {
				var body []byte
				if err := rows.Scan(&body); err != nil {
					_ = rows.Close()
					return err
				}
				costs[section] += escapedTextBytes(string(body))
			}
			err = rows.Err()
			closeErr := rows.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE activity SET json_bytes=?,heading_json_bytes=? WHERE id=?", costs[0], costs[1], id); err != nil {
			return err
		}
	}
	return nil
}
