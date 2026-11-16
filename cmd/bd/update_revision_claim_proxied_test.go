//go:build cgo

package main

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProxiedServerUpdateRevisionClaim(t *testing.T) {
	requireSharedProxiedServer(t)
	bd := buildEmbeddedBD(t)
	p := newSharedProxiedProject(t, bd, "rpc")
	run := func(args ...string) {
		t.Helper()
		if out, err := bdProxiedRun(t, bd, p.dir, args...); err != nil {
			t.Fatalf("proxied command %v: %v\n%s", args, err, out)
		}
	}
	show := func(id string) map[string]interface{} {
		t.Helper()
		raw := bdProxiedShowRaw(t, bd, p.dir, id, "--json")
		start := strings.IndexByte(raw, '{')
		if start < 0 {
			t.Fatalf("missing show object: %s", raw)
		}
		var result map[string]interface{}
		if err := json.NewDecoder(strings.NewReader(raw[start:])).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	t.Run("claim_with_native_lease_and_metadata", func(t *testing.T) {
		issue := bdProxiedCreate(t, bd, p.dir, "Guarded native claim")
		selected := show(issue.ID)["revision"].(string)
		run("--actor", "revision-worker", "update", issue.ID, "--claim", "--if-revision", selected, "--set-metadata", "reservation=owned")
		got := show(issue.ID)
		if got["assignee"] != "revision-worker" || got["status"] != "in_progress" {
			t.Fatalf("claim not acquired: %#v", got)
		}
		metadata, ok := got["metadata"].(map[string]interface{})
		if !ok || metadata["reservation"] != "owned" {
			t.Fatalf("metadata missing: %#v", got)
		}
		lease, _ := got["lease_expires_at"].(string)
		expires, err := time.Parse(time.RFC3339Nano, lease)
		if err != nil || !expires.After(time.Now()) {
			t.Fatalf("native lease missing or expired: %#v", got)
		}
	})
	for _, authority := range []string{"metadata", "label", "dependency"} {
		t.Run("stale_"+authority, func(t *testing.T) {
			issue := bdProxiedCreate(t, bd, p.dir, "Stale guarded claim")
			selected := show(issue.ID)["revision"].(string)
			switch authority {
			case "metadata":
				run("update", issue.ID, "--set-metadata", "hold=newer")
			case "label":
				run("label", "add", issue.ID, "hold:operator")
			case "dependency":
				blocker := bdProxiedCreate(t, bd, p.dir, "New prerequisite")
				run("dep", "add", issue.ID, blocker.ID)
			}
			before := show(issue.ID)
			out, code := bdProxiedUpdateFailCode(t, bd, p.dir, issue.ID, "--actor", "revision-worker", "--claim", "--if-revision", selected, "--set-metadata", "reservation=owned", "--json")
			if code != ExitGuardMismatch {
				t.Fatalf("exit=%d want %d: %s", code, ExitGuardMismatch, out)
			}
			if !strings.Contains(out, `"code":"precondition_failed"`) {
				t.Errorf("missing typed stale verdict: %s", out)
			}
			after := show(issue.ID)
			for _, key := range []string{"revision", "assignee", "status", "metadata", "labels", "dependencies", "lease_expires_at"} {
				if !reflect.DeepEqual(before[key], after[key]) {
					t.Errorf("stale claim changed %s: %#v -> %#v", key, before[key], after[key])
				}
			}
			if _, err := strconv.ParseInt(selected, 10, 64); err != nil {
				t.Fatalf("revision lost signed width: %s", selected)
			}
		})
	}
}

func TestProxiedServerUpdateRevisionMutations(t *testing.T) {
	requireSharedProxiedServer(t)
	bd := buildEmbeddedBD(t)
	p := newSharedProxiedProject(t, bd, "rpm")
	for _, ephemeral := range []bool{false, true} {
		create := func(title string) string {
			args := []string{title}
			if ephemeral {
				args = append(args, "--ephemeral")
			}
			return bdProxiedCreate(t, bd, p.dir, args...).ID
		}
		source, target := create("Guarded source"), create("Edge target")
		revision := func(id string) int64 {
			return revisionFromShowJSON(t, []byte(bdProxiedShowRaw(t, bd, p.dir, id, "--json")))
		}
		targetRevision := revision(target)
		for _, step := range []struct {
			name    string
			args    []string
			changed bool
		}{
			{"label_add", []string{"label", "add", source, "hold:operator"}, true},
			{"label_add_noop", []string{"label", "add", source, "hold:operator"}, false},
			{"label_replace", []string{"update", source, "--set-labels", "hold:review"}, true},
			{"label_replace_noop", []string{"update", source, "--set-labels", "hold:review"}, false},
			{"label_remove", []string{"label", "remove", source, "hold:review"}, true},
			{"label_remove_noop", []string{"label", "remove", source, "hold:review"}, false},
			{"dep_add", []string{"dep", "add", source, target}, true},
			{"dep_add_noop", []string{"dep", "add", source, target}, false},
			{"dep_remove", []string{"dep", "remove", source, target}, true},
			{"dep_remove_noop", []string{"dep", "remove", source, target}, false},
		} {
			before := revision(source)
			if out, err := bdProxiedRun(t, bd, p.dir, step.args...); err != nil {
				t.Fatalf("%s: %v %s", step.name, err, out)
			}
			after := revision(source)
			if changed := before != after; changed != step.changed {
				t.Errorf("%s ephemeral=%v: revision changed=%v want=%v", step.name, ephemeral, changed, step.changed)
			}
			if revision(target) != targetRevision {
				t.Fatalf("%s changed unrelated target revision", step.name)
			}
			if step.changed {
				snapshot := bdProxiedShowRaw(t, bd, p.dir, source, "--json")
				out, code := bdProxiedUpdateFailCode(t, bd, p.dir, source, "--if-revision", strconv.FormatInt(before, 10), "--set-metadata", "route=restored", "--json")
				if code != ExitGuardMismatch {
					t.Fatalf("stale restore %s: exit=%d %s", step.name, code, out)
				}
				if got := bdProxiedShowRaw(t, bd, p.dir, source, "--json"); got != snapshot {
					t.Fatalf("stale restore changed source after %s", step.name)
				}
			}
		}
	}
}
