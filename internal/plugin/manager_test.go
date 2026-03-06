package plugin

import (
	"testing"

	"github.com/zhaoxinyi02/ClawPanel/internal/config"
)

func TestResolvePluginInstallStrategyPrefersRegistryGitOverNpm(t *testing.T) {
	t.Parallel()

	strategy := resolvePluginInstallStrategy(&RegistryPlugin{
		GitURL:     "https://github.com/example/repo.git",
		NpmPackage: "@openclaw/wecom",
	}, "")

	if strategy.kind != "download" || strategy.target != "https://github.com/example/repo.git" {
		t.Fatalf("expected git/download strategy, got %#v", strategy)
	}
}

func TestResolvePluginInstallStrategyUsesExplicitNpmSource(t *testing.T) {
	t.Parallel()

	strategy := resolvePluginInstallStrategy(&RegistryPlugin{
		GitURL:     "https://github.com/example/repo.git",
		NpmPackage: "@openclaw/wecom",
	}, "@openclaw/custom")

	if strategy.kind != "npm" || strategy.target != "@openclaw/custom" {
		t.Fatalf("expected explicit npm strategy, got %#v", strategy)
	}
}

func TestNormalizeOpenClawInstallSource(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"npm":      "npm",
		"archive":  "archive",
		"path":     "path",
		"local":    "path",
		"registry": "path",
		"custom":   "path",
		"github":   "path",
		"git":      "path",
		"":         "path",
	}

	for input, want := range tests {
		if got := normalizeOpenClawInstallSource(input); got != want {
			t.Fatalf("normalizeOpenClawInstallSource(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSyncOpenClawPluginStateWritesEntriesAndInstalls(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	m := &Manager{cfg: cfg}

	if err := m.syncOpenClawPluginState("dingtalk", dir+"/extensions/dingtalk", true, "registry", "0.2.0"); err != nil {
		t.Fatalf("syncOpenClawPluginState: %v", err)
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("ReadOpenClawJSON: %v", err)
	}
	pl, _ := saved["plugins"].(map[string]interface{})
	ent, _ := pl["entries"].(map[string]interface{})
	ins, _ := pl["installs"].(map[string]interface{})
	entry, _ := ent["dingtalk"].(map[string]interface{})
	install, _ := ins["dingtalk"].(map[string]interface{})
	if enabled, _ := entry["enabled"].(bool); !enabled {
		t.Fatalf("expected dingtalk entry enabled, got %#v", entry)
	}
	if got, _ := install["installPath"].(string); got == "" {
		t.Fatalf("expected installPath, got %#v", install)
	}
	if got, _ := install["version"].(string); got != "0.2.0" {
		t.Fatalf("expected version 0.2.0, got %#v", install)
	}
}

func TestRemoveOpenClawPluginStateDeletesEntriesAndInstalls(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	m := &Manager{cfg: cfg}
	if err := m.syncOpenClawPluginState("wecom", dir+"/extensions/wecom", true, "registry", "latest"); err != nil {
		t.Fatalf("seed syncOpenClawPluginState: %v", err)
	}
	if err := m.removeOpenClawPluginState("wecom"); err != nil {
		t.Fatalf("removeOpenClawPluginState: %v", err)
	}
	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("ReadOpenClawJSON: %v", err)
	}
	pl, _ := saved["plugins"].(map[string]interface{})
	ent, _ := pl["entries"].(map[string]interface{})
	ins, _ := pl["installs"].(map[string]interface{})
	if _, ok := ent["wecom"]; ok {
		t.Fatalf("expected wecom entry removed")
	}
	if _, ok := ins["wecom"]; ok {
		t.Fatalf("expected wecom install removed")
	}
}
