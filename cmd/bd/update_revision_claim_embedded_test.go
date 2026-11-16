//go:build cgo

package main

import (
	"os"
	"reflect"
	"testing"
	"time"
)

// Exercise the public command against isolated real storage. A claim and its
// metadata must share the revision precondition, including a competing edit
// that leaves status and assignee unchanged.
func TestUpdateRevisionClaimCLI(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 for the real provider proof")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "rc")
	revision := func(id string) string {
		t.Helper()
		value, ok := bdShowDetails(t, bd, dir, id)["revision"].(string)
		if !ok || value == "" {
			t.Fatal("show did not preserve the opaque revision as a string")
		}
		return value
	}
	t.Run("claim_and_metadata_with_matching_revision", func(t *testing.T) {
		issue := bdCreate(t, bd, dir, "Revision guarded claim", "--type", "task")
		before := revision(issue.ID)
		bdRunOK(t, bd, dir, "--actor", "revision-worker", "update", issue.ID,
			"--claim", "--if-revision", before, "--set-metadata", "reservation=owned")
		got := bdShowDetails(t, bd, dir, issue.ID)
		if got["assignee"] != "revision-worker" || got["status"] != "in_progress" {
			t.Fatalf("claim did not acquire the issue: %#v", got)
		}
		metadata, ok := got["metadata"].(map[string]interface{})
		if !ok || metadata["reservation"] != "owned" {
			t.Fatalf("claim metadata missing: %#v", got["metadata"])
		}
		lease, ok := got["lease_expires_at"].(string)
		expires, err := time.Parse(time.RFC3339Nano, lease)
		if !ok || err != nil || !expires.After(time.Now()) {
			t.Fatalf("native claim lease missing or expired: %#v", got)
		}
		if revision(issue.ID) == before {
			t.Fatal("successful claim did not advance revision")
		}
	})
	t.Run("same_status_edit_invalidates_claim_and_patch", func(t *testing.T) {
		issue := bdCreate(t, bd, dir, "Concurrent authority edit", "--type", "task")
		selected := revision(issue.ID)
		bdUpdate(t, bd, dir, issue.ID, "--set-metadata", "hold=newer")
		before := bdShowDetails(t, bd, dir, issue.ID)
		out, code := bdRunFailCode(t, bd, dir, "--actor", "revision-worker", "update", issue.ID,
			"--claim", "--if-revision", selected, "--unset-metadata", "hold", "--set-metadata", "reservation=owned")
		if code != ExitGuardMismatch {
			t.Fatalf("stale claim exit=%d, want %d: %s", code, ExitGuardMismatch, out)
		}
		after := bdShowDetails(t, bd, dir, issue.ID)
		for _, key := range []string{"status", "assignee", "metadata", "revision"} {
			if !reflect.DeepEqual(before[key], after[key]) {
				t.Errorf("stale claim changed %s: before=%#v after=%#v", key, before[key], after[key])
			}
		}
	})
}

// AT5: an authority change outside the issue row must invalidate the selection
// or refuse its claim. Merely preserving the label/edge is insufficient when a
// worker has already acquired work selected before that authority changed.
func TestUpdateRevisionClaimAuthorityRaceCLI(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 for the real provider proof")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ra")
	for _, authority := range []string{"label", "dependency"} {
		t.Run(authority, func(t *testing.T) {
			issue := bdCreate(t, bd, dir, "Work selected before authority edit", "--type", "task")
			selected := bdShowDetails(t, bd, dir, issue.ID)["revision"].(string)
			if authority == "label" {
				bdRunOK(t, bd, dir, "label", "add", issue.ID, "hold:operator")
			} else {
				blocker := bdCreate(t, bd, dir, "New unresolved prerequisite", "--type", "task")
				bdRunOK(t, bd, dir, "dep", "add", issue.ID, blocker.ID)
			}
			before := bdShowDetails(t, bd, dir, issue.ID)
			out, code := bdRunFailCode(t, bd, dir, "--actor", "revision-worker", "update", issue.ID,
				"--claim", "--if-revision", selected, "--set-metadata", "reservation=owned")
			if code == 0 {
				t.Fatalf("claim accepted after %s authority changed: %s", authority, out)
			}
			after := bdShowDetails(t, bd, dir, issue.ID)
			for _, key := range []string{"status", "assignee", "metadata", "revision", "labels", "dependencies", "lease_expires_at"} {
				if !reflect.DeepEqual(before[key], after[key]) {
					t.Errorf("refused claim changed %s: before=%#v after=%#v", key, before[key], after[key])
				}
			}
		})
	}
}

func TestUpdateRevisionLabelMutationsCLI(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 for the real provider proof")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "rl")
	issue := bdCreate(t, bd, dir, "Label revision invalidation", "--type", "task")
	for _, step := range []struct {
		name    string
		args    []string
		changed bool
	}{
		{"add", []string{"label", "add", issue.ID, "hold:operator"}, true},
		{"add_noop", []string{"label", "add", issue.ID, "hold:operator"}, false},
		{"replace", []string{"update", issue.ID, "--set-labels", "hold:review"}, true},
		{"replace_noop", []string{"update", issue.ID, "--set-labels", "hold:review"}, false},
		{"remove", []string{"label", "remove", issue.ID, "hold:review"}, true},
		{"remove_noop", []string{"label", "remove", issue.ID, "hold:review"}, false},
	} {
		t.Run(step.name, func(t *testing.T) {
			before := bdShowDetails(t, bd, dir, issue.ID)
			bdRunOK(t, bd, dir, step.args...)
			after := bdShowDetails(t, bd, dir, issue.ID)
			if changed := before["revision"] != after["revision"]; changed != step.changed {
				t.Errorf("revision changed=%v, want %v", changed, step.changed)
			}
		})
	}
}

func TestUpdateRevisionDependencyMutationsCLI(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 for the real provider proof")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "rd")
	source := bdCreate(t, bd, dir, "Dependency source", "--type", "task")
	target := bdCreate(t, bd, dir, "Dependency target", "--type", "task")
	targetRevision := bdShowDetails(t, bd, dir, target.ID)["revision"]
	for _, step := range []struct {
		name, verb string
		changed    bool
	}{
		{"add", "add", true}, {"add_noop", "add", false},
		{"remove", "remove", true}, {"remove_noop", "remove", false},
	} {
		t.Run(step.name, func(t *testing.T) {
			before := bdShowDetails(t, bd, dir, source.ID)["revision"]
			bdRunOK(t, bd, dir, "dep", step.verb, source.ID, target.ID)
			after := bdShowDetails(t, bd, dir, source.ID)["revision"]
			if changed := before != after; changed != step.changed {
				t.Errorf("source revision changed=%v, want %v", changed, step.changed)
			}
			if got := bdShowDetails(t, bd, dir, target.ID)["revision"]; got != targetRevision {
				t.Errorf("inbound-only edge edit changed target revision: %v -> %v", targetRevision, got)
			}
		})
	}
}

func TestUpdateRevisionWispClaimCLI(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 for the real provider proof")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "rw")
	for _, authority := range []string{"label", "dependency"} {
		t.Run(authority, func(t *testing.T) {
			issue := bdCreate(t, bd, dir, "Wisp guarded claim", "--type", "task", "--ephemeral")
			selected := bdShowDetails(t, bd, dir, issue.ID)["revision"].(string)
			if authority == "label" {
				bdRunOK(t, bd, dir, "label", "add", issue.ID, "hold:operator")
			} else {
				blocker := bdCreate(t, bd, dir, "Wisp prerequisite", "--type", "task", "--ephemeral")
				bdRunOK(t, bd, dir, "dep", "add", issue.ID, blocker.ID)
			}
			before := bdShowDetails(t, bd, dir, issue.ID)
			out, code := bdRunFailCode(t, bd, dir, "--actor", "revision-worker", "update", issue.ID, "--claim", "--if-revision", selected, "--set-metadata", "reservation=owned")
			if code != ExitGuardMismatch {
				t.Fatalf("stale wisp claim exit=%d: %s", code, out)
			}
			after := bdShowDetails(t, bd, dir, issue.ID)
			for _, key := range []string{"revision", "status", "assignee", "metadata", "labels", "dependencies", "lease_expires_at"} {
				if !reflect.DeepEqual(before[key], after[key]) {
					t.Errorf("stale wisp claim changed %s", key)
				}
			}
		})
	}
}

func TestUpdateRevisionDependencyReplacementCLI(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("requires embedded Dolt")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "rp")
	for _, ephemeral := range []bool{false, true} {
		args := []string{"--type", "task"}
		if ephemeral {
			args = append(args, "--ephemeral")
		}
		source := bdCreate(t, bd, dir, append([]string{"Child selected before parent replacement"}, args...)...)
		first := bdCreate(t, bd, dir, append([]string{"First parent"}, args...)...)
		second := bdCreate(t, bd, dir, append([]string{"Second parent"}, args...)...)
		bdUpdate(t, bd, dir, source.ID, "--parent", first.ID)
		selected := bdShowDetails(t, bd, dir, source.ID)["revision"].(string)
		bdUpdate(t, bd, dir, source.ID, "--parent", second.ID)
		before := bdShowDetails(t, bd, dir, source.ID)
		if before["revision"] == selected {
			t.Fatal("parent replacement kept stale revision")
		}
		out, code := bdRunFailCode(t, bd, dir, "update", source.ID, "--if-revision", selected, "--set-metadata", "route=restored", "--assignee", "worker")
		if code != ExitGuardMismatch {
			t.Fatalf("stale restore exit %d: %s", code, out)
		}
		after := bdShowDetails(t, bd, dir, source.ID)
		for _, key := range []string{"revision", "metadata", "assignee", "status", "dependencies", "lease_expires_at"} {
			if !reflect.DeepEqual(before[key], after[key]) {
				t.Errorf("stale restore changed %s", key)
			}
		}
	}
}

func TestUpdateRevisionDependencyDeletionCLI(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("requires embedded Dolt")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "rx")
	for _, ephemeral := range []bool{false, true} {
		args := []string{"--type", "task"}
		if ephemeral {
			args = append(args, "--ephemeral")
		}
		source := bdCreate(t, bd, dir, append([]string{"Surviving dependency source"}, args...)...)
		target := bdCreate(t, bd, dir, append([]string{"Deleted dependency target"}, args...)...)
		bdRunOK(t, bd, dir, "dep", "add", source.ID, target.ID, "--type", "relates-to")
		selected := bdShowDetails(t, bd, dir, source.ID)["revision"].(string)
		bdRunOK(t, bd, dir, "delete", target.ID, "--force")
		if got := bdShowDetails(t, bd, dir, source.ID)["revision"]; got == selected {
			t.Errorf("dependency removal by target deletion left source revision valid (ephemeral=%v)", ephemeral)
		}
	}
}

func TestUpdateRevisionDependencyRenameCLI(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("requires embedded Dolt")
	}
	bd := buildEmbeddedBD(t)
	for _, ephemeral := range []bool{false, true} {
		dir, _, _ := bdInit(t, bd, "--prefix", "rn")
		source := bdCreate(t, bd, dir, "Source selected before target rename")
		args := []string{"Target before rename"}
		if ephemeral {
			args = append(args, "--ephemeral")
		}
		target := bdCreate(t, bd, dir, args...)
		bdRunOK(t, bd, dir, "dep", "add", source.ID, target.ID, "--type", "relates-to")
		selected := bdShowDetails(t, bd, dir, source.ID)["revision"].(string)
		bdRunOK(t, bd, dir, "rename", target.ID, "rn-renamed")
		if got := bdShowDetails(t, bd, dir, source.ID)["revision"]; got == selected {
			t.Errorf("target rename retained stale source revision (wisp=%v)", ephemeral)
		}
	}
}
