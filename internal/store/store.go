// Package store owns the sqlite connection, schema migration, and CRUD for
// itworks.dev entries. No IP addresses are ever stored.
package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Status values for entries.status.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusHidden   = "hidden"
)

const idAlphabet = "abcdefghijklmnopqrstuvwxyz234567"
const idLength = 12

// Entry mirrors the entries table.
type Entry struct {
	ID           string
	Name         string
	Summary      string
	Source       string
	RepoURL      string
	AuditTier    string
	AuditDate    string
	Found        int
	Fixed        int
	Accepted     int
	CriticalOpen int
	Status       string
	CreatedAt    string
	ApprovedAt   sql.NullString
}

// NewEntry carries the fields needed to insert a new pending entry.
type NewEntry struct {
	Name         string
	Summary      string
	Source       string
	RepoURL      string
	AuditTier    string
	AuditDate    string
	Found        int
	Fixed        int
	Accepted     int
	CriticalOpen int
}

// ErrNotFound is returned when an entry id does not exist.
var ErrNotFound = errors.New("entry not found")

// DB wraps the sqlite connection pool.
type DB struct {
	sql *sql.DB
}

// Open opens (creating if needed) the sqlite file at path, sets WAL mode,
// a 5 second busy timeout, a single open connection (sqlite is single
// writer), and runs the schema migration.
func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	db := &DB{sql: sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Close closes the underlying connection.
func (db *DB) Close() error {
	return db.sql.Close()
}

func (db *DB) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS entries (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	summary TEXT NOT NULL,
	source TEXT NOT NULL,
	repo_url TEXT NOT NULL,
	audit_tier TEXT NOT NULL,
	audit_date TEXT NOT NULL,
	found INTEGER NOT NULL,
	fixed INTEGER NOT NULL,
	accepted INTEGER NOT NULL,
	critical_open INTEGER NOT NULL,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	approved_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_entries_status ON entries(status);
`
	_, err := db.sql.Exec(schema)
	return err
}

func generateID() (string, error) {
	b := make([]byte, idLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, idLength)
	for i, v := range b {
		out[i] = idAlphabet[int(v)%len(idAlphabet)]
	}
	return string(out), nil
}

// Create inserts a new pending entry and returns its generated id.
func (db *DB) Create(e NewEntry) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	for attempt := 0; attempt < 5; attempt++ {
		id, err := generateID()
		if err != nil {
			return "", err
		}
		_, err = db.sql.Exec(
			`INSERT INTO entries (id, name, summary, source, repo_url, audit_tier, audit_date, found, fixed, accepted, critical_open, status, created_at, approved_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
			id, e.Name, e.Summary, e.Source, e.RepoURL, e.AuditTier, e.AuditDate, e.Found, e.Fixed, e.Accepted, e.CriticalOpen, StatusPending, now,
		)
		if err == nil {
			return id, nil
		}
		// Extremely unlikely id collision: retry with a fresh id.
		if isUniqueViolation(err) {
			continue
		}
		return "", err
	}
	return "", errors.New("could not generate a unique id")
}

func isUniqueViolation(err error) bool {
	// modernc.org/sqlite wraps the sqlite error; matching by substring
	// avoids importing its internal error code type.
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}

func scanEntry(row interface {
	Scan(dest ...any) error
}) (Entry, error) {
	var e Entry
	err := row.Scan(&e.ID, &e.Name, &e.Summary, &e.Source, &e.RepoURL, &e.AuditTier, &e.AuditDate,
		&e.Found, &e.Fixed, &e.Accepted, &e.CriticalOpen, &e.Status, &e.CreatedAt, &e.ApprovedAt)
	return e, err
}

const selectCols = `id, name, summary, source, repo_url, audit_tier, audit_date, found, fixed, accepted, critical_open, status, created_at, approved_at`

// GetByID returns the entry with the given id, or ErrNotFound.
func (db *DB) GetByID(id string) (Entry, error) {
	row := db.sql.QueryRow(`SELECT `+selectCols+` FROM entries WHERE id = ?`, id)
	e, err := scanEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	if err != nil {
		return Entry{}, err
	}
	return e, nil
}

func (db *DB) queryList(query string, args ...any) ([]Entry, error) {
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListApproved returns approved entries, newest approved_at first.
func (db *DB) ListApproved() ([]Entry, error) {
	return db.queryList(`SELECT ` + selectCols + ` FROM entries WHERE status = '` + StatusApproved + `' ORDER BY approved_at DESC`)
}

// ListPending returns pending entries, oldest created_at first.
func (db *DB) ListPending() ([]Entry, error) {
	return db.queryList(`SELECT ` + selectCols + ` FROM entries WHERE status = '` + StatusPending + `' ORDER BY created_at ASC`)
}

// ListHidden returns hidden entries, oldest created_at first.
func (db *DB) ListHidden() ([]Entry, error) {
	return db.queryList(`SELECT ` + selectCols + ` FROM entries WHERE status = '` + StatusHidden + `' ORDER BY created_at ASC`)
}

// CountPending returns the number of pending entries.
func (db *DB) CountPending() (int, error) {
	var n int
	err := db.sql.QueryRow(`SELECT COUNT(*) FROM entries WHERE status = ?`, StatusPending).Scan(&n)
	return n, err
}

// Approve sets an entry's status to approved and stamps approved_at. Returns
// ErrNotFound if the id does not exist.
func (db *DB) Approve(id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.sql.Exec(`UPDATE entries SET status = ?, approved_at = ? WHERE id = ?`, StatusApproved, now, id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res)
}

// Hide sets an entry's status to hidden. Returns ErrNotFound if the id does
// not exist.
func (db *DB) Hide(id string) error {
	res, err := db.sql.Exec(`UPDATE entries SET status = ? WHERE id = ?`, StatusHidden, id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res)
}

func checkRowsAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
