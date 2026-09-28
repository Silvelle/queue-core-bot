package memory

import (
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/storage"
	"github.com/Silvelle/queue-core-bot/internal/storage/storagetest"
)

func TestStorage(t *testing.T) {
	storagetest.Run(t, func(*testing.T) storage.Storage { return New() })
}
