package dolt

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
)

// Hold a claim transaction after its revision read, then commit an authority
// edit on another connection. This exercises the commit-time half of CAS;
// sequential stale-token CLI tests only exercise its read-time half.
func TestRevisionAuthorityEditDefeatsInterleavedClaim(t *testing.T) {
	for _, authority := range []string{"label", "dependency"} {
		t.Run(authority, func(t *testing.T) {
			store, cleanup := setupConcurrentTestStore(t)
			defer cleanup()
			ctx, cancel := testContext(t)
			defer cancel()
			createPerm(t, ctx, store, "race-source")
			createPerm(t, ctx, store, "race-target")
			selected, err := store.GetIssue(ctx, "race-source")
			if err != nil {
				t.Fatal(err)
			}
			claimPool, err := sql.Open("mysql", store.connStr)
			if err != nil {
				t.Fatal(err)
			}
			defer claimPool.Close()
			claim, err := claimPool.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer claim.Rollback()
			if err := issueops.CheckVersionInTx(ctx, claim, selected.ID, selected.RowVersion); err != nil {
				t.Fatal(err)
			}
			if _, err := issueops.ClaimIssueInTx(ctx, claim, selected.ID, "stale-worker"); err != nil {
				t.Fatal(err)
			}
			editPool, err := sql.Open("mysql", store.connStr)
			if err != nil {
				t.Fatal(err)
			}
			defer editPool.Close()
			edit, err := editPool.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer edit.Rollback()
			if authority == "label" {
				err = issueops.AddLabelInTx(ctx, edit, "", "", selected.ID, "hold:operator", "authority")
			} else {
				_, err = issueops.AddDependencyInTx(ctx, edit, &types.Dependency{IssueID: selected.ID, DependsOnID: "race-target", Type: types.DepBlocks}, "authority", issueops.AddDependencyOpts{})
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := edit.Commit(); err != nil {
				t.Fatalf("authority commit: %v", err)
			}
			err = claim.Commit()
			if err == nil {
				t.Fatal("stale claim committed after authority edit won")
			}
			var mysqlErr *mysql.MySQLError
			if !errors.As(err, &mysqlErr) || (mysqlErr.Number != 1213 && mysqlErr.Number != 1205) {
				t.Fatalf("expected transaction conflict, got %v", err)
			}
			got, err := store.GetIssue(ctx, selected.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.RowVersion == selected.RowVersion || got.Assignee != "" || got.Status != types.StatusOpen {
				t.Fatalf("winner or claim isolation lost: %#v", got)
			}
			var leases int
			if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM leases WHERE issue_id = ?", selected.ID).Scan(&leases); err != nil {
				t.Fatal(err)
			}
			if leases != 0 {
				t.Fatalf("refused claim leaked %d leases", leases)
			}
		})
	}
}

func TestRevisionSDKWispDeletionInvalidatesSources(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		store, cleanup := setupConcurrentTestStore(t)
		ctx, cancel := testContext(t)
		createPerm(t, ctx, store, "sdk-source")
		createWisp(t, ctx, store, "sdk-target")
		addDependency(t, ctx, store, "sdk-source", "sdk-target", types.DepRelatesTo)
		before, err := store.GetIssue(ctx, "sdk-source")
		if err != nil {
			t.Fatal(err)
		}
		if bulk {
			_, err = store.DeleteIssues(ctx, []string{"sdk-target"}, false, true, false)
		} else {
			err = store.DeleteIssue(ctx, "sdk-target")
		}
		if err != nil {
			t.Fatal(err)
		}
		after, err := store.GetIssue(ctx, "sdk-source")
		if err != nil {
			t.Fatal(err)
		}
		if after.RowVersion == before.RowVersion {
			t.Errorf("SDK wisp delete kept old source revision (bulk=%v)", bulk)
		}
		// This SDK primitive leaves version commits to its caller. Verify that
		// the normal caller-owned commit preserves the changed token in HEAD.
		if err := store.Commit(ctx, "SDK deletion caller commit"); err != nil {
			t.Fatal(err)
		}
		var committed int64
		if err := store.db.QueryRowContext(ctx, "SELECT row_lock FROM issues AS OF 'HEAD' WHERE id = ?", "sdk-source").Scan(&committed); err != nil {
			t.Fatal(err)
		}
		if committed != after.RowVersion {
			t.Fatal("SDK caller commit omitted source revision")
		}
		cancel()
		cleanup()
	}
}

func TestRevisionTransactionWispDeleteAtomicOrRefused(t *testing.T) {
	for _, journal := range []bool{false, true} {
		store, cleanup := setupConcurrentTestStore(t)
		ctx, cancel := testContext(t)
		store.SetEventsJournalEnabled(journal)
		createPerm(t, ctx, store, "split-source")
		createWisp(t, ctx, store, "split-target")
		addDependency(t, ctx, store, "split-source", "split-target", types.DepRelatesTo)
		if err := store.Commit(ctx, "seed split transaction"); err != nil {
			t.Fatal(err)
		}
		before, err := store.GetIssue(ctx, "split-source")
		if err != nil {
			t.Fatal(err)
		}
		err = store.RunInTransaction(ctx, "delete target", func(tx storage.Transaction) error { return tx.DeleteIssue(ctx, "split-target") })
		if !journal {
			var unsupported *storage.ErrUnsupported
			if !errors.As(err, &unsupported) {
				t.Fatalf("split transaction must refuse before writes; got %v", err)
			}
			after, err := store.GetIssue(ctx, "split-source")
			if err != nil {
				t.Fatal(err)
			}
			if after.RowVersion != before.RowVersion {
				t.Fatal("refusal changed source revision")
			}
			if _, err := store.GetIssue(ctx, "split-target"); err != nil {
				t.Fatalf("refusal removed target: %v", err)
			}
			deps, err := store.GetDependencyRecords(ctx, "split-source")
			if err != nil || len(deps) != 1 {
				t.Fatalf("refusal changed edge: %v %v", deps, err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			after, err := store.GetIssue(ctx, "split-source")
			if err != nil {
				t.Fatal(err)
			}
			var committed int64
			if err := store.db.QueryRowContext(ctx, "SELECT row_lock FROM issues AS OF 'HEAD' WHERE id = ?", "split-source").Scan(&committed); err != nil {
				t.Fatal(err)
			}
			if after.RowVersion == before.RowVersion || committed != after.RowVersion {
				t.Fatal("pinned transaction did not commit changed source revision")
			}
		}
		cancel()
		cleanup()
	}
}

func TestRevisionSourceRepoDeletionInvalidatesSources(t *testing.T) {
	store, cleanup := setupConcurrentTestStore(t)
	defer cleanup()
	ctx, cancel := testContext(t)
	defer cancel()
	target := &types.Issue{ID: "repo-target", Title: "Deleted repository target", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask, SourceRepo: "removed-repo"}
	if err := store.CreateIssue(ctx, target, "tester"); err != nil {
		t.Fatal(err)
	}
	createPerm(t, ctx, store, "repo-source")
	createWisp(t, ctx, store, "repo-wisp")
	versions := map[string]int64{}
	for _, id := range []string{"repo-source", "repo-wisp"} {
		addDependency(t, ctx, store, id, target.ID, types.DepRelatesTo)
		issue, err := store.GetIssue(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		versions[id] = issue.RowVersion
	}
	if _, err := store.DeleteIssuesBySourceRepo(ctx, "removed-repo"); err != nil {
		t.Fatal(err)
	}
	for id, revision := range versions {
		issue, err := store.GetIssue(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if issue.RowVersion == revision {
			t.Errorf("source-repo deletion retained selection of %s", id)
		}
	}
}

func TestRevisionSplitDeleteDurableTargetRefusesBeforeWrites(t *testing.T) {
	store, cleanup := setupConcurrentTestStore(t)
	defer cleanup()
	ctx, cancel := testContext(t)
	defer cancel()
	store.SetEventsJournalEnabled(false)
	createPerm(t, ctx, store, "opposite-target")
	createWisp(t, ctx, store, "opposite-source")
	addDependency(t, ctx, store, "opposite-source", "opposite-target", types.DepRelatesTo)
	before, err := store.GetIssue(ctx, "opposite-source")
	if err != nil {
		t.Fatal(err)
	}
	err = store.RunInTransaction(ctx, "delete with cross-plane source edit", func(tx storage.Transaction) error {
		if err := tx.DeleteIssue(ctx, "opposite-target"); err != nil {
			return err
		}
		return tx.UpdateIssue(ctx, "opposite-source", map[string]interface{}{"title": "changed"}, "tester")
	})
	var unsupported *storage.ErrUnsupported
	if !errors.As(err, &unsupported) {
		t.Fatalf("cross-plane split deletion must refuse, got %v", err)
	}
	after, err := store.GetIssue(ctx, "opposite-source")
	if err != nil {
		t.Fatal(err)
	}
	if after.RowVersion != before.RowVersion || after.Title != before.Title {
		t.Fatal("refused callback changed wisp source")
	}
	if _, err := store.GetIssue(ctx, "opposite-target"); err != nil {
		t.Fatalf("refusal deleted target: %v", err)
	}
}
