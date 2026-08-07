package migrations_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/nori-kamiya/xordpress/internal/storage/migrations"
)

// TestFilenamesHaveStrictlyIncreasingNumericPrefix guards against the class
// of bug fixed on 0006_sessions_meta.sql (originally drafted as 0004, but
// 0005_mfa.sql had already shipped): storage.Migrate assigns each file's
// schema_migrations version by its position in the lexically sorted file
// list, not by parsing the filename's numeric prefix. Inserting a new file
// whose prefix sorts before an already-shipped file silently desyncs
// version numbers on any database that already applied the later file.
func TestFilenamesHaveStrictlyIncreasingNumericPrefix(t *testing.T) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	var prev int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		prefix, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			t.Fatalf("migration filename %q has no underscore-delimited numeric prefix", e.Name())
		}
		n, err := strconv.Atoi(prefix)
		if err != nil {
			t.Fatalf("migration filename %q has non-numeric prefix: %v", e.Name(), err)
		}
		if n <= prev {
			t.Fatalf("migration filename %q has prefix %d, which does not sort strictly after the previous file's %d", e.Name(), n, prev)
		}
		prev = n
	}
}
