package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// A MACE installation runs one session at a time. Each session has its own
// database: agents, groups, messages, history, statuses, and jobs, so nothing
// leaks between sessions. What every session shares lives in global.db: this
// machine's links to other machines, signed-in phones and browsers, push
// subscriptions, settings, the session list, and the audit log.

// globalKinds are the documents every session shares.
var globalKinds = map[string]bool{
	"node": true, "node-removed": true, "peer": true, "device": true, "push": true,
	"setting": true, "signin-code": true, "join-invite": true, "workspace": true,
	"machine-facts": true,
}

// docs is the database that holds a kind of document.
func (s *Store) docs(kind string) *sql.DB {
	if globalKinds[kind] {
		return s.global
	}
	return s.db
}

// Workspace is one session: its name and its database file, relative to the
// data folder.
type Workspace struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	File    string    `json:"file"`
	Created time.Time `json:"created"`
	// Active is when this session last became the active one.
	Active time.Time `json:"active"`
}

// ActiveWorkspace names the session the installation runs.
type ActiveWorkspace struct {
	ID string    `json:"id"`
	At time.Time `json:"at"`
}

const activeKey = "active-workspace"

// SessionPath is the database file of the open session.
func (s *Store) SessionPath() string { return s.sessionPath }

// OpenWorkspaces opens the shared database and the active session's database
// in dataDir. The first time, the existing database file (defaultFile, usually
// mace.db) becomes the "Default" session, and what every session shares is
// copied out of it into global.db; nothing is moved or deleted.
func OpenWorkspaces(dataDir, defaultFile string) (*Store, Workspace, error) {
	globalPath := filepath.Join(dataDir, "global.db")
	_, statErr := os.Stat(globalPath)
	fresh := errors.Is(statErr, os.ErrNotExist)
	global, err := Open(globalPath)
	if err != nil {
		return nil, Workspace{}, err
	}
	ctx := context.Background()
	if fresh {
		if err := global.adoptDefault(ctx, dataDir, defaultFile); err != nil {
			global.Close()
			return nil, Workspace{}, err
		}
	}
	w, err := global.ActiveWorkspace(ctx)
	if err != nil {
		global.Close()
		return nil, Workspace{}, err
	}
	session, err := Open(filepath.Join(dataDir, w.File))
	if err != nil {
		global.Close()
		return nil, Workspace{}, err
	}
	store := &Store{db: session.db, global: global.db, path: globalPath, sessionPath: session.path, Recovered: session.Recovered}
	return store, w, nil
}

// adoptDefault makes the existing database the "Default" session and copies
// the shared documents out of it.
func (g *Store) adoptDefault(ctx context.Context, dataDir, file string) error {
	path := filepath.Join(dataDir, file)
	if _, err := os.Stat(path); err == nil {
		old, err := Open(path)
		if err != nil {
			return err
		}
		defer old.Close()
		for kind := range globalKinds {
			rows, err := old.db.QueryContext(ctx, `SELECT id, data FROM documents WHERE kind=?`, kind)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id, data string
				if rows.Scan(&id, &data) == nil {
					g.db.ExecContext(ctx, `INSERT OR IGNORE INTO documents(kind,id,data) VALUES(?,?,?)`, kind, id, data)
				}
			}
			rows.Close()
		}
	}
	now := time.Now().UTC()
	w := Workspace{ID: "default", Name: "Default", File: file, Created: now, Active: now}
	if err := g.PutDocument(ctx, "workspace", w.ID, w); err != nil {
		return err
	}
	return g.PutDocument(ctx, "setting", activeKey, ActiveWorkspace{ID: w.ID, At: now})
}

// Workspaces lists the sessions, oldest first.
func (s *Store) Workspaces(ctx context.Context) ([]Workspace, error) {
	raw, err := s.Documents(ctx, "workspace")
	if err != nil {
		return nil, err
	}
	out := []Workspace{}
	for _, b := range raw {
		var w Workspace
		if json.Unmarshal(b, &w) == nil && w.ID != "" {
			out = append(out, w)
		}
	}
	return out, nil
}

// ActiveWorkspace returns the session the installation runs.
func (s *Store) ActiveWorkspace(ctx context.Context) (Workspace, error) {
	var a ActiveWorkspace
	if err := s.Document(ctx, "setting", activeKey, &a); err != nil {
		return Workspace{}, fmt.Errorf("no active session: %w", err)
	}
	var w Workspace
	if err := s.Document(ctx, "workspace", a.ID, &w); err != nil {
		return Workspace{}, fmt.Errorf("the active session %s is missing: %w", a.ID, err)
	}
	return w, nil
}

// ActiveSince is when the active session was chosen, to settle which of two
// machines switched last.
func (s *Store) ActiveSince(ctx context.Context) ActiveWorkspace {
	var a ActiveWorkspace
	s.Document(ctx, "setting", activeKey, &a)
	return a
}

// AddWorkspace records a session with its own new database file.
func (s *Store) AddWorkspace(ctx context.Context, id, name string) (Workspace, error) {
	w := Workspace{ID: id, Name: name, File: filepath.Join("sessions", id+".db"), Created: time.Now().UTC()}
	return w, s.PutDocument(ctx, "workspace", id, w)
}

// SetActiveWorkspace chooses the session the installation runs after its
// next start.
func (s *Store) SetActiveWorkspace(ctx context.Context, id string, at time.Time) error {
	var w Workspace
	if err := s.Document(ctx, "workspace", id, &w); err != nil {
		return fmt.Errorf("there is no session %s", id)
	}
	w.Active = at
	if err := s.PutDocument(ctx, "workspace", id, w); err != nil {
		return err
	}
	return s.PutDocument(ctx, "setting", activeKey, ActiveWorkspace{ID: id, At: at})
}

// RemoveWorkspace deletes a session that is not active, with its database
// files. It returns the IDs of the session's groups, so their shared files
// can be removed too.
func (s *Store) RemoveWorkspace(ctx context.Context, dataDir, id string) ([]string, error) {
	if s.ActiveSince(ctx).ID == id {
		return nil, errors.New("the active session cannot be removed; switch to another one first")
	}
	var w Workspace
	if err := s.Document(ctx, "workspace", id, &w); err != nil {
		return nil, fmt.Errorf("there is no session %s", id)
	}
	path := filepath.Join(dataDir, w.File)
	rooms := []string{}
	if old, err := Open(path); err == nil {
		rows, err := old.db.QueryContext(ctx, `SELECT id FROM documents WHERE kind='room'`)
		if err == nil {
			for rows.Next() {
				var r string
				if rows.Scan(&r) == nil {
					rooms = append(rooms, r)
				}
			}
			rows.Close()
		}
		old.Close()
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(path + suffix)
	}
	_, err := s.DeleteDocument(ctx, "workspace", id)
	return rooms, err
}
