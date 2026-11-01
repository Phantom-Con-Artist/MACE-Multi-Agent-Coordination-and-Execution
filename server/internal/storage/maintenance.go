package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupDir holds VACUUM INTO snapshots next to the database.
func (s *Store) BackupDir() string { return filepath.Join(filepath.Dir(s.path), "backups") }

// Backup writes a consistent snapshot of the database and returns its path.
func (s *Store) Backup(label string) (string, error) {
	if s.path == ":memory:" {
		return "", nil
	}
	dir := s.BackupDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if s.global != s.db {
		// With sessions, each snapshot names the session it is of.
		label += "-" + strings.TrimSuffix(filepath.Base(s.sessionPath), ".db")
	}
	path := filepath.Join(dir, fmt.Sprintf("mace-%s-%s.db", time.Now().UTC().Format("20060102T150405"), label))
	if _, err := s.db.Exec(`VACUUM INTO ?`, path); err != nil {
		return "", err
	}
	return path, os.Chmod(path, 0600)
}

// LatestBackup returns the newest snapshot and when it was taken.
func (s *Store) LatestBackup() (string, time.Time) {
	files := backups(s.BackupDir())
	if len(files) == 0 {
		return "", time.Time{}
	}
	info, err := os.Stat(files[0])
	if err != nil {
		return "", time.Time{}
	}
	return files[0], info.ModTime()
}

// PruneBackups keeps the newest keep snapshots.
func (s *Store) PruneBackups(keep int) {
	for i, path := range backups(s.BackupDir()) {
		if i >= keep {
			os.Remove(path)
		}
	}
}

// PruneEvents deletes native output of finished processes older than cutoff.
// Messages, audit records, and live processes are never pruned.
func (s *Store) PruneEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agent_events WHERE timestamp < ? AND agent_id IN (SELECT id FROM agents WHERE status IN ('stopped','failed','killed'))`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	_, err = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return n, err
}

func (s *Store) healthy() (bool, string) {
	var result string
	if err := s.db.QueryRow(`PRAGMA quick_check`).Scan(&result); err != nil {
		if strings.Contains(err.Error(), "not a database") || strings.Contains(err.Error(), "malformed") {
			return false, err.Error()
		}
		// Other errors, such as a locked file, are not evidence of corruption.
		return true, ""
	}
	return result == "ok", result
}

// recoverCorrupt moves a damaged database aside and restores the newest backup.
// It never deletes the damaged file, so data can still be salvaged by hand.
func recoverCorrupt(path, detail string) (string, error) {
	stamp := time.Now().UTC().Format("20060102T150405")
	aside := path + ".corrupt-" + stamp
	if err := os.Rename(path, aside); err != nil {
		return "", fmt.Errorf("database is damaged (%s) and could not be moved aside: %w", detail, err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		os.Rename(path+suffix, aside+suffix)
	}
	files := backups(filepath.Join(filepath.Dir(path), "backups"))
	if len(files) == 0 {
		return fmt.Sprintf("Database was damaged (%s). It was moved to %s and no backup existed, so Mace started with empty history.", detail, aside), nil
	}
	if err := copyFile(files[0], path); err != nil {
		return "", fmt.Errorf("restore from %s failed: %w", files[0], err)
	}
	info, _ := os.Stat(files[0])
	return fmt.Sprintf("Database was damaged (%s). It was moved to %s and the backup from %s was restored. Changes after that time are not in the restored history.", detail, aside, info.ModTime().UTC().Format(time.RFC3339)), nil
}

func backups(dir string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "mace-*.db"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(to)
		return err
	}
	return out.Close()
}
