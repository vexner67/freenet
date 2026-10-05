package postgres

import (
	"strings"
	"testing"
)

func TestSearchUsersQuery(t *testing.T) {
	t.Parallel()

	q := searchUsersQuery(2)

	for _, want := range []string{"AND id < $1", searchHaystack + " LIKE $3", searchHaystack + " LIKE $4", "LIMIT $2"} {
		if !strings.Contains(q, want) {
			t.Fatalf("query lacks %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "$5") || strings.Contains(q, "LIKE ALL") {
		t.Fatalf("one LIKE per word:\n%s", q)
	}
	// The migration indexes exactly this expression.
	if searchHaystack != "lower(nickname || ' ' || name || ' ' || surname)" {
		t.Fatalf("haystack drifted from users_search_idx: %s", searchHaystack)
	}
}
