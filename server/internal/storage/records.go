package storage

import (
	"context"
	"database/sql"
	"errors"
)

// RecordRow is a stored record with the fields needed to find and order it.
// Data holds the full record as JSON.
type RecordRow struct {
	ID           string
	Node         string
	Conversation string
	Wall         int64
	Count        uint32
	Prev         string
	Kind         string
	Ref          string
	Cached       bool
	Data         []byte
}

// ErrHeadMoved means another write extended the chain first.
var ErrHeadMoved = errors.New("the record chain moved; retry")

// AppendRecord stores a record that extends its node's chain in its
// conversation, provided the chain still ends at expectPrev.
func (s *Store) AppendRecord(ctx context.Context, r RecordRow, expectPrev string) error {
	return s.AppendRecords(ctx, []RecordRow{r}, expectPrev)
}

// AppendRecords stores consecutive records of one node's chain in one
// conversation, oldest first, provided the chain still ends at expectPrev and
// each record links to the one before it.
func (s *Store) AppendRecords(ctx context.Context, rows []RecordRow, expectPrev string) error {
	if len(rows) == 0 {
		return nil
	}
	node, conversation := rows[0].Node, rows[0].Conversation
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var head string
	err = tx.QueryRowContext(ctx, `SELECT head FROM heads WHERE node=? AND conversation=?`, node, conversation).Scan(&head)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if head != expectPrev {
		return ErrHeadMoved
	}
	prev := expectPrev
	for _, r := range rows {
		if r.Node != node || r.Conversation != conversation || r.Prev != prev {
			return ErrHeadMoved
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO records(id, node, conversation, wall, count, prev, kind, ref, cached, data) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			r.ID, r.Node, r.Conversation, r.Wall, r.Count, r.Prev, r.Kind, r.Ref, r.Cached, string(r.Data)); err != nil {
			return err
		}
		prev = r.ID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO heads(node, conversation, head) VALUES(?,?,?) ON CONFLICT(node, conversation) DO UPDATE SET head=excluded.head`,
		node, conversation, prev); err != nil {
		return err
	}
	return tx.Commit()
}

// ChainHead is the newest record of one node's chain in one conversation.
type ChainHead struct {
	Node         string `json:"node"`
	Conversation string `json:"conversation"`
	Head         string `json:"head"`
}

// AllHeads lists every chain this store holds, its own and cached ones.
func (s *Store) AllHeads(ctx context.Context) ([]ChainHead, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT node, conversation, head FROM heads ORDER BY conversation, node`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChainHead{}
	for rows.Next() {
		var h ChainHead
		if err := rows.Scan(&h.Node, &h.Conversation, &h.Head); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// RecordByID returns one stored record.
func (s *Store) RecordByID(ctx context.Context, id string) (RecordRow, error) {
	var r RecordRow
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT id, node, conversation, wall, count, prev, kind, ref, cached, data FROM records WHERE id=?`, id).
		Scan(&r.ID, &r.Node, &r.Conversation, &r.Wall, &r.Count, &r.Prev, &r.Kind, &r.Ref, &r.Cached, &data)
	r.Data = []byte(data)
	return r, err
}

// RecordHead returns the newest record of a node's chain in a conversation,
// or "" when the node has written nothing there.
func (s *Store) RecordHead(ctx context.Context, node, conversation string) (string, error) {
	var head string
	err := s.db.QueryRowContext(ctx, `SELECT head FROM heads WHERE node=? AND conversation=?`, node, conversation).Scan(&head)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return head, err
}

// ConversationHeads returns every node's head in a conversation.
func (s *Store) ConversationHeads(ctx context.Context, conversation string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT node, head FROM heads WHERE conversation=?`, conversation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var node, head string
		if err := rows.Scan(&node, &head); err != nil {
			return nil, err
		}
		out[node] = head
	}
	return out, rows.Err()
}

// RecordIDByRef returns the ID of the record a node wrote for ref, or "".
func (s *Store) RecordIDByRef(ctx context.Context, node, ref string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM records WHERE node=? AND ref=?`, node, ref).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// LatestRecordClock returns the highest clock a node has written, so its
// clock never runs backwards across restarts.
func (s *Store) LatestRecordClock(ctx context.Context, node string) (int64, uint32, error) {
	var wall int64
	var count uint32
	err := s.db.QueryRowContext(ctx, `SELECT wall, count FROM records WHERE node=? ORDER BY wall DESC, count DESC LIMIT 1`, node).Scan(&wall, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return wall, count, err
}

// ConversationRecords returns every stored record in a conversation, in no
// particular order; records.Order sorts them.
func (s *Store) ConversationRecords(ctx context.Context, conversation string) ([]RecordRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, node, conversation, wall, count, prev, kind, ref, cached, data FROM records WHERE conversation=?`, conversation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecordRow{}
	for rows.Next() {
		var r RecordRow
		var data string
		if err := rows.Scan(&r.ID, &r.Node, &r.Conversation, &r.Wall, &r.Count, &r.Prev, &r.Kind, &r.Ref, &r.Cached, &data); err != nil {
			return nil, err
		}
		r.Data = []byte(data)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Path reports where the store lives, ":memory:" for an in-memory store.
func (s *Store) Path() string { return s.path }

// DB exposes the database for maintenance tools and tests that need to
// inspect or damage stored data directly.
func (s *Store) DB() *sql.DB { return s.db }
