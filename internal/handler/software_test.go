package handler

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhaoxinyi02/ClawPanel/internal/config"
)

func TestBuildNapCatInstallScriptUsesConfiguredQQToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	openClawDir := filepath.Join(dir, "custom-openclaw")
	if err := os.MkdirAll(openClawDir, 0755); err != nil {
		t.Fatalf("mkdir openclaw dir: %v", err)
	}

	raw := `{
  // json5 comment
  channels: {
    qq: {
      enabled: true,
      accessToken: "qq-token-$123",
    },
  },
}`
	if err := os.WriteFile(filepath.Join(openClawDir, "openclaw.json"), []byte(raw), 0644); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	cfg := &config.Config{
		OpenClawDir:  openClawDir,
		OpenClawWork: filepath.Join(dir, "work"),
	}

	script := buildNapCatInstallScript(cfg)
	if strings.Contains(script, `${HOME}/.openclaw`) {
		t.Fatalf("script should not hardcode the default openclaw dir")
	}
	if !strings.Contains(script, openClawDir) {
		t.Fatalf("script should mount the configured openclaw dir")
	}

	expectedB64 := base64.StdEncoding.EncodeToString([]byte("qq-token-$123"))
	if !strings.Contains(script, expectedB64) {
		t.Fatalf("script should embed the configured qq access token via base64")
	}
	if !strings.Contains(script, "export WS_TOKEN_B64=") {
		t.Fatalf("script should export WS_TOKEN_B64 for the python subprocess")
	}
	if !strings.Contains(script, "WS_TOKEN_JSON=$(python3 - <<'PY'") {
		t.Fatalf("script should JSON-encode the token before writing onebot11.json")
	}
	if !strings.Contains(script, `\"token\": ${WS_TOKEN_JSON}`) {
		t.Fatalf("script should inject the JSON-encoded token into onebot11.json")
	}
}
