//go:build cgo

package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestProxiedRevisionWriteVerbs(t *testing.T) {
	requireSharedProxiedServer(t)
	bd := buildEmbeddedBD(t)
	for _, verb := range []string{"assign", "close", "delete"} {
		t.Run(verb, func(t *testing.T) {
			for _, ephemeral := range []bool{false, true} {
				p := newSharedProxiedProject(t, bd, "rvp")
				createArgs := []string{"Revision guarded " + verb}
				if ephemeral {
					createArgs = append(createArgs, "--ephemeral")
				}
				issue := bdProxiedCreate(t, bd, p.dir, createArgs...)
				selected := bdProxiedShowRevision(t, bd, p.dir, issue.ID)
				bdProxiedUpdateOne(t, bd, p.dir, issue.ID, "--notes", "newer authority")
				current := bdProxiedShowRevision(t, bd, p.dir, issue.ID)
				args := func(revision int64) []string {
					result := []string{verb, issue.ID}
					if verb == "assign" {
						result = append(result, "worker")
					}
					if verb == "delete" {
						result = append(result, "--force")
					}
					return append(result, "--if-revision", revStr(revision), "--json")
				}
				before := bdProxiedShowRaw(t, bd, p.dir, issue.ID, "--json")
				out, err := bdProxiedRun(t, bd, p.dir, args(selected)...)
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != ExitGuardMismatch {
					t.Fatalf("%s stale exit: %v %s", verb, err, out)
				}
				if !strings.Contains(string(out), `"code":"precondition_failed"`) {
					t.Fatalf("missing typed failure: %s", out)
				}
				if after := bdProxiedShowRaw(t, bd, p.dir, issue.ID, "--json"); after != before {
					t.Fatal("stale write changed issue")
				}
				if out, err := bdProxiedRun(t, bd, p.dir, args(current)...); err != nil {
					t.Fatalf("matching revision: %v %s", err, out)
				}
				if verb == "delete" {
					if out, err := bdProxiedRun(t, bd, p.dir, "show", issue.ID, "--json"); err == nil {
						t.Fatalf("delete left issue: %s", out)
					}
				} else {
					got := bdProxiedShow(t, bd, p.dir, issue.ID)
					if verb == "assign" && got.Assignee != "worker" {
						t.Fatal("assign did not persist")
					}
					if verb == "close" && got.Status != "closed" {
						t.Fatal("close did not persist")
					}
				}
			}
		})
	}
}

// Guarded close retains CLI policy and retry-safe molecule completion.
func TestProxiedRevisionClosePolicyAndRetry(t *testing.T) {
	requireSharedProxiedServer(t)
	bd := buildEmbeddedBD(t)
	p := newSharedProxiedProject(t, bd, "rcp")
	dir := p.dir
	pinned := bdProxiedCreate(t, bd, dir, "Pinned")
	bdProxiedUpdateOne(t, bd, dir, pinned.ID, "--status", "pinned")
	revision := bdProxiedShowRevision(t, bd, dir, pinned.ID)
	out, err := bdProxiedRun(t, bd, dir, "close", pinned.ID, "--if-revision", revStr(revision))
	if err == nil || !strings.Contains(strings.ToLower(string(out)), "pinned") {
		t.Fatalf("pin policy: %v %s", err, out)
	}
	if got := bdProxiedShowRevision(t, bd, dir, pinned.ID); got != revision {
		t.Fatal("refusal changed revision")
	}
	root := bdProxiedCreate(t, bd, dir, "Molecule", "--type", "epic", "--labels", "template")
	step := bdProxiedCreate(t, bd, dir, "Step", "--parent", root.ID)
	if out, err := bdProxiedRun(t, bd, dir, "close", step.ID, "--if-revision", revStr(bdProxiedShowRevision(t, bd, dir, step.ID))); err != nil {
		t.Fatalf("close: %v %s", err, out)
	}
	if bdProxiedShow(t, bd, dir, root.ID).Status != "closed" {
		t.Fatal("guarded close did not complete molecule")
	}
	if out, err := bdProxiedRun(t, bd, dir, "reopen", root.ID); err != nil {
		t.Fatalf("reopen: %v %s", err, out)
	}
	if out, err := bdProxiedRun(t, bd, dir, "close", step.ID, "--if-revision", revStr(bdProxiedShowRevision(t, bd, dir, step.ID))); err != nil {
		t.Fatalf("close: %v %s", err, out)
	}
	if bdProxiedShow(t, bd, dir, root.ID).Status != "closed" {
		t.Fatal("retry did not complete stranded molecule")
	}
}
