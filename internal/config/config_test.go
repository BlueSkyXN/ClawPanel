package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadOpenClawJSONSupportsJSON5AndWriteCreatesBackup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &Config{OpenClawDir: dir}

	json5Raw := `{
  // json5 comment
  tools: {
    agentToAgent: true,
  },
  session: {
    maxMessages: 30,
  },
  agents: {
    default: "main",
    list: [
      { id: "main", },
    ],
  },
}`
	configPath := filepath.Join(dir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(json5Raw), 0644); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	parsed, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("ReadOpenClawJSON should parse JSON5, got error: %v", err)
	}
	if _, ok := parsed["tools"].(map[string]interface{}); !ok {
		t.Fatalf("tools should exist after JSON5 parse")
	}
	if _, ok := parsed["session"].(map[string]interface{}); !ok {
		t.Fatalf("session should exist after JSON5 parse")
	}

	if err := cfg.WriteOpenClawJSON(parsed); err != nil {
		t.Fatalf("WriteOpenClawJSON failed: %v", err)
	}

	backupDir := filepath.Join(dir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("backup dir should exist: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "pre-edit-") && strings.HasSuffix(e.Name(), ".json") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pre-edit backup file to be created")
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	var written map[string]interface{}
	if err := json.Unmarshal(raw, &written); err != nil {
		t.Fatalf("written config should be standard JSON: %v", err)
	}
	if _, ok := written["tools"]; !ok {
		t.Fatalf("written config should preserve tools")
	}
	if _, ok := written["session"]; !ok {
		t.Fatalf("written config should preserve session")
	}
}

func TestWriteOpenClawJSONNormalizesLegacyAgentModelFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &Config{OpenClawDir: dir}

	input := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": map[string]interface{}{
					"primary":       "cpa/gemini-3.1-pro-preview",
					"contextTokens": 200000,
					"maxTokens":     8192,
				},
			},
		},
	}

	if err := cfg.WriteOpenClawJSON(input); err != nil {
		t.Fatalf("WriteOpenClawJSON failed: %v", err)
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("ReadOpenClawJSON failed: %v", err)
	}

	agents, _ := saved["agents"].(map[string]interface{})
	defaults, _ := agents["defaults"].(map[string]interface{})
	if defaults == nil {
		t.Fatalf("agents.defaults should exist")
	}
	if _, ok := defaults["contextTokens"]; !ok {
		t.Fatalf("legacy contextTokens should be migrated to agents.defaults.contextTokens")
	}

	model, _ := defaults["model"].(map[string]interface{})
	if model == nil {
		t.Fatalf("agents.defaults.model should still exist")
	}
	if _, ok := model["contextTokens"]; ok {
		t.Fatalf("agents.defaults.model.contextTokens should be removed")
	}
	if _, ok := model["maxTokens"]; ok {
		t.Fatalf("agents.defaults.model.maxTokens should be removed")
	}
}

func TestWriteOpenClawJSONNormalizesLegacyPanelFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &Config{OpenClawDir: dir}

	input := map[string]interface{}{
		"gateway": map[string]interface{}{
			"mode":        "hosted",
			"bindAddress": "0.0.0.0",
		},
		"hooks": map[string]interface{}{
			"enabled":  true,
			"basePath": "/hooks",
			"secret":   "test-token",
		},
		"messages": map[string]interface{}{
			"systemPrompt":       "legacy",
			"maxHistoryMessages": 50,
			"ackReactionScope":   "group-mentions",
		},
	}

	if err := cfg.WriteOpenClawJSON(input); err != nil {
		t.Fatalf("WriteOpenClawJSON failed: %v", err)
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("ReadOpenClawJSON failed: %v", err)
	}

	gateway, _ := saved["gateway"].(map[string]interface{})
	if gateway == nil {
		t.Fatalf("gateway should exist")
	}
	if got, _ := gateway["mode"].(string); got != "remote" {
		t.Fatalf("gateway.mode should normalize to remote, got %q", got)
	}
	if got, _ := gateway["customBindHost"].(string); got != "0.0.0.0" {
		t.Fatalf("gateway.customBindHost should be migrated, got %q", got)
	}
	if _, ok := gateway["bindAddress"]; ok {
		t.Fatalf("gateway.bindAddress should be removed")
	}

	hooks, _ := saved["hooks"].(map[string]interface{})
	if hooks == nil {
		t.Fatalf("hooks should exist")
	}
	if got, _ := hooks["path"].(string); got != "/hooks" {
		t.Fatalf("hooks.path should be migrated, got %q", got)
	}
	if got, _ := hooks["token"].(string); got != "test-token" {
		t.Fatalf("hooks.token should be migrated, got %q", got)
	}
	if _, ok := hooks["basePath"]; ok {
		t.Fatalf("hooks.basePath should be removed")
	}
	if _, ok := hooks["secret"]; ok {
		t.Fatalf("hooks.secret should be removed")
	}

	messages, _ := saved["messages"].(map[string]interface{})
	if messages == nil {
		t.Fatalf("messages should exist")
	}
	if _, ok := messages["systemPrompt"]; ok {
		t.Fatalf("messages.systemPrompt should be removed")
	}
	if _, ok := messages["maxHistoryMessages"]; ok {
		t.Fatalf("messages.maxHistoryMessages should be removed")
	}
}

func TestReadQQChannelStateSupportsJSON5(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &Config{OpenClawDir: dir}

	raw := `{
  channels: {
    qq: {
      enabled: true,
      accessToken: "qq-token",
    },
  },
}`
	if err := os.WriteFile(filepath.Join(dir, "openclaw.json"), []byte(raw), 0644); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	enabled, token, err := cfg.ReadQQChannelState()
	if err != nil {
		t.Fatalf("ReadQQChannelState failed: %v", err)
	}
	if !enabled {
		t.Fatalf("expected qq channel to be enabled")
	}
	if token != "qq-token" {
		t.Fatalf("expected qq access token to be preserved, got %q", token)
	}
}
