package postgres

import (
	"strings"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestSearchUsersQuery(t *testing.T) {
	t.Parallel()

	q, err := user.NewSearchQuery("ann lee", user.SearchFilters{City: "Москва", Film: "50%"})
	if err != nil {
		t.Fatal(err)
	}

	conds := searchConditions(q)
	sql := searchUsersQuery(conds)

	for _, want := range []string{
		"AND id < $1", searchHaystack + " LIKE $3", searchHaystack + " LIKE $4",
		"lower(city) = $5", "lower(fav_films) LIKE $6", "LIMIT $2",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("query lacks %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "$7") || strings.Contains(sql, "LIKE ALL") {
		t.Fatalf("one condition per word and filter:\n%s", sql)
	}
	if conds[2].param != "москва" || conds[3].param != `%50\%%` {
		t.Fatalf("params = %+v", conds)
	}
	// The migration indexes exactly this expression.
	if searchHaystack != "lower(nickname || ' ' || name || ' ' || surname)" {
		t.Fatalf("haystack drifted from users_search_idx: %s", searchHaystack)
	}
}
