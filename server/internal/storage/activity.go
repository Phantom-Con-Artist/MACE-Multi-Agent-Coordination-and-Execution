package storage

import (
	"context"
	"encoding/json"
)

func (s *Store) Activity(ctx context.Context) ([]map[string]any, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,actor,action,target_id,details_json,timestamp FROM audit_log ORDER BY id DESC LIMIT 300`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int
		var actor, action, target, details, at string
		if e := rows.Scan(&id, &actor, &action, &target, &details, &at); e != nil {
			return nil, e
		}
		var d any
		json.Unmarshal([]byte(details), &d)
		out = append(out, map[string]any{"id": id, "actor": actor, "action": action, "target": target, "details": d, "at": at})
	}
	return out, rows.Err()
}
