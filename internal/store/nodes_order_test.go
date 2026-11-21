package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/k9032431-cmd/cloudrix/internal/model"
)

// Nodes are listed in the order set by ReorderNodes; new ones go last.
func TestReorderNodes(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	var ids []int64
	for _, name := range []string{"a", "b", "c"} {
		n := &model.Node{Name: name, Address: name + ".example.com", UsageCoefficient: 1, Status: model.NodeConnecting}
		if err := s.CreateNode(ctx, n); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID)
	}
	names := func() string {
		list, err := s.ListNodes(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := ""
		for _, n := range list {
			out += n.Name
		}
		return out
	}
	if got := names(); got != "abc" {
		t.Fatalf("initial order %q", got)
	}
	if err := s.ReorderNodes(ctx, []int64{ids[2], ids[0]}); err != nil {
		t.Fatal(err)
	}
	if got := names(); got != "cab" {
		t.Fatalf("after reorder %q", got)
	}
	n := &model.Node{Name: "d", Address: "d.example.com", UsageCoefficient: 1, Status: model.NodeConnecting}
	if err := s.CreateNode(ctx, n); err != nil {
		t.Fatal(err)
	}
	if got := names(); got != "cabd" {
		t.Fatalf("new node not last: %q", got)
	}
}
