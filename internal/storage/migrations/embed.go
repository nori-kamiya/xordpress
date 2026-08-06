// Package migrations embeds the SQL migration files applied by storage.Migrate.
package migrations

import "embed"

// FS holds the embedded migration SQL files, sorted by filename when read via fs.ReadDir.
//
//go:embed *.sql
var FS embed.FS
