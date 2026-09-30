// Package sqlite is a storage.Storage kept in a SQLite file, so queues
// survive restarts.
//
// It uses a pure-Go driver, so building needs no C compiler.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/storage"
)

var _ storage.Storage = (*Storage)(nil)

// queueColumns is the column list scanQueue expects, in its order.
const queueColumns = `id, chat_id, name, board_msg_id, created_by, created_at, closed`

type Storage struct {
	db *sql.DB
}

// Open opens the database at path, creating the file and its directory if
// needed, and brings the schema up to date.
func Open(path string) (*Storage, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// WAL lets reads run while a write is in progress; busy_timeout makes
	// a connection wait for a lock instead of failing at once.
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite allows one writer at a time. A single connection makes Go
	// queue the requests instead of SQLite rejecting them, and the bot's
	// load is far below what one connection handles.
	db.SetMaxOpenConns(1)

	if err := migrate(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Storage{db: db}, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) SaveUser(ctx context.Context, u model.User) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, first_name, last_name, username)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			first_name = excluded.first_name,
			last_name  = excluded.last_name,
			username   = excluded.username`,
		u.ID, u.FirstName, u.LastName, u.Username)
	if err != nil {
		return fmt.Errorf("save user %d: %w", u.ID, err)
	}
	return nil
}

func (s *Storage) User(ctx context.Context, id int64) (model.User, error) {
	u := model.User{ID: id}
	err := s.db.QueryRowContext(ctx,
		`SELECT first_name, last_name, username FROM users WHERE id = ?`, id).
		Scan(&u.FirstName, &u.LastName, &u.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return model.User{}, model.ErrNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("load user %d: %w", id, err)
	}
	return u, nil
}

func (s *Storage) CreateQueue(ctx context.Context, q model.Queue) (model.Queue, error) {
	err := inTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO queues (chat_id, name, board_msg_id, created_by, created_at, closed)
			VALUES (?, ?, ?, ?, ?, ?)`,
			q.ChatID, q.Name, q.BoardMsgID, q.CreatedBy, toUnix(q.CreatedAt), q.Closed)
		if err != nil {
			return err
		}
		if q.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		return insertEntries(ctx, tx, q.ID, q.Entries)
	})
	if err != nil {
		return model.Queue{}, fmt.Errorf("create queue: %w", err)
	}
	return q, nil
}

func (s *Storage) Queue(ctx context.Context, id int64) (model.Queue, error) {
	q, err := scanQueue(s.db.QueryRowContext(ctx,
		`SELECT `+queueColumns+` FROM queues WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Queue{}, model.ErrNotFound
	}
	if err != nil {
		return model.Queue{}, fmt.Errorf("load queue %d: %w", id, err)
	}

	if q.Entries, err = s.entries(ctx, id); err != nil {
		return model.Queue{}, fmt.Errorf("load entries of queue %d: %w", id, err)
	}
	return q, nil
}

// UpdateQueue replaces the queue and all its entries in one transaction,
// so a queue is never left half-saved.
func (s *Storage) UpdateQueue(ctx context.Context, q model.Queue) error {
	err := inTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE queues
			SET chat_id = ?, name = ?, board_msg_id = ?, created_by = ?, created_at = ?, closed = ?
			WHERE id = ?`,
			q.ChatID, q.Name, q.BoardMsgID, q.CreatedBy, toUnix(q.CreatedAt), q.Closed, q.ID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return model.ErrNotFound
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM entries WHERE queue_id = ?`, q.ID); err != nil {
			return err
		}
		return insertEntries(ctx, tx, q.ID, q.Entries)
	})
	if errors.Is(err, model.ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("update queue %d: %w", q.ID, err)
	}
	return nil
}

func (s *Storage) OpenQueues(ctx context.Context, chatID int64) ([]model.Queue, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+queueColumns+` FROM queues WHERE chat_id = ? AND NOT closed ORDER BY id`, chatID)
	if err != nil {
		return nil, fmt.Errorf("list open queues of chat %d: %w", chatID, err)
	}

	var out []model.Queue
	for rows.Next() {
		q, err := scanQueue(rows)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("list open queues of chat %d: %w", chatID, err)
		}
		out = append(out, q)
	}
	// Close before loading entries: with one connection, the entries query
	// would otherwise wait for these rows forever.
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list open queues of chat %d: %w", chatID, err)
	}

	for i := range out {
		if out[i].Entries, err = s.entries(ctx, out[i].ID); err != nil {
			return nil, fmt.Errorf("load entries of queue %d: %w", out[i].ID, err)
		}
	}
	return out, nil
}

// entries loads a queue's entries in their order.
func (s *Storage) entries(ctx context.Context, queueID int64) ([]model.Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, joined_at, done, done_at
		FROM entries WHERE queue_id = ?
		ORDER BY pos`, queueID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []model.Entry
	for rows.Next() {
		var (
			e              model.Entry
			joined, doneAt int64
		)
		if err := rows.Scan(&e.UserID, &joined, &e.Done, &doneAt); err != nil {
			return nil, err
		}
		e.JoinedAt, e.DoneAt = fromUnix(joined), fromUnix(doneAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

// insertEntries saves entries with their slice index as pos, which is what
// keeps their order.
func insertEntries(ctx context.Context, tx *sql.Tx, queueID int64, entries []model.Entry) error {
	for pos, e := range entries {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO entries (queue_id, pos, user_id, joined_at, done, done_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			queueID, pos, e.UserID, toUnix(e.JoinedAt), e.Done, toUnix(e.DoneAt))
		if err != nil {
			return err
		}
	}
	return nil
}

// inTx runs fn in a transaction, committing if it succeeds and rolling back
// if it fails.
func inTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// scanner is what *sql.Row and *sql.Rows have in common. The columns must
// be queueColumns.
type scanner interface {
	Scan(dest ...any) error
}

func scanQueue(row scanner) (model.Queue, error) {
	var (
		q       model.Queue
		created int64
	)
	err := row.Scan(&q.ID, &q.ChatID, &q.Name, &q.BoardMsgID, &q.CreatedBy, &created, &q.Closed)
	q.CreatedAt = fromUnix(created)
	return q, err
}

// Times are stored as Unix nanoseconds, with 0 meaning "not set": the zero
// time.Time is outside the range UnixNano can represent.
func toUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func fromUnix(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}
