//go:build cgo

package embeddeddolt_test

import (
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
	"testing"
)

func TestTransactionAuthorityRevisionStaging(t *testing.T) {
	skipUnlessEmbeddedDolt(t)
	for _, kind := range []string{"label_noop", "dependency_noop", "label_remove_noop", "dependency_remove_noop", "wisp_delete"} {
		t.Run(kind, func(t *testing.T) {
			te := newTestEnv(t, "trs")
			ctx := t.Context()
			for _, id := range []string{"trs-source", "trs-target", "trs-unrelated"} {
				issue := &types.Issue{ID: id, Title: id, Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}
				if kind == "wisp_delete" && id == "trs-target" {
					issue.Ephemeral = true
				}
				if err := te.store.CreateIssue(ctx, issue, "tester"); err != nil {
					t.Fatal(err)
				}
			}
			dep := &types.Dependency{IssueID: "trs-source", DependsOnID: "trs-target", Type: types.DepRelatesTo}
			if err := te.store.AddDependency(ctx, dep, "tester"); err != nil {
				t.Fatal(err)
			}
			if err := te.store.AddLabel(ctx, "trs-source", "hold:operator", "tester"); err != nil {
				t.Fatal(err)
			}
			if err := te.store.Commit(ctx, "seed authority"); err != nil {
				t.Fatal(err)
			}
			var before int64
			te.queryScalar(t, ctx, "SELECT row_lock FROM issues AS OF 'HEAD' WHERE id = ?", []any{"trs-source"}, &before)
			if kind != "wisp_delete" {
				if err := te.store.UpdateIssue(ctx, "trs-unrelated", map[string]interface{}{"title": "pending unrelated edit"}, "tester"); err != nil {
					t.Fatal(err)
				}
			}
			err := te.store.RunInTransaction(ctx, "authority operation", func(tx storage.Transaction) error {
				switch kind {
				case "label_remove_noop":
					return tx.RemoveLabel(ctx, "trs-source", "absent", "tester")
				case "dependency_remove_noop":
					return tx.RemoveDependency(ctx, "trs-source", "trs-unrelated", "tester")
				case "label_noop":
					return tx.AddLabel(ctx, "trs-source", "hold:operator", "tester")
				case "dependency_noop":
					return tx.AddDependency(ctx, dep, "tester")
				default:
					return tx.DeleteIssue(ctx, "trs-target")
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			var working, committed int64
			te.queryScalar(t, ctx, "SELECT row_lock FROM issues WHERE id = ?", []any{"trs-source"}, &working)
			te.queryScalar(t, ctx, "SELECT row_lock FROM issues AS OF 'HEAD' WHERE id = ?", []any{"trs-source"}, &committed)
			if working != committed {
				t.Errorf("revision left unstaged: working=%d HEAD=%d", working, committed)
			}
			if kind == "wisp_delete" {
				if working == before {
					t.Fatal("deletion left old selection valid")
				}
			} else {
				if working != before {
					t.Fatal("no-op reminted source")
				}
				var title string
				te.queryScalar(t, ctx, "SELECT title FROM issues AS OF 'HEAD' WHERE id = ?", []any{"trs-unrelated"}, &title)
				if title != "trs-unrelated" {
					t.Fatal("no-op committed unrelated pending issue edit")
				}
			}
		})
	}
}
