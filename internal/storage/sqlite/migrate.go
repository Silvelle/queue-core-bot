package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// migrations bring the schema up to date, one version at a time. The
// database remembers its version in PRAGMA user_version, so each migration
// runs exactly once. Never edit a migration that has shipped: add a new one.
var migrations = []string{
	// 1: users, queues and their entries.
	`
	CREATE TABLE users (
		id         INTEGER PRIMARY KEY,
		first_name TEXT NOT NULL DEFAULT '',
		last_name  TEXT NOT NULL DEFAULT '',
		username   TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE queues (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		chat_id      INTEGER NOT NULL,
		name         TEXT    NOT NULL,
		board_msg_id INTEGER NOT NULL DEFAULT 0,
		created_by   INTEGER NOT NULL,
		created_at   INTEGER NOT NULL,
		closed       BOOLEAN NOT NULL DEFAULT FALSE
	);

	CREATE INDEX queues_by_chat ON queues (chat_id, closed);

	-- pos is the entry's index in model.Queue.Entries, done entries
	-- included. Each user appears at most once per queue.
	CREATE TABLE entries (
		queue_id  INTEGER NOT NULL REFERENCES queues (id) ON DELETE CASCADE,
		pos       INTEGER NOT NULL,
		user_id   INTEGER NOT NULL,
		joined_at INTEGER NOT NULL DEFAULT 0,
		done      BOOLEAN NOT NULL DEFAULT FALSE,
		done_at   INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (queue_id, pos),
		UNIQUE (queue_id, user_id)
	);
	`,

	// 2: the message in each chat that lists its open queues. The bot no
	// longer tracks it, so the table is unused, but a shipped migration
	// is never changed or removed.
	`
	CREATE TABLE chats (
		id           INTEGER PRIMARY KEY,
		index_msg_id INTEGER NOT NULL DEFAULT 0
	);
	`,

	// 3: queues belong to a forum topic, and "done" is gone. People who had
	// defended were already off the board, so their entries are removed;
	// the done and done_at columns stay unused, as SQLite can't easily drop
	// them.
	`
	ALTER TABLE queues ADD COLUMN thread_id INTEGER NOT NULL DEFAULT 0;
	DELETE FROM entries WHERE done;
	CREATE INDEX queues_by_topic ON queues (chat_id, thread_id, closed);
	`,
}

func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this bot knows (%d)", version, len(migrations))
	}

	for v := version; v < len(migrations); v++ {
		// The migration and its version number are saved together, so a
		// crash can't leave a migration applied but not recorded.
		err := inTx(ctx, db, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
				return err
			}
			// PRAGMA doesn't take ? parameters; v+1 is our own number.
			_, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, v+1))
			return err
		})
		if err != nil {
			return fmt.Errorf("migrate schema to version %d: %w", v+1, err)
		}
	}
	return nil
}
