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
		"info_read = $5", "lower(city) = $6", "lower(fav_films) LIKE $7", "LIMIT $2",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("query lacks %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "$8") || strings.Contains(sql, "LIKE ALL") {
		t.Fatalf("one condition per word and filter:\n%s", sql)
	}
	if conds[2].param != "everyone" || conds[3].param != "москва" || conds[4].param != `%50\%%` {
		t.Fatalf("params = %+v", conds)
	}
	// The migration indexes exactly this expression.
	if searchHaystack != "lower(nickname || ' ' || name || ' ' || surname)" {
		t.Fatalf("haystack drifted from users_search_idx: %s", searchHaystack)
	}
}
