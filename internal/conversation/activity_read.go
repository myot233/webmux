package conversation

import (
	"context"
	"database/sql"
	"unicode/utf8"
)

const MaxActivityBytes = 64 << 10

// readText presents the normalized heading and output as one byte-addressed
// item. Metadata-only patches never read or rewrite accumulated tool output.
func readText(ctx context.Context, tx *sql.Tx, id string, offset int64, maxBytes int) (string, int64, error) {
	if offset < 0 || maxBytes < 4 || maxBytes > MaxActivityBytes {
		return "", 0, ErrInvalid
	}
	var heading, output int64
	if err := tx.QueryRowContext(ctx, "SELECT heading_length,byte_length FROM activity WHERE id=?", id).Scan(&heading, &output); err != nil {
		return "", 0, storageError(ctx, err)
	}
	length := heading + output
	if offset > length {
		return "", 0, ErrInvalid
	}
	if offset == length {
		return "", offset, nil
	}
	end := min(length, offset+int64(maxBytes))
	body := make([]byte, 0, maxBytes)
	if offset < heading {
		data, err := readSection(ctx, tx, "activity_heading_chunk", id, offset, min(end, heading))
		if err != nil {
			return "", 0, err
		}
		body = append(body, data...)
	}
	if end > heading {
		data, err := readSection(ctx, tx, "activity_chunk", id, max(int64(0), offset-heading), end-heading)
		if err != nil {
			return "", 0, err
		}
		body = append(body, data...)
	}
	if int64(len(body)) != end-offset {
		return "", 0, ErrUnavailable
	}
	if len(body) > 0 && !utf8.RuneStart(body[0]) {
		return "", 0, ErrInvalid
	}
	for len(body) > 0 && !utf8.Valid(body) {
		body = body[:len(body)-1]
	}
	if len(body) == 0 {
		return "", 0, ErrUnavailable
	}
	return string(body), offset + int64(len(body)), nil
}

// Only internal constant table names enter this helper. The lower bound uses
// the maximum chunk size to seek the relevant index range instead of rescanning
// every chunk before a late byte offset.
func readSection(ctx context.Context, tx *sql.Tx, table, id string, start, end int64) ([]byte, error) {
	rows, err := tx.QueryContext(ctx, `SELECT substr(text,max(0,?-byte_offset)+1,min(byte_length,?-byte_offset)-max(0,?-byte_offset)) FROM `+table+` WHERE activity_id=? AND byte_offset>=? AND byte_offset<? AND byte_offset+byte_length>? ORDER BY byte_offset`, start, end, start, id, max(int64(0), start-MaxActivityBytes+1), end, start)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	body := make([]byte, 0, end-start)
	for rows.Next() {
		var chunk []byte
		if err := rows.Scan(&chunk); err != nil {
			return nil, storageError(ctx, err)
		}
		body = append(body, chunk...)
	}
	return body, storageError(ctx, rows.Err())
}
