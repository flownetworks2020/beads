//go:build cgo

package main

// Adapted from Julian Knutsen's gastownhall/beads#7203; retains the real CLI
// match/mismatch proof for every verb the engine capability probe requires.
import (
	"github.com/steveyegge/beads/internal/types"
	"os"
	"strconv"
	"strings"
	"testing"
)

func revStr(revision int64) string { return strconv.FormatInt(revision, 10) }

func TestIfRevisionMatchAndMismatch(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "mm")

	t.Run("update", func(t *testing.T) {
		issue := bdCreate(t, bd, dir, "Guarded update", "--type", "task", "--priority", "2")
		rev0 := bdShowRevision(t, bd, dir, issue.ID)
		// Bump the revision via an unguarded edit so rev0 is now stale.
		bdUpdate(t, bd, dir, issue.ID, "--notes", "bump")
		rev1 := bdShowRevision(t, bd, dir, issue.ID)
		if rev1 == rev0 {
			t.Fatalf("unguarded update did not change the revision")
		}

		out, code := bdUpdateFailCode(t, bd, dir, issue.ID, "--if-revision", revStr(rev0), "--priority", "3")
		if code != ExitGuardMismatch {
			t.Errorf("stale --if-revision exit code = %d, want %d\n%s", code, ExitGuardMismatch, out)
		}
		if !strings.Contains(out, "version mismatch") {
			t.Errorf("mismatch error should say \"version mismatch\", got:\n%s", out)
		}
		if got := bdShow(t, bd, dir, issue.ID); got.Priority != 2 {
			t.Errorf("stale --if-revision update still applied: priority = %d, want 2", got.Priority)
		}
		if got := bdShowRevision(t, bd, dir, issue.ID); got != rev1 {
			t.Errorf("stale --if-revision update still advanced the revision: %d, want unchanged %d", got, rev1)
		}

		bdUpdate(t, bd, dir, issue.ID, "--if-revision", revStr(rev1), "--priority", "3")
		if got := bdShow(t, bd, dir, issue.ID); got.Priority != 3 {
			t.Errorf("matching --if-revision update did not apply: priority = %d, want 3", got.Priority)
		}
		if got := bdShowRevision(t, bd, dir, issue.ID); got == rev1 {
			t.Errorf("matching --if-revision update did not advance the revision")
		}
	})

	t.Run("close", func(t *testing.T) {
		issue := bdCreate(t, bd, dir, "Guarded close", "--type", "task")
		rev0 := bdShowRevision(t, bd, dir, issue.ID)
		bdUpdate(t, bd, dir, issue.ID, "--notes", "bump")
		rev1 := bdShowRevision(t, bd, dir, issue.ID)

		out, code := bdRunFailCode(t, bd, dir, "close", issue.ID, "--if-revision", revStr(rev0))
		if code != ExitGuardMismatch {
			t.Errorf("stale --if-revision close exit code = %d, want %d\n%s", code, ExitGuardMismatch, out)
		}
		if !strings.Contains(out, "version mismatch") {
			t.Errorf("mismatch error should say \"version mismatch\", got:\n%s", out)
		}
		if got := bdShow(t, bd, dir, issue.ID); got.Status == types.StatusClosed {
			t.Errorf("stale --if-revision close still closed the issue")
		}
		if got := bdShowRevision(t, bd, dir, issue.ID); got != rev1 {
			t.Errorf("stale --if-revision close still advanced the revision: %d, want unchanged %d", got, rev1)
		}

		bdRunOK(t, bd, dir, "close", issue.ID, "--if-revision", revStr(rev1))
		if got := bdShow(t, bd, dir, issue.ID); got.Status != types.StatusClosed {
			t.Errorf("matching --if-revision close did not apply: status = %s, want closed", got.Status)
		}
	})

	t.Run("assign", func(t *testing.T) {
		issue := bdCreate(t, bd, dir, "Guarded assign", "--type", "task")
		rev0 := bdShowRevision(t, bd, dir, issue.ID)
		bdUpdate(t, bd, dir, issue.ID, "--notes", "bump")
		rev1 := bdShowRevision(t, bd, dir, issue.ID)

		out, code := bdRunFailCode(t, bd, dir, "assign", issue.ID, "worker", "--if-revision", revStr(rev0))
		if code != ExitGuardMismatch {
			t.Errorf("stale --if-revision assign exit code = %d, want %d\n%s", code, ExitGuardMismatch, out)
		}
		if !strings.Contains(out, "version mismatch") {
			t.Errorf("mismatch error should say \"version mismatch\", got:\n%s", out)
		}
		if got := bdShow(t, bd, dir, issue.ID); got.Assignee != "" {
			t.Errorf("stale --if-revision assign still applied: assignee = %q, want unassigned", got.Assignee)
		}
		if got := bdShowRevision(t, bd, dir, issue.ID); got != rev1 {
			t.Errorf("stale --if-revision assign still advanced the revision: %d, want unchanged %d", got, rev1)
		}

		bdRunOK(t, bd, dir, "assign", issue.ID, "worker", "--if-revision", revStr(rev1))
		if got := bdShow(t, bd, dir, issue.ID); got.Assignee != "worker" {
			t.Errorf("matching --if-revision assign did not apply: assignee = %q, want worker", got.Assignee)
		}
	})

	t.Run("delete", func(t *testing.T) {
		issue := bdCreate(t, bd, dir, "Guarded delete", "--type", "task")
		rev0 := bdShowRevision(t, bd, dir, issue.ID)
		bdUpdate(t, bd, dir, issue.ID, "--notes", "bump")
		rev1 := bdShowRevision(t, bd, dir, issue.ID)

		for _, flags := range [][]string{{"--dry-run"}, {"--force", "--cascade", "--dry-run"}, {"--force", "--cascade"}} {
			args := append([]string{"delete", issue.ID, "--if-revision", revStr(rev0)}, flags...)
			out, code := bdRunFailCode(t, bd, dir, args...)
			if code != ExitGuardMismatch {
				t.Fatalf("stale delete %v: %d %s", flags, code, out)
			}
			if bdShowRevision(t, bd, dir, issue.ID) != rev1 {
				t.Fatal("stale delete changed issue")
			}
		}

		out, code := bdRunFailCode(t, bd, dir, "delete", issue.ID, "--if-revision", revStr(rev0), "--force")
		if code != ExitGuardMismatch {
			t.Errorf("stale --if-revision delete exit code = %d, want %d\n%s", code, ExitGuardMismatch, out)
		}
		if !strings.Contains(out, "version mismatch") {
			t.Errorf("mismatch error should say \"version mismatch\", got:\n%s", out)
		}
		// The row must still exist: `bd show` must still succeed.
		bdShow(t, bd, dir, issue.ID)

		bdRunOK(t, bd, dir, "delete", issue.ID, "--if-revision", revStr(rev1), "--force")
		bdShowFail(t, bd, dir, issue.ID)
	})
}

// Guarded close retains CLI policy and retry-safe molecule completion.
func TestRevisionClosePolicyAndRetry(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("embedded Dolt required")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "rcp")
	pinned := bdCreate(t, bd, dir, "Pinned")
	bdUpdate(t, bd, dir, pinned.ID, "--status", "pinned")
	revision := bdShowRevision(t, bd, dir, pinned.ID)
	out, code := bdRunFailCode(t, bd, dir, "close", pinned.ID, "--if-revision", revStr(revision))
	if code == 0 || !strings.Contains(strings.ToLower(out), "pinned") {
		t.Fatalf("pin policy: %d %s", code, out)
	}
	if got := bdShowRevision(t, bd, dir, pinned.ID); got != revision {
		t.Fatal("refusal changed revision")
	}
	root := bdCreate(t, bd, dir, "Molecule", "--type", "epic", "--labels", "template")
	step := bdCreate(t, bd, dir, "Step", "--parent", root.ID)
	bdRunOK(t, bd, dir, "close", step.ID, "--if-revision", revStr(bdShowRevision(t, bd, dir, step.ID)))
	if bdShow(t, bd, dir, root.ID).Status != types.StatusClosed {
		t.Fatal("guarded close did not complete molecule")
	}
	bdReopen(t, bd, dir, root.ID)
	bdRunOK(t, bd, dir, "close", step.ID, "--if-revision", revStr(bdShowRevision(t, bd, dir, step.ID)))
	if bdShow(t, bd, dir, root.ID).Status != types.StatusClosed {
		t.Fatal("retry did not complete stranded molecule")
	}
}
