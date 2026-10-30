package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mace/pkg/protocol"
	_ "modernc.org/sqlite"
)

// Store provides persistence for MACE agents, execution events, and audit logs.
type Store struct {
	db *sql.DB
}

// Open initializes an SQLite database with WAL mode and foreign key constraints enabled.
func Open(dbPath string) (*Store, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create storage directory: %w", err)
		}
	}

	// SQLite connection string with WAL and busy timeout
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite performs best with a single write connection
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	return store, nil
}

// Close closes the underlying SQLite database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		pid INTEGER NOT NULL,
		status TEXT NOT NULL,
		command TEXT NOT NULL,
		args_json TEXT,
		working_dir TEXT NOT NULL,
		exit_code INTEGER,
		error TEXT,
		started_at DATETIME NOT NULL,
		stopped_at DATETIME,
		bytes_read INTEGER DEFAULT 0,
		bytes_written INTEGER DEFAULT 0,
		labels_json TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
	CREATE INDEX IF NOT EXISTS idx_agents_started_at ON agents(started_at);

	CREATE TABLE IF NOT EXISTS agent_events (
		event_id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		type TEXT NOT NULL,
		data TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		sequence INTEGER NOT NULL,
		metadata_json TEXT,
		FOREIGN KEY(agent_id) REFERENCES agents(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_agent_events_agent_seq ON agent_events(agent_id, sequence);

	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		target_id TEXT,
		details_json TEXT,
		timestamp DATETIME NOT NULL
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// SaveAgent inserts or replaces an agent record.
func (s *Store) SaveAgent(ctx context.Context, a *protocol.AgentProcess) error {
	argsJSON, _ := json.Marshal(a.Args)
	labelsJSON, _ := json.Marshal(a.Labels)

	query := `
	INSERT INTO agents (
		id, name, pid, status, command, args_json, working_dir,
		exit_code, error, started_at, stopped_at, bytes_read, bytes_written, labels_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		status = excluded.status,
		exit_code = excluded.exit_code,
		error = excluded.error,
		stopped_at = excluded.stopped_at,
		bytes_read = excluded.bytes_read,
		bytes_written = excluded.bytes_written;
	`
	_, err := s.db.ExecContext(ctx, query,
		a.ID,
		a.Name,
		a.PID,
		string(a.Status),
		a.Command,
		string(argsJSON),
		a.WorkingDir,
		a.ExitCode,
		a.Error,
		a.StartedAt,
		a.StoppedAt,
		a.BytesRead,
		a.BytesWritten,
		string(labelsJSON),
	)
	return err
}

// UpdateAgentStatus updates the agent's execution status, exit code, error, and terminal timestamp.
func (s *Store) UpdateAgentStatus(ctx context.Context, id string, status protocol.AgentStatus, exitCode *int, errMsg string, stoppedAt *time.Time) error {
	query := `
	UPDATE agents SET
		status = ?,
		exit_code = ?,
		error = ?,
		stopped_at = ?
	WHERE id = ?;
	`
	_, err := s.db.ExecContext(ctx, query, string(status), exitCode, errMsg, stoppedAt, id)
	return err
}

// GetAgent retrieves an agent process by its ID.
func (s *Store) GetAgent(ctx context.Context, id string) (*protocol.AgentProcess, error) {
	query := `
	SELECT id, name, pid, status, command, args_json, working_dir,
	       exit_code, error, started_at, stopped_at, bytes_read, bytes_written, labels_json
	FROM agents WHERE id = ?;
	`
	row := s.db.QueryRowContext(ctx, query, id)

	var a protocol.AgentProcess
	var status string
	var argsJSON, labelsJSON sql.NullString

	err := row.Scan(
		&a.ID,
		&a.Name,
		&a.PID,
		&status,
		&a.Command,
		&argsJSON,
		&a.WorkingDir,
		&a.ExitCode,
		&a.Error,
		&a.StartedAt,
		&a.StoppedAt,
		&a.BytesRead,
		&a.BytesWritten,
		&labelsJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	a.Status = protocol.AgentStatus(status)
	if argsJSON.Valid && argsJSON.String != "" {
		_ = json.Unmarshal([]byte(argsJSON.String), &a.Args)
	}
	if labelsJSON.Valid && labelsJSON.String != "" {
		_ = json.Unmarshal([]byte(labelsJSON.String), &a.Labels)
	}

	return &a, nil
}

// ListAgents returns all registered agents ordered by start time descending.
func (s *Store) ListAgents(ctx context.Context) ([]*protocol.AgentProcess, error) {
	query := `
	SELECT id, name, pid, status, command, args_json, working_dir,
	       exit_code, error, started_at, stopped_at, bytes_read, bytes_written, labels_json
	FROM agents ORDER BY started_at DESC;
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*protocol.AgentProcess
	for rows.Next() {
		var a protocol.AgentProcess
		var status string
		var argsJSON, labelsJSON sql.NullString

		err := rows.Scan(
			&a.ID,
			&a.Name,
			&a.PID,
			&status,
			&a.Command,
			&argsJSON,
			&a.WorkingDir,
			&a.ExitCode,
			&a.Error,
			&a.StartedAt,
			&a.StoppedAt,
			&a.BytesRead,
			&a.BytesWritten,
			&labelsJSON,
		)
		if err != nil {
			return nil, err
		}

		a.Status = protocol.AgentStatus(status)
		if argsJSON.Valid && argsJSON.String != "" {
			_ = json.Unmarshal([]byte(argsJSON.String), &a.Args)
		}
		if labelsJSON.Valid && labelsJSON.String != "" {
			_ = json.Unmarshal([]byte(labelsJSON.String), &a.Labels)
		}
		results = append(results, &a)
	}

	return results, rows.Err()
}

// AppendEvent stores an agent streaming output or status event.
func (s *Store) AppendEvent(ctx context.Context, e *protocol.AgentEvent) error {
	metaJSON, _ := json.Marshal(e.Metadata)

	query := `
	INSERT INTO agent_events (event_id, agent_id, type, data, timestamp, sequence, metadata_json)
	VALUES (?, ?, ?, ?, ?, ?, ?);
	`
	_, err := s.db.ExecContext(ctx, query,
		e.EventID,
		e.AgentID,
		string(e.Type),
		e.Data,
		e.Timestamp,
		e.Sequence,
		string(metaJSON),
	)
	return err
}

// GetAgentEvents retrieves historical events for an agent after a given sequence.
func (s *Store) GetAgentEvents(ctx context.Context, agentID string, afterSeq uint64, limit int) ([]*protocol.AgentEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
	SELECT event_id, agent_id, type, data, timestamp, sequence, metadata_json
	FROM agent_events
	WHERE agent_id = ? AND sequence > ?
	ORDER BY sequence ASC LIMIT ?;
	`
	rows, err := s.db.QueryContext(ctx, query, agentID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*protocol.AgentEvent
	for rows.Next() {
		var e protocol.AgentEvent
		var eventType string
		var metaJSON sql.NullString

		if err := rows.Scan(&e.EventID, &e.AgentID, &eventType, &e.Data, &e.Timestamp, &e.Sequence, &metaJSON); err != nil {
			return nil, err
		}

		e.Type = protocol.EventType(eventType)
		if metaJSON.Valid && metaJSON.String != "" {
			_ = json.Unmarshal([]byte(metaJSON.String), &e.Metadata)
		}
		events = append(events, &e)
	}

	return events, rows.Err()
}

// AppendAudit records an immutable audit log entry.
func (s *Store) AppendAudit(ctx context.Context, actor, action, targetID string, details map[string]interface{}) error {
	detailsJSON, _ := json.Marshal(details)

	query := `
	INSERT INTO audit_log (actor, action, target_id, details_json, timestamp)
	VALUES (?, ?, ?, ?, ?);
	`
	_, err := s.db.ExecContext(ctx, query, actor, action, targetID, string(detailsJSON), time.Now().UTC())
	return err
}
