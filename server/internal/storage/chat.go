package storage

import (
	"context"
	"encoding/json"
)

// Documents stores versioned chat records alongside the process audit database.
func (s *Store) PutDocument(ctx context.Context, kind, id string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.docs(kind).ExecContext(ctx, `INSERT INTO documents(kind,id,data) VALUES(?,?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data`, kind, id, string(b))
	return err
}
func (s *Store) Documents(ctx context.Context, kind string) ([]json.RawMessage, error) {
	rows, err := s.docs(kind).QueryContext(ctx, `SELECT data FROM documents WHERE kind=? ORDER BY rowid`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []json.RawMessage{}
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		result = append(result, json.RawMessage(b))
	}
	return result, rows.Err()
}

func (s *Store) Document(ctx context.Context, kind, id string, value any) error {
	var data string
	if err := s.docs(kind).QueryRowContext(ctx, `SELECT data FROM documents WHERE kind=? AND id=?`, kind, id).Scan(&data); err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), value)
}

// DeleteDocument removes a document and reports whether it existed, so a
// caller can consume a one-time document exactly once.
func (s *Store) DeleteDocument(ctx context.Context, kind, id string) (bool, error) {
	res, err := s.docs(kind).ExecContext(ctx, `DELETE FROM documents WHERE kind=? AND id=?`, kind, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
