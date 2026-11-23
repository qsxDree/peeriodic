package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedMigrations(t *testing.T) {
	entries, err := fs.ReadDir(MigrationFS, ".")
	if err != nil {
		t.Fatalf("failed to read embedded migrations: %v", err)
	}

	foundSQL := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			foundSQL = true
			content, err := MigrationFS.ReadFile(entry.Name())
			if err != nil {
				t.Errorf("failed to read migration %s: %v", entry.Name(), err)
			}
			if len(content) == 0 {
				t.Errorf("migration file %s is empty", entry.Name())
			}
		}
	}

	if !foundSQL {
		t.Errorf("expected at least one embedded SQL migration file")
	}
}
