package storage

import (
	"context"
	"encoding/json"
)

// StoredMessage is a conversation message with its position in the host-wide
// message sequence. Sequence numbers only grow, so they order a conversation
// and serve as read cursors.
type StoredMessage struct {
	Seq  int64
	Data json.RawMessage
}

// AppendMessage stores a new message and returns its sequence number.
func (s *Store) AppendMessage(ctx context.Context, room, id string, value any) (int64, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO messages(id, room, data) VALUES(?,?,?)`, id, room, string(b))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateMessage replaces a stored message's data, keeping its position.
func (s *Store) UpdateMessage(ctx context.Context, id string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE messages SET data=? WHERE id=?`, string(b), id)
	return err
}

// MessageByID returns one message.
func (s *Store) MessageByID(ctx context.Context, id string) (StoredMessage, error) {
	var m StoredMessage
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT seq, data FROM messages WHERE id=?`, id).Scan(&m.Seq, &data)
	m.Data = json.RawMessage(data)
	return m, err
}

// Messages returns up to limit messages of a room in ascending order. With
// before > 0 it returns the newest messages older than before; with after > 0
// the oldest messages newer than after; otherwise the newest messages. more
// reports whether further messages exist in the requested direction.
func (s *Store) Messages(ctx context.Context, room string, before, after int64, limit int) (out []StoredMessage, more bool, err error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	query := `SELECT seq, data FROM messages WHERE room=? ORDER BY seq DESC LIMIT ?`
	args := []any{room, limit + 1}
	switch {
	case after > 0:
		query = `SELECT seq, data FROM messages WHERE room=? AND seq>? ORDER BY seq ASC LIMIT ?`
		args = []any{room, after, limit + 1}
	case before > 0:
		query = `SELECT seq, data FROM messages WHERE room=? AND seq<? ORDER BY seq DESC LIMIT ?`
		args = []any{room, before, limit + 1}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var m StoredMessage
		var data string
		if err := rows.Scan(&m.Seq, &data); err != nil {
			return nil, false, err
		}
		m.Data = json.RawMessage(data)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(out) > limit {
		out, more = out[:limit], true
	}
	if after <= 0 {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, more, nil
}
