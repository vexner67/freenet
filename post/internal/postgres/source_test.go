package postgres_test

import (
	"testing"

	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/sourcetest"
)

func TestSourceRepository_Sync(t *testing.T) {
	sourcetest.RunSync(t, postgres.NewSourceRepository(newPool(t)))
}
