package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/service"
	"github.com/Silvelle/queue-core-bot/internal/storage"
	"github.com/Silvelle/queue-core-bot/internal/storage/storagetest"
)

// open creates a storage in a fresh file that is deleted after the test.
func open(t *testing.T, path string) *Storage {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStorage(t *testing.T) {
	storagetest.Run(t, func(t *testing.T) storage.Storage {
		return open(t, filepath.Join(t.TempDir(), "queue.db"))
	})
}

// The whole point of this package: data is still there after a restart.
func TestDataSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "queue.db")

	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	q, err := first.CreateQueue(ctx, model.Queue{ChatID: 10, Name: "Practice 4", Entries: []model.Entry{{UserID: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SaveUser(ctx, model.User{ID: 1, FirstName: "Anna"}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second := open(t, path)
	got, err := second.Queue(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Practice 4" || len(got.Entries) != 1 {
		t.Errorf("after reopening: queue = %+v, want Practice 4 with 1 entry", got)
	}
	if u, err := second.User(ctx, 1); err != nil || u.FirstName != "Anna" {
		t.Errorf("after reopening: user = %+v, %v, want Anna", u, err)
	}
}

// Opening an up-to-date database must not run the migrations again.
func TestMigrateTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	open(t, path)
	s := open(t, path)

	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) {
		t.Errorf("schema version = %d, want %d", version, len(migrations))
	}
}

// A database from a newer bot must not be opened by an older one, which
// could damage data it doesn't understand.
func TestRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	s := open(t, path)
	if _, err := s.db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	if _, err := Open(path); err == nil {
		t.Error("Open accepted a database with a newer schema")
	}
}

// The database itself refuses a user twice in one queue, as a last line of
// defense behind the service's rules.
func TestRejectsDuplicateUser(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "queue.db"))

	_, err := s.CreateQueue(context.Background(), model.Queue{ChatID: 10, Entries: []model.Entry{{UserID: 1}, {UserID: 1}}})
	if err == nil {
		t.Error("CreateQueue accepted the same user twice")
	}
}

// A failed update must leave the stored queue as it was.
func TestFailedUpdateChangesNothing(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "queue.db"))

	q, err := s.CreateQueue(ctx, model.Queue{ChatID: 10, Name: "A", Entries: []model.Entry{{UserID: 1}}})
	if err != nil {
		t.Fatal(err)
	}

	broken := q
	broken.Name = "B"
	broken.Entries = []model.Entry{{UserID: 2}, {UserID: 2}}
	if err := s.UpdateQueue(ctx, broken); err == nil {
		t.Fatal("UpdateQueue accepted the same user twice")
	}

	got, err := s.Queue(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "A" || len(got.Entries) != 1 || got.Entries[0].UserID != 1 {
		t.Errorf("after a failed update: %+v, want the original queue", got)
	}
}

func TestUpdateUnknownQueue(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "queue.db"))
	if err := s.UpdateQueue(context.Background(), model.Queue{ID: 5}); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// Open creates missing directories, so DB_PATH=data/queue.db works on a
// fresh checkout.
func TestOpenCreatesDirectory(t *testing.T) {
	open(t, filepath.Join(t.TempDir(), "nested", "dir", "queue.db"))
}

// The service's lock plus SQLite's single connection must keep a queue
// valid when many people press Join at the same moment, as they do right
// after /new.
func TestServiceConcurrentJoin(t *testing.T) {
	ctx := context.Background()
	svc := service.New(open(t, filepath.Join(t.TempDir(), "queue.db")))

	q, err := svc.Create(ctx, 10, "Practice 4", 1)
	if err != nil {
		t.Fatal(err)
	}

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			if _, err := svc.Join(ctx, q.ID, int64(i+1)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	got, err := svc.Queue(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	waiting := got.Waiting()
	if len(waiting) != n {
		t.Fatalf("%d waiting, want %d", len(waiting), n)
	}
	for i, e := range waiting {
		if pos := got.Position(e.UserID); pos != i+1 {
			t.Fatalf("user %d at position %d, want %d", e.UserID, pos, i+1)
		}
	}
}

// A database made by the previous version of the bot, like the one already
// on a server, is upgraded on start and keeps its queues.
func TestUpgradeFromVersion1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "queue.db")

	// Build a version 1 database by hand, the way the old bot left it.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0] + `PRAGMA user_version = 1;`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO queues (chat_id, name, created_by, created_at) VALUES (10, 'Practice 4', 1, 0)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	s := open(t, path)
	queues, err := s.OpenQueues(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queues) != 1 || queues[0].Name != "Practice 4" {
		t.Errorf("after upgrade: open queues = %+v, want Practice 4", queues)
	}
	if err := s.SetIndexMessage(ctx, 10, 5); err != nil {
		t.Errorf("after upgrade, the new chats table doesn't work: %v", err)
	}
}
