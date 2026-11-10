// Package migrations embeds the SQL migrations into the binary, so the
// service can apply them on start (MIGRATE_ON_START).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
