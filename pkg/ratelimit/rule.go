package ratelimit

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rule allows at most Limit events per Window.
type Rule struct {
	Limit  int
	Window time.Duration
}

func (r Rule) String() string {
	return fmt.Sprintf("%d/%s", r.Limit, r.Window)
}

// Rules all apply at once, written as "5/1m,100/24h": a short window
// stops bursts, a long one slow spam. Empty means no limit.
type Rules []Rule

var errBadRule = errors.New(`want rules like "5/1m,100/24h"`)

// UnmarshalText reads rules from configuration.
func (rs *Rules) UnmarshalText(text []byte) error {
	*rs = nil

	s := strings.TrimSpace(string(text))
	if s == "" {
		return nil
	}

	for _, item := range strings.Split(s, ",") {
		limit, window, ok := strings.Cut(strings.TrimSpace(item), "/")
		if !ok {
			return fmt.Errorf("%q: %w", item, errBadRule)
		}

		n, err := strconv.Atoi(limit)
		if err != nil || n <= 0 {
			return fmt.Errorf("%q: %w", item, errBadRule)
		}

		d, err := time.ParseDuration(window)
		if err != nil || d <= 0 {
			return fmt.Errorf("%q: %w", item, errBadRule)
		}

		*rs = append(*rs, Rule{Limit: n, Window: d})
	}

	return nil
}

func (rs Rules) String() string {
	items := make([]string, len(rs))
	for i, r := range rs {
		items[i] = r.String()
	}

	return strings.Join(items, ",")
}
