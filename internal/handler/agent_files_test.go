package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zhaoxinyi02/ClawPanel/internal/config"
)

func TestResolveAgentWorkspacePathSupportsWorkspaceVariants(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	cfg := &config.Config{
		OpenClawDir:  filepath.Join(root, ".openclaw"),
		OpenClawWork: filepath.Join(root, ".openclaw"),
	}
	if err := cfg.WriteOpenClawJSON(map[string]interface{}{"agents": []interface{}{}}); err != nil {
		t.Fatalf("WriteOpenClawJSON failed: %v", err)
	}

	workspace := filepath.Join(root, ".openclaw", "workspace", "main")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	got := resolveAgentWorkspacePath(cfg, "main")
	if got != workspace {
		t.Fatalf("workspace=%q, want %q", got, workspace)
	}
}

func TestManagedAgentWorkspaceRootsIncludesRelativeWorkspaceParents(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	cfg := &config.Config{
		OpenClawDir:  filepath.Join(root, ".openclaw"),
		OpenClawWork: filepath.Join(root, ".openclaw"),
	}
	if err := cfg.WriteOpenClawJSON(map[string]interface{}{
		"agents": []interface{}{
			map[string]interface{}{"id": "main", "workspace": "workspace/main"},
		},
	}); err != nil {
		t.Fatalf("WriteOpenClawJSON failed: %v", err)
	}

	roots := managedAgentWorkspaceRoots(cfg)
	want := filepath.Join(root, "workspace")
	for _, root := range roots {
		if filepath.Clean(root) == filepath.Clean(want) {
			return
		}
	}
	t.Fatalf("managed roots %v do not contain %q", roots, want)
}

func TestResolveAgentCoreWorkspaceFromExplicitPath(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	cfg := &config.Config{
		OpenClawDir:  filepath.Join(root, ".openclaw"),
		OpenClawWork: filepath.Join(root, ".openclaw"),
	}
	workspace := filepath.Join(root, ".openclaw", "workspace", "draft")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	loc, err := resolveAgentCoreWorkspaceFromPath(cfg, workspace, false)
	if err != nil {
		t.Fatalf("resolveAgentCoreWorkspaceFromPath failed: %v", err)
	}
	if filepath.Clean(loc.Display) != filepath.Clean(workspace) {
		t.Fatalf("display=%q, want %q", loc.Display, workspace)
	}
	if filepath.Clean(loc.Safe) != filepath.Clean(workspace) {
		t.Fatalf("safe=%q, want %q", loc.Safe, workspace)
	}
}
