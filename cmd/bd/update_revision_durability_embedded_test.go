//go:build cgo

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
)

// Reading HEAD after reopening distinguishes a durable token from a token
// present only in the working set. A subsequent process reopen alone cannot.
func TestUpdateRevisionCommittedTokenCLI(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("requires embedded Dolt")
	}
	bd := buildEmbeddedBD(t)
	dir, beadsDir, _ := bdInit(t, bd, "--prefix", "rt")
	source := bdCreate(t, bd, dir, "Durable source")
	target := bdCreate(t, bd, dir, "Durable target")
	wisp := bdCreate(t, bd, dir, "Ephemeral target", "--ephemeral")
	steps := [][]string{
		{"label", "add", source.ID, "hold:operator"},
		{"update", source.ID, "--set-labels", "hold:review"},
		{"label", "remove", source.ID, "hold:review"},
		{"dep", "add", source.ID, target.ID, "--type", "relates-to"},
		{"dep", "remove", source.ID, target.ID},
		{"dep", "add", source.ID, wisp.ID, "--type", "relates-to"},
		{"delete", wisp.ID, "--force"},
	}
	for _, args := range steps {
		before := bdShowDetails(t, bd, dir, source.ID)["revision"].(string)
		bdRunOK(t, bd, dir, args...)
		after := bdShowDetails(t, bd, dir, source.ID)["revision"].(string)
		if before == after {
			t.Fatalf("%v did not invalidate selected revision", args)
		}
		cfg, err := configfile.Load(beadsDir)
		if err != nil {
			t.Fatal(err)
		}
		db, cleanup, err := embeddeddolt.OpenSQL(t.Context(), filepath.Join(beadsDir, "embeddeddolt"), cfg.GetDoltDatabase(), "main")
		if err != nil {
			t.Fatal(err)
		}
		var committed int64
		err = db.QueryRowContext(t.Context(), "SELECT row_lock FROM issues AS OF 'HEAD' WHERE id = ?", source.ID).Scan(&committed)
		cleanup()
		if err != nil {
			t.Fatal(err)
		}
		if strconv.FormatInt(committed, 10) != after {
			t.Errorf("%v: committed token %d differs from reopened token %s", args, committed, after)
		}
	}
}
