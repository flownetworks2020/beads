//go:build cgo

package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The same public-command safety contract must hold through both adapters.
func TestRevisionGuardSafetyMatrix(t *testing.T) {
	for _, proxied := range []bool{false, true} {
		name := "direct"
		if proxied {
			name = "proxied"
		}
		t.Run(name, func(t *testing.T) {
			if proxied {
				requireSharedProxiedServer(t)
			} else if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
				t.Skip("embedded Dolt required")
			}
			bd := buildEmbeddedBD(t)
			var dir string
			if proxied {
				dir = newSharedProxiedProject(t, bd, "rgs").dir
			} else {
				dir, _, _ = bdInit(t, bd, "--prefix", "rgs")
			}
			run := func(args ...string) ([]byte, error) {
				if proxied {
					return bdProxiedRun(t, bd, dir, args...)
				}
				return bdRunWithFlockRetry(t, bd, dir, args...)
			}
			ok := func(args ...string) []byte {
				t.Helper()
				out, err := run(args...)
				if err != nil {
					t.Fatalf("%v: %v %s", args, err, out)
				}
				return out
			}
			object := func(raw []byte) map[string]interface{} {
				t.Helper()
				start := strings.IndexByte(string(raw), '{')
				if start < 0 {
					t.Fatalf("no JSON object: %s", raw)
				}
				var value map[string]interface{}
				if err := json.NewDecoder(strings.NewReader(string(raw[start:]))).Decode(&value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			show := func(id string) map[string]interface{} { return object(ok("show", id, "--json")) }
			create := func(args ...string) string {
				return object(ok(append(append([]string{"create"}, args...), "--json")...))["id"].(string)
			}
			refusedUnchanged := func(id string, args ...string) {
				t.Helper()
				before := show(id)
				if out, err := run(args...); err == nil {
					t.Fatalf("unexpected success %v: %s", args, out)
				}
				if after := show(id); !reflect.DeepEqual(before, after) {
					t.Fatalf("refusal changed %s: %#v -> %#v", id, before, after)
				}
			}
			for _, ephemeral := range []bool{false, true} {
				args := []string{"Guarded owner"}
				if ephemeral {
					args = append(args, "--ephemeral")
				}
				id := create(args...)
				selected := show(id)["revision"].(string)
				ok("--actor", "owner", "update", id, "--claim", "--if-revision", selected, "--set-metadata", "reservation=owned")
				// Simulate a lost success response: read back identity and refuse a blind
				// replay with the old selection before retrying with the observed token.
				// Native storage intentionally leases durable work only; wisps are ephemeral.
				acquired := show(id)
				if acquired["assignee"] != "owner" || (!ephemeral && acquired["lease_expires_at"] == nil) {
					t.Fatalf("claim receipt missing: %#v", acquired)
				}
				out, err := run("--actor", "owner", "update", id, "--claim", "--if-revision", selected)
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != ExitGuardMismatch {
					t.Fatalf("blind replay: %v %s", err, out)
				}
				revision := acquired["revision"].(string)
				refusedUnchanged(id, "--actor", "stranger", "update", id, "--claim", "--if-revision", revision, "--set-metadata", "reservation=stolen")
				refusedUnchanged(id, "--actor", "stranger", "close", id, "--if-revision", revision)
				ok("--actor", "owner", "update", id, "--claim", "--if-revision", revision)
				after := show(id)
				if after["assignee"] != "owner" || after["lease_expires_at"] != acquired["lease_expires_at"] {
					t.Fatal("same-actor replay replaced claim identity or lease")
				}
			}
			for _, empty := range []bool{false, true} {
				id := create("Guarded metadata restoration")
				metadata := `{"hold":"operator","predecessor_outcome":"pass","custody":"existing","repair":"pending"}`
				if empty {
					metadata = `{"route":"","hold":"operator","predecessor_outcome":"pass","custody":"existing","repair":"pending"}`
				}
				ok("update", id, "--metadata", metadata)
				selected := show(id)["revision"].(string)
				ok("update", id, "--if-revision", selected, "--set-metadata", "route=rig/worker", "--unset-metadata", "repair")
				got := show(id)["metadata"].(map[string]interface{})
				if got["route"] != "rig/worker" || got["hold"] != "operator" || got["predecessor_outcome"] != "pass" || got["custody"] != "existing" {
					t.Fatalf("restoration lost authority: %#v", got)
				}
				if _, exists := got["repair"]; exists {
					t.Fatal("cleanup did not apply in the guarded patch")
				}
				refusedUnchanged(id, "update", id, "--if-revision", selected, "--set-metadata", "route=other", "--unset-metadata", "hold")
			}

			// A matching token cannot turn an abbreviated destructive target into an
			// exact ID. Test the token of the survivor, not an unrelated stale token.
			survivor := create("Longer identifier", "--id", "rgs-t000031")
			refusedUnchanged(survivor, "delete", "rgs-t00003", "--force", "--if-revision", show(survivor)["revision"].(string))
			for _, kind := range []string{"boolean_pin", "timer_gate"} {
				var id string
				if kind == "boolean_pin" {
					file := filepath.Join(dir, "pinned-plan.json")
					if err := os.WriteFile(file, []byte(`{"nodes":[{"key":"p","title":"Pinned","type":"task","pinned":true}]}`), 0600); err != nil {
						t.Fatal(err)
					}
					result := object(ok("create", "--graph", file, "--json"))
					id = result["ids"].(map[string]interface{})["p"].(string)
				} else {
					task := create("Waits for gate")
					id = object(ok("gate", "create", "--blocks", task, "--type", "timer", "--timeout", "2h", "--json"))["id"].(string)
				}
				refusedUnchanged(id, "close", id, "--if-revision", show(id)["revision"].(string))
			}
		})
	}
}
