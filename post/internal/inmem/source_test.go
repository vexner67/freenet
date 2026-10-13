package inmem_test

import (
	"testing"

	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/sourcetest"
)

func TestSources_Sync(t *testing.T) {
	t.Parallel()

	sourcetest.RunSync(t, inmem.NewStore().Sources())
}
