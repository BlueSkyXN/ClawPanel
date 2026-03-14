package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zhaoxinyi02/ClawPanel/internal/config"
)

func TestGetSessionsAgentAllAggregates(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}

	oc := map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), oc)

	mainSessionFile := filepath.Join(dir, "agents", "main", "sessions", "s_main.jsonl")
	workSessionFile := filepath.Join(dir, "agents", "work", "sessions", "s_work.jsonl")
	if err := os.MkdirAll(filepath.Dir(mainSessionFile), 0755); err != nil {
		t.Fatalf("mkdir main sessions dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(workSessionFile), 0755); err != nil {
		t.Fatalf("mkdir work sessions dir: %v", err)
	}
	_ = os.WriteFile(mainSessionFile, []byte(`{"type":"message","id":"1","message":{"role":"user","content":"hello"}}`+"\n"), 0644)
	_ = os.WriteFile(workSessionFile, []byte(`{"type":"assistant","id":"2","message":{"role":"assistant","content":"world"}}`+"\n"), 0644)

	writeJSON(t, filepath.Join(dir, "agents", "main", "sessions", "sessions.json"), map[string]interface{}{
		"main-key": map[string]interface{}{
			"sessionId":   "session-main",
			"chatType":    "direct",
			"updatedAt":   float64(1000),
			"sessionFile": mainSessionFile,
		},
	})
	writeJSON(t, filepath.Join(dir, "agents", "work", "sessions", "sessions.json"), map[string]interface{}{
		"work-key": map[string]interface{}{
			"sessionId":   "session-work",
			"chatType":    "group",
			"updatedAt":   float64(2000),
			"sessionFile": workSessionFile,
		},
	})

	r := gin.New()
	r.GET("/sessions", GetSessions(cfg))
	req := httptest.NewRequest(http.MethodGet, "/sessions?agent=all", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK       bool          `json:"ok"`
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(resp.Sessions))
	}
	if resp.Sessions[0].AgentID != "work" || resp.Sessions[0].SessionID != "session-work" {
		t.Fatalf("sessions should be aggregated and sorted by updatedAt desc")
	}
}

func TestGetSessionsAgentAllIgnoresOrphanAgentDirsWhenExplicitListExists(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
			},
		},
	})

	mainSessionFile := filepath.Join(dir, "agents", "main", "sessions", "main.jsonl")
	if err := os.MkdirAll(filepath.Dir(mainSessionFile), 0755); err != nil {
		t.Fatalf("mkdir main sessions dir: %v", err)
	}
	_ = os.WriteFile(mainSessionFile, []byte(`{"type":"message","id":"1","message":{"role":"user","content":"hello"}}`+"\n"), 0644)
	writeJSON(t, filepath.Join(dir, "agents", "main", "sessions", "sessions.json"), map[string]interface{}{
		"main-key": map[string]interface{}{
			"sessionId":   "session-main",
			"updatedAt":   float64(1000),
			"sessionFile": mainSessionFile,
		},
	})

	orphanSessionFile := filepath.Join(dir, "agents", "work", "sessions", "work.jsonl")
	if err := os.MkdirAll(filepath.Dir(orphanSessionFile), 0755); err != nil {
		t.Fatalf("mkdir orphan sessions dir: %v", err)
	}
	_ = os.WriteFile(orphanSessionFile, []byte(`{"type":"assistant","id":"2","message":{"role":"assistant","content":"ghost"}}`+"\n"), 0644)
	writeJSON(t, filepath.Join(dir, "agents", "work", "sessions", "sessions.json"), map[string]interface{}{
		"work-key": map[string]interface{}{
			"sessionId":   "session-work",
			"updatedAt":   float64(2000),
			"sessionFile": orphanSessionFile,
		},
	})

	r := gin.New()
	r.GET("/sessions", GetSessions(cfg))
	req := httptest.NewRequest(http.MethodGet, "/sessions?agent=all", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK       bool          `json:"ok"`
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].AgentID != "main" {
		t.Fatalf("expected only explicit main sessions, got %+v", resp.Sessions)
	}
}

func TestGetOpenClawAgentsMarksImplicitMainPlaceholder(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
		},
	})

	r := gin.New()
	r.GET("/openclaw/agents", GetOpenClawAgents(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK     bool `json:"ok"`
		Agents struct {
			List []map[string]interface{} `json:"list"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if len(resp.Agents.List) != 1 {
		t.Fatalf("expected one synthesized agent, got %d", len(resp.Agents.List))
	}
	if got := resp.Agents.List[0]["id"]; got != "main" {
		t.Fatalf("expected synthesized main, got %#v", got)
	}
	if got := resp.Agents.List[0]["implicit"]; got != true {
		t.Fatalf("expected synthesized main to be implicit=true, got %#v", got)
	}
}

func TestGetOpenClawAgentsKeepsExplicitMainNonImplicit(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
			},
		},
	})

	r := gin.New()
	r.GET("/openclaw/agents", GetOpenClawAgents(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK     bool `json:"ok"`
		Agents struct {
			List []map[string]interface{} `json:"list"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if len(resp.Agents.List) != 1 {
		t.Fatalf("expected one explicit agent, got %d", len(resp.Agents.List))
	}
	if got := resp.Agents.List[0]["id"]; got != "main" {
		t.Fatalf("expected explicit main, got %#v", got)
	}
	if got := resp.Agents.List[0]["implicit"]; got != false {
		t.Fatalf("expected explicit main to be implicit=false, got %#v", got)
	}
}

func newAgentCoreTestEnv(t *testing.T) (string, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	if realDir, err := filepath.EvalSymlinks(dir); err == nil && realDir != "" {
		dir = realDir
	}
	openClawDir := filepath.Join(dir, ".openclaw")
	if err := os.MkdirAll(openClawDir, 0755); err != nil {
		t.Fatalf("mkdir openclaw dir: %v", err)
	}
	return dir, &config.Config{OpenClawDir: openClawDir}
}

func writeAgentCoreOpenClawJSON(t *testing.T, cfg *config.Config, payload map[string]interface{}) {
	t.Helper()
	writeJSON(t, filepath.Join(cfg.OpenClawDir, "openclaw.json"), payload)
}

func expectAgentCoreIOStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) bool {
	t.Helper()
	if runtime.GOOS == "windows" {
		if w.Code != http.StatusNotImplemented {
			t.Fatalf("expected 501 on windows, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode windows unsupported response: %v", err)
		}
		if resp.Error != errAgentCoreFileUnsupportedPlatform.Error() {
			t.Fatalf("expected unsupported-platform message, got %+v", resp)
		}
		return false
	}
	if w.Code != expected {
		t.Fatalf("expected %d, got %d: %s", expected, w.Code, w.Body.String())
	}
	return true
}

func TestGetOpenClawAgentCoreFilesReadsWorkspaceDocs(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("# Agent instructions"), 0644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	r := gin.New()
	r.GET("/openclaw/agents/:id/core-files", GetOpenClawAgentCoreFiles(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents/main/core-files", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !expectAgentCoreIOStatus(t, w, http.StatusOK) {
		return
	}

	var resp struct {
		OK        bool   `json:"ok"`
		Workspace string `json:"workspace"`
		Files     []struct {
			Name    string `json:"name"`
			Exists  bool   `json:"exists"`
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if resp.Workspace != workspace {
		t.Fatalf("expected workspace %q, got %q", workspace, resp.Workspace)
	}
	if len(resp.Files) != len(agentCoreFileNames) {
		t.Fatalf("expected %d core files, got %d", len(agentCoreFileNames), len(resp.Files))
	}

	foundAgents := false
	foundMemory := false
	for _, file := range resp.Files {
		switch file.Name {
		case "AGENTS.md":
			foundAgents = true
			if !file.Exists || !strings.Contains(file.Content, "Agent instructions") {
				t.Fatalf("expected AGENTS.md content, got %+v", file)
			}
		case "MEMORY.md":
			foundMemory = true
			if file.Exists {
				t.Fatalf("expected missing MEMORY.md to be reported as absent")
			}
		}
	}
	if !foundAgents || !foundMemory {
		t.Fatalf("expected AGENTS.md and MEMORY.md entries, got %+v", resp.Files)
	}
}

func TestGetOpenClawAgentCoreFilesTruncatesOversizedContent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	oversized := strings.Repeat("a", int(agentCoreFileMaxBytes)+64)
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte(oversized), 0644); err != nil {
		t.Fatalf("write oversized AGENTS.md: %v", err)
	}

	r := gin.New()
	r.GET("/openclaw/agents/:id/core-files", GetOpenClawAgentCoreFiles(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents/main/core-files", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !expectAgentCoreIOStatus(t, w, http.StatusOK) {
		return
	}

	var resp struct {
		Files []struct {
			Name    string `json:"name"`
			Exists  bool   `json:"exists"`
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, file := range resp.Files {
		if file.Name != "AGENTS.md" {
			continue
		}
		if !file.Exists {
			t.Fatalf("expected oversized AGENTS.md to exist")
		}
		if !strings.Contains(file.Content, "文件过大，已截断") {
			t.Fatalf("expected truncation marker, got %q", file.Content)
		}
		if len(file.Content) <= 0 || int64(len(file.Content)) > agentCoreFileMaxBytes+128 {
			t.Fatalf("expected bounded truncated content length, got %d", len(file.Content))
		}
		return
	}
	t.Fatalf("expected AGENTS.md entry in response")
}

func TestSaveOpenClawAgentCoreFileCreatesWorkspaceDoc(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/agents/:id/core-files", SaveOpenClawAgentCoreFile(cfg))
	body := []byte(`{"name":"MEMORY.md","content":"# Memory\n\nSaved from test."}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/main/core-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !expectAgentCoreIOStatus(t, w, http.StatusOK) {
		return
	}

	saved, err := os.ReadFile(filepath.Join(workspace, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if !strings.Contains(string(saved), "Saved from test") {
		t.Fatalf("expected saved content, got %q", string(saved))
	}
}

func TestSaveOpenClawAgentCoreFileReplacesExistingDoc(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "MEMORY.md"), []byte("old content"), 0644); err != nil {
		t.Fatalf("seed MEMORY.md: %v", err)
	}

	r := gin.New()
	r.PUT("/openclaw/agents/:id/core-files", SaveOpenClawAgentCoreFile(cfg))
	body := []byte(`{"name":"MEMORY.md","content":"new content"}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/main/core-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !expectAgentCoreIOStatus(t, w, http.StatusOK) {
		return
	}
	saved, err := os.ReadFile(filepath.Join(workspace, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read replaced file: %v", err)
	}
	if string(saved) != "new content" {
		t.Fatalf("expected overwrite, got %q", string(saved))
	}
}

func TestGetOpenClawAgentCoreFilesDoesNotFollowSymlink(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	sensitiveFile := filepath.Join(dir, "sensitive.txt")
	if err := os.WriteFile(sensitiveFile, []byte("top-secret"), 0644); err != nil {
		t.Fatalf("write sensitive file: %v", err)
	}
	if err := os.Symlink(sensitiveFile, filepath.Join(workspace, "AGENTS.md")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := gin.New()
	r.GET("/openclaw/agents/:id/core-files", GetOpenClawAgentCoreFiles(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents/main/core-files", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !expectAgentCoreIOStatus(t, w, http.StatusOK) {
		return
	}

	var resp struct {
		Files []struct {
			Name    string `json:"name"`
			Exists  bool   `json:"exists"`
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, file := range resp.Files {
		if file.Name == "AGENTS.md" {
			if file.Exists || file.Content != "" {
				t.Fatalf("expected symlinked AGENTS.md to be treated as absent, got %+v", file)
			}
			return
		}
	}
	t.Fatalf("expected AGENTS.md entry in response")
}

func TestSaveOpenClawAgentCoreFileRejectsSymlink(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	sensitiveFile := filepath.Join(dir, "sensitive.txt")
	if err := os.WriteFile(sensitiveFile, []byte("top-secret"), 0644); err != nil {
		t.Fatalf("write sensitive file: %v", err)
	}
	target := filepath.Join(workspace, "MEMORY.md")
	if err := os.Symlink(sensitiveFile, target); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := gin.New()
	r.PUT("/openclaw/agents/:id/core-files", SaveOpenClawAgentCoreFile(cfg))
	body := []byte(`{"name":"MEMORY.md","content":"overwrite attempt"}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/main/core-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !expectAgentCoreIOStatus(t, w, http.StatusForbidden) {
		return
	}
	saved, err := os.ReadFile(sensitiveFile)
	if err != nil {
		t.Fatalf("read sensitive file: %v", err)
	}
	if string(saved) != "top-secret" {
		t.Fatalf("expected sensitive file to remain unchanged, got %q", string(saved))
	}
}

func TestGetOpenClawAgentCoreFilesRejectsSymlinkedWorkspace(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	sensitiveDir := filepath.Join(dir, "sensitive")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(filepath.Dir(workspace), 0755); err != nil {
		t.Fatalf("mkdir workspaces root: %v", err)
	}
	if err := os.MkdirAll(sensitiveDir, 0755); err != nil {
		t.Fatalf("mkdir sensitive dir: %v", err)
	}
	if err := os.Symlink(sensitiveDir, workspace); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := gin.New()
	r.GET("/openclaw/agents/:id/core-files", GetOpenClawAgentCoreFiles(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents/main/core-files", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSaveOpenClawAgentCoreFileRejectsSymlinkedWorkspace(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	sensitiveDir := filepath.Join(dir, "sensitive")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(filepath.Dir(workspace), 0755); err != nil {
		t.Fatalf("mkdir workspaces root: %v", err)
	}
	if err := os.MkdirAll(sensitiveDir, 0755); err != nil {
		t.Fatalf("mkdir sensitive dir: %v", err)
	}
	if err := os.Symlink(sensitiveDir, workspace); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := gin.New()
	r.PUT("/openclaw/agents/:id/core-files", SaveOpenClawAgentCoreFile(cfg))
	body := []byte(`{"name":"MEMORY.md","content":"blocked"}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/main/core-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(sensitiveDir, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("expected sensitive dir to remain untouched, got err=%v", err)
	}
}

func TestSaveOpenClawAgentCoreFileRejectsSymlinkedWorkspaceAncestor(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "shared", "main")
	sensitiveDir := filepath.Join(dir, "sensitive")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(dir, "workspaces"), 0755); err != nil {
		t.Fatalf("mkdir workspaces root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sensitiveDir, "main"), 0755); err != nil {
		t.Fatalf("mkdir sensitive workspace: %v", err)
	}
	if err := os.Symlink(sensitiveDir, filepath.Join(dir, "workspaces", "shared")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := gin.New()
	r.PUT("/openclaw/agents/:id/core-files", SaveOpenClawAgentCoreFile(cfg))
	body := []byte(`{"name":"AGENTS.md","content":"blocked ancestor"}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/main/core-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(sensitiveDir, "main", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected symlink target to remain untouched, got err=%v", err)
	}
}

func TestGetOpenClawAgentCoreFilesRejectsSymlinkedWorkspaceAncestor(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "shared", "main")
	sensitiveDir := filepath.Join(dir, "sensitive")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(dir, "workspaces"), 0755); err != nil {
		t.Fatalf("mkdir workspaces root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sensitiveDir, "main"), 0755); err != nil {
		t.Fatalf("mkdir sensitive workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sensitiveDir, "main", "AGENTS.md"), []byte("should stay hidden"), 0644); err != nil {
		t.Fatalf("write sensitive agent file: %v", err)
	}
	if err := os.Symlink(sensitiveDir, filepath.Join(dir, "workspaces", "shared")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := gin.New()
	r.GET("/openclaw/agents/:id/core-files", GetOpenClawAgentCoreFiles(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents/main/core-files", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetOpenClawAgentCoreFilesAllowsConfiguredWorkspaceOutsideStandardRoots(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "outside", "main")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir outside workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("outside"), 0644); err != nil {
		t.Fatalf("write outside AGENTS.md: %v", err)
	}

	r := gin.New()
	r.GET("/openclaw/agents/:id/core-files", GetOpenClawAgentCoreFiles(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents/main/core-files", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !expectAgentCoreIOStatus(t, w, http.StatusOK) {
		return
	}
}

func TestSaveOpenClawAgentCoreFileAllowsConfiguredWorkspaceOutsideStandardRoots(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "outside", "main")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir outside workspace: %v", err)
	}

	r := gin.New()
	r.PUT("/openclaw/agents/:id/core-files", SaveOpenClawAgentCoreFile(cfg))
	body := []byte(`{"name":"MEMORY.md","content":"outside"}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/main/core-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !expectAgentCoreIOStatus(t, w, http.StatusOK) {
		return
	}
	if _, err := os.Stat(filepath.Join(workspace, "MEMORY.md")); os.IsNotExist(err) {
		t.Fatalf("expected MEMORY.md to be written in outside workspace")
	}
}

func TestSaveOpenClawAgentCoreFileRejectsOversizedContent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	r := gin.New()
	r.PUT("/openclaw/agents/:id/core-files", SaveOpenClawAgentCoreFile(cfg))
	oversized := strings.Repeat("a", int(agentCoreFileMaxBytes)+1)
	body := []byte(`{"name":"MEMORY.md","content":"` + oversized + `"}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/main/core-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("expected oversized save to be rejected, got err=%v", err)
	}
}

func TestGetOpenClawAgentCoreFilesRejectsSymlinkedManagedRoot(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	sensitiveRoot := filepath.Join(dir, "sensitive-root")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(sensitiveRoot, 0755); err != nil {
		t.Fatalf("mkdir sensitive root: %v", err)
	}
	if err := os.Symlink(sensitiveRoot, filepath.Join(dir, "workspaces")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := gin.New()
	r.GET("/openclaw/agents/:id/core-files", GetOpenClawAgentCoreFiles(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents/main/core-files", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSaveOpenClawAgentCoreFileRejectsSymlinkedManagedRoot(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir, cfg := newAgentCoreTestEnv(t)
	workspace := filepath.Join(dir, "workspaces", "main")
	sensitiveRoot := filepath.Join(dir, "sensitive-root")
	writeAgentCoreOpenClawJSON(t, cfg, map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(sensitiveRoot, 0755); err != nil {
		t.Fatalf("mkdir sensitive root: %v", err)
	}
	if err := os.Symlink(sensitiveRoot, filepath.Join(dir, "workspaces")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	r := gin.New()
	r.PUT("/openclaw/agents/:id/core-files", SaveOpenClawAgentCoreFile(cfg))
	body := []byte(`{"name":"AGENTS.md","content":"blocked root"}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/main/core-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(sensitiveRoot, "main", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected sensitive root to remain untouched, got err=%v", err)
	}
}

func TestLoadDefaultAgentIDPrefersListDefaultFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work", "default": true},
			},
		},
	})

	if got := loadDefaultAgentID(cfg); got != "work" {
		t.Fatalf("expected list default flag to win over legacy agents.default, got %q", got)
	}
}

func TestGetOpenClawAgentsKeepsLegacyDefaultForDiskOnlyAgents(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "work",
		},
	})
	if err := os.MkdirAll(filepath.Join(dir, "agents", "work"), 0755); err != nil {
		t.Fatalf("mkdir work agent dir: %v", err)
	}

	r := gin.New()
	r.GET("/openclaw/agents", GetOpenClawAgents(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK     bool `json:"ok"`
		Agents struct {
			Default           string                   `json:"default"`
			DefaultConfigured bool                     `json:"defaultConfigured"`
			List              []map[string]interface{} `json:"list"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if resp.Agents.Default != "work" {
		t.Fatalf("expected legacy default work to remain effective for disk-only agents, got %q", resp.Agents.Default)
	}
	if !resp.Agents.DefaultConfigured {
		t.Fatalf("expected legacy default to still be reported as configured")
	}
	if len(resp.Agents.List) != 1 || resp.Agents.List[0]["id"] != "work" {
		t.Fatalf("expected synthesized work agent, got %#v", resp.Agents.List)
	}
}

func TestGetOpenClawAgentsKeepsLegacyDefaultWithoutAgentDir(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "work",
		},
	})

	r := gin.New()
	r.GET("/openclaw/agents", GetOpenClawAgents(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK     bool `json:"ok"`
		Agents struct {
			Default           string                   `json:"default"`
			DefaultConfigured bool                     `json:"defaultConfigured"`
			List              []map[string]interface{} `json:"list"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if resp.Agents.Default != "work" {
		t.Fatalf("expected legacy default work to remain effective without agent dir, got %q", resp.Agents.Default)
	}
	if !resp.Agents.DefaultConfigured {
		t.Fatalf("expected legacy default to still be reported as configured")
	}
	if len(resp.Agents.List) != 1 || resp.Agents.List[0]["id"] != "work" {
		t.Fatalf("expected synthesized implicit work agent, got %#v", resp.Agents.List)
	}
}

func TestGetSessionsAllowsImplicitMainForDiskOnlyConfig(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
		},
	})
	if err := os.MkdirAll(filepath.Join(dir, "agents", "work", "sessions"), 0755); err != nil {
		t.Fatalf("mkdir work sessions dir: %v", err)
	}
	writeJSON(t, filepath.Join(dir, "agents", "work", "sessions", "sessions.json"), map[string]interface{}{})

	r := gin.New()
	r.GET("/sessions", GetSessions(cfg))
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected implicit main default to validate, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSessionsSupportsUpstreamAgentDirSubdirLayout(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "agentDir": "agents/main/agent"},
			},
		},
	})

	sessionFile := filepath.Join(dir, "agents", "main", "sessions", "session-main.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	if err := os.WriteFile(sessionFile, []byte(`{"type":"message","id":"1","message":{"role":"user","content":"hello"}}`+"\n"), 0644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	writeJSON(t, filepath.Join(dir, "agents", "main", "sessions", "sessions.json"), map[string]interface{}{
		"agent:main:main": map[string]interface{}{
			"sessionId":   "session-main",
			"chatType":    "direct",
			"updatedAt":   float64(1000),
			"sessionFile": sessionFile,
		},
	})

	r := gin.New()
	r.GET("/sessions", GetSessions(cfg))
	req := httptest.NewRequest(http.MethodGet, "/sessions?agent=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK       bool          `json:"ok"`
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d (%s)", len(resp.Sessions), w.Body.String())
	}
	if resp.Sessions[0].SessionID != "session-main" {
		t.Fatalf("expected session-main, got %+v", resp.Sessions[0])
	}
}

func TestGetOpenClawAgentsCountsSessionsWithUpstreamAgentDirSubdirLayout(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "agentDir": "agents/main/agent"},
			},
		},
	})

	if err := os.MkdirAll(filepath.Join(dir, "agents", "main", "sessions"), 0755); err != nil {
		t.Fatalf("mkdir sessions dir: %v", err)
	}
	writeJSON(t, filepath.Join(dir, "agents", "main", "sessions", "sessions.json"), map[string]interface{}{
		"agent:main:main": map[string]interface{}{
			"sessionId": "session-main",
			"updatedAt": float64(1234567890),
		},
	})

	r := gin.New()
	r.GET("/openclaw/agents", GetOpenClawAgents(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK     bool `json:"ok"`
		Agents struct {
			List []map[string]interface{} `json:"list"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if len(resp.Agents.List) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(resp.Agents.List))
	}
	if got := int(resp.Agents.List[0]["sessions"].(float64)); got != 1 {
		t.Fatalf("expected session count 1, got %+v", resp.Agents.List[0])
	}
}

func TestNormalizeAgentPathExpandsTilde(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(dir)
		if volume != "" {
			t.Setenv("HOMEDRIVE", volume)
			t.Setenv("HOMEPATH", strings.TrimPrefix(dir, volume))
		}
	}

	got := normalizeAgentPath("/ignored/base", "~/.openclaw/agents/main/agent")
	want := filepath.Join(dir, ".openclaw", "agents", "main", "agent")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveAgentRootDirKeepsBundleRootNamedAgent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{"id": "work", "agentDir": "custom/agent"},
			},
		},
	})

	got := resolveAgentRootDir(cfg, "work")
	want := filepath.Join(dir, "custom", "agent")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveAgentRootDirKeepsCustomBundleRootEndingWithAgent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{"id": "work", "agentDir": "custom/work/agent"},
			},
		},
	})

	got := resolveAgentRootDir(cfg, "work")
	want := filepath.Join(dir, "custom", "work", "agent")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestValidateAgentUniquenessRejectsNestedAgentDirAlias(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	existing := []map[string]interface{}{
		{"id": "work", "agentDir": "agents/work"},
	}

	err := validateAgentUniqueness(cfg, existing, "other", "", filepath.Join(dir, "agents", "work", "agent"), "")
	if err == nil || !strings.Contains(err.Error(), "agentDir 已被占用") {
		t.Fatalf("expected nested alias conflict, got %v", err)
	}
}

func TestLoadDefaultAgentIDFallsBackToExistingDiskAgentWhenNoDefaultConfigured(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{},
	})
	if err := os.MkdirAll(filepath.Join(dir, "agents", "work"), 0755); err != nil {
		t.Fatalf("mkdir work agent dir: %v", err)
	}

	if got := loadDefaultAgentID(cfg); got != "work" {
		t.Fatalf("expected disk-backed agent work to win when no explicit default is configured, got %q", got)
	}
}

func TestLoadDefaultAgentIDKeepsLegacyDefaultWithoutAgentDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "work",
		},
	})

	if got := loadDefaultAgentID(cfg); got != "work" {
		t.Fatalf("expected configured legacy default work to be preserved without agent dir, got %q", got)
	}
}

func TestCreateOpenClawAgentWritesExplicitDefaultFlagOnly(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{"id": "main", "default": true},
			},
		},
	})

	r := gin.New()
	r.POST("/openclaw/agents", CreateOpenClawAgent(cfg))
	body := []byte(`{"id":"work","default":true}`)
	req := httptest.NewRequest(http.MethodPost, "/openclaw/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	if agents == nil {
		t.Fatalf("agents should exist")
	}
	if _, ok := agents["default"]; ok {
		t.Fatalf("legacy agents.default should not be written anymore")
	}
	list, _ := agents["list"].([]interface{})
	if len(list) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(list))
	}
	defaultByID := map[string]interface{}{}
	for _, raw := range list {
		item, _ := raw.(map[string]interface{})
		id := strings.TrimSpace(getString(item, "id"))
		defaultByID[id] = item["default"]
	}
	if got := defaultByID["work"]; got != true {
		t.Fatalf("expected work to be the only explicit default, got %#v", defaultByID)
	}
	if got, ok := defaultByID["main"]; ok && got != nil {
		t.Fatalf("expected non-default main agent to omit default flag, got %#v", defaultByID)
	}
}

func TestCreateOpenClawAgentRejectsDuplicateExplicitAgentID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{"id": "main", "default": true},
				map[string]interface{}{"id": "work"},
			},
		},
	})

	r := gin.New()
	r.POST("/openclaw/agents", CreateOpenClawAgent(cfg))
	body := []byte(`{"id":"work","name":"Duplicate"}`)
	req := httptest.NewRequest(http.MethodPost, "/openclaw/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected duplicate explicit create to be rejected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateOpenClawAgentMaterializesDiskOnlyAgentsWithoutDroppingOthers(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
		},
	})
	if err := os.MkdirAll(filepath.Join(dir, "agents", "work"), 0755); err != nil {
		t.Fatalf("mkdir work agent dir: %v", err)
	}

	r := gin.New()
	r.POST("/openclaw/agents", CreateOpenClawAgent(cfg))
	body := []byte(`{"id":"work","name":"Work","default":false}`)
	req := httptest.NewRequest(http.MethodPost, "/openclaw/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	list, _ := agents["list"].([]interface{})
	if len(list) != 2 {
		t.Fatalf("expected implicit main and disk work to both remain materialized, got %#v", list)
	}
	var ids []string
	var workItem map[string]interface{}
	for _, raw := range list {
		item, _ := raw.(map[string]interface{})
		if item == nil {
			continue
		}
		id := strings.TrimSpace(getString(item, "id"))
		if id != "" {
			ids = append(ids, id)
		}
		if id == "work" {
			workItem = item
		}
	}
	if strings.Join(ids, ",") != "main,work" {
		t.Fatalf("expected materialized list to preserve main placeholder and work agent, got %v", ids)
	}
	if workItem == nil || getString(workItem, "name") != "Work" {
		t.Fatalf("expected work agent fields to be merged into disk-discovered agent, got %#v", workItem)
	}
}

func TestCreateOpenClawAgentPreservesLegacyDefaultWithoutAgentDir(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "work",
		},
	})

	r := gin.New()
	r.POST("/openclaw/agents", CreateOpenClawAgent(cfg))
	body := []byte(`{"id":"support","name":"Support"}`)
	req := httptest.NewRequest(http.MethodPost, "/openclaw/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	list, _ := agents["list"].([]interface{})
	if len(list) != 2 {
		t.Fatalf("expected legacy default work and new support agent to both be materialized, got %#v", list)
	}
	defaultByID := map[string]interface{}{}
	for _, raw := range list {
		item, _ := raw.(map[string]interface{})
		id := strings.TrimSpace(getString(item, "id"))
		defaultByID[id] = item["default"]
	}
	if got := defaultByID["work"]; got != true {
		t.Fatalf("expected legacy default work to stay explicit after create, got %#v", defaultByID)
	}
}

func TestUpdateOpenClawAgentAllowsClearingSandboxOverrideWithNull(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "default": true},
				map[string]interface{}{
					"id":   "work",
					"name": "Work",
					"sandbox": map[string]interface{}{
						"mode":            "non-main",
						"workspaceAccess": "none",
						"browser": map[string]interface{}{
							"allowHostControl": true,
						},
					},
				},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/agents/:id", UpdateOpenClawAgent(cfg))
	body := []byte(`{"sandbox":null}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/work", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	list, _ := agents["list"].([]interface{})
	var workItem map[string]interface{}
	for _, raw := range list {
		item, _ := raw.(map[string]interface{})
		if strings.TrimSpace(getString(item, "id")) == "work" {
			workItem = item
			break
		}
	}
	if workItem == nil {
		t.Fatalf("expected work agent to remain present")
	}
	if _, ok := workItem["sandbox"]; ok {
		t.Fatalf("expected sandbox override to be removed, got %#v", workItem["sandbox"])
	}
}

func TestUpdateOpenClawAgentAllowsUnchangedLegacyAvatar(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "default": true},
				map[string]interface{}{
					"id":   "work",
					"name": "Work",
					"identity": map[string]interface{}{
						"avatar": "/legacy/avatar.png",
					},
				},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/agents/:id", UpdateOpenClawAgent(cfg))
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/work", bytes.NewReader([]byte(`{"name":"Renamed Work"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unchanged legacy avatar, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	list, _ := agents["list"].([]interface{})
	var workItem map[string]interface{}
	for _, raw := range list {
		item, _ := raw.(map[string]interface{})
		if strings.TrimSpace(getString(item, "id")) == "work" {
			workItem = item
			break
		}
	}
	if got := strings.TrimSpace(getString(workItem, "name")); got != "Renamed Work" {
		t.Fatalf("expected name update to be saved, got %q", got)
	}
	identity, _ := workItem["identity"].(map[string]interface{})
	if got := strings.TrimSpace(getString(identity, "avatar")); got != "/legacy/avatar.png" {
		t.Fatalf("expected legacy avatar to be preserved, got %q", got)
	}
}

func TestUpdateOpenClawAgentPreservesLegacyIdentityFields(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "default": true},
				map[string]interface{}{
					"id":   "work",
					"name": "Work",
					"identity": map[string]interface{}{
						"description": "friendly crab",
						"customKey":   "kept",
					},
				},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/agents/:id", UpdateOpenClawAgent(cfg))
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/work", bytes.NewReader([]byte(`{"name":"Renamed Work"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when updating agent with legacy identity fields, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	list, _ := agents["list"].([]interface{})
	var workItem map[string]interface{}
	for _, raw := range list {
		item, _ := raw.(map[string]interface{})
		if strings.TrimSpace(getString(item, "id")) == "work" {
			workItem = item
			break
		}
	}
	identity, _ := workItem["identity"].(map[string]interface{})
	if got := strings.TrimSpace(getString(identity, "description")); got != "friendly crab" {
		t.Fatalf("expected description to survive update, got %q", got)
	}
	if got := strings.TrimSpace(getString(identity, "customKey")); got != "kept" {
		t.Fatalf("expected custom identity key to survive update, got %q", got)
	}
	if got := strings.TrimSpace(getString(identity, "theme")); got != "friendly crab" {
		t.Fatalf("expected theme to be backfilled from legacy description, got %q", got)
	}
}

func TestUpdateOpenClawAgentDropsUnsupportedPerAgentContextOverrides(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "default": true},
				map[string]interface{}{
					"id":            "work",
					"name":          "Work",
					"contextTokens": 4096,
					"compaction": map[string]interface{}{
						"mode":            "safeguard",
						"maxHistoryShare": 0.5,
					},
					"tools": map[string]interface{}{
						"profile": "coding",
						"agentToAgent": map[string]interface{}{
							"enabled": true,
						},
						"sessions": map[string]interface{}{
							"visibility": "all",
						},
					},
				},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/agents/:id", UpdateOpenClawAgent(cfg))
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/work", bytes.NewReader([]byte(`{"name":"Renamed Work","contextTokens":8192,"compaction":{"mode":"default","maxHistoryShare":0.6},"tools":{"profile":"full","agentToAgent":{"enabled":false},"sessions":{"visibility":"self"}}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	list, _ := agents["list"].([]interface{})
	var workItem map[string]interface{}
	for _, raw := range list {
		item, _ := raw.(map[string]interface{})
		if strings.TrimSpace(getString(item, "id")) == "work" {
			workItem = item
			break
		}
	}
	if got := strings.TrimSpace(getString(workItem, "name")); got != "Renamed Work" {
		t.Fatalf("expected name update to be saved, got %q", got)
	}
	if _, ok := workItem["contextTokens"]; ok {
		t.Fatalf("expected unsupported per-agent contextTokens to be dropped, got %#v", workItem["contextTokens"])
	}
	if _, ok := workItem["compaction"]; ok {
		t.Fatalf("expected unsupported per-agent compaction to be dropped, got %#v", workItem["compaction"])
	}
	tools, _ := workItem["tools"].(map[string]interface{})
	if got := strings.TrimSpace(getString(tools, "profile")); got != "full" {
		t.Fatalf("expected supported per-agent tools.profile to be updated, got %q", got)
	}
	if _, ok := tools["agentToAgent"]; ok {
		t.Fatalf("expected unsupported per-agent tools.agentToAgent to be dropped, got %#v", tools["agentToAgent"])
	}
	if _, ok := tools["sessions"]; ok {
		t.Fatalf("expected unsupported per-agent tools.sessions to be dropped, got %#v", tools["sessions"])
	}
}

func TestUpdateOpenClawAgentPreservesSupportedSubagentAllowlist(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "default": true},
				map[string]interface{}{
					"id":   "work",
					"name": "Work",
					"subagents": map[string]interface{}{
						"allowAgents": []interface{}{"research"},
					},
				},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/agents/:id", UpdateOpenClawAgent(cfg))
	req := httptest.NewRequest(http.MethodPut, "/openclaw/agents/work", bytes.NewReader([]byte(`{"name":"Renamed Work","subagents":{"allowAgents":["reviewer","*"]}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	list, _ := agents["list"].([]interface{})
	var workItem map[string]interface{}
	for _, raw := range list {
		item, _ := raw.(map[string]interface{})
		if strings.TrimSpace(getString(item, "id")) == "work" {
			workItem = item
			break
		}
	}
	if got := strings.TrimSpace(getString(workItem, "name")); got != "Renamed Work" {
		t.Fatalf("expected name update to be saved, got %q", got)
	}
	subagents, _ := workItem["subagents"].(map[string]interface{})
	allowAgents, _ := subagents["allowAgents"].([]interface{})
	if len(allowAgents) != 2 || strings.TrimSpace(toString(allowAgents[0])) != "reviewer" || strings.TrimSpace(toString(allowAgents[1])) != "*" {
		t.Fatalf("expected supported subagents.allowAgents to be preserved, got %#v", subagents["allowAgents"])
	}
}

func TestGetOpenClawAgentsMarksSynthesizedDiskAgentsImplicit(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
		},
	})
	if err := os.MkdirAll(filepath.Join(dir, "agents", "work"), 0755); err != nil {
		t.Fatalf("mkdir work agent dir: %v", err)
	}

	r := gin.New()
	r.GET("/openclaw/agents", GetOpenClawAgents(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK     bool `json:"ok"`
		Agents struct {
			List []map[string]interface{} `json:"list"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if len(resp.Agents.List) != 2 {
		t.Fatalf("expected synthesized main and disk work agent, got %d entries", len(resp.Agents.List))
	}

	var mainAgent, workAgent map[string]interface{}
	for _, item := range resp.Agents.List {
		switch item["id"] {
		case "main":
			mainAgent = item
		case "work":
			workAgent = item
		}
	}
	if mainAgent == nil || workAgent == nil {
		t.Fatalf("expected main and work agents, got %#v", resp.Agents.List)
	}
	if got := mainAgent["implicit"]; got != true {
		t.Fatalf("expected synthesized main to be implicit=true, got %#v", got)
	}
	if got := workAgent["implicit"]; got != true {
		t.Fatalf("expected synthesized disk work agent to be implicit=true, got %#v", got)
	}
}

func TestDeleteOpenClawAgentKeepsSessionsWhenConfigWriteFails(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based write failure injection is unreliable on Windows")
	}

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
	})
	writeJSON(t, filepath.Join(dir, "agents", "work", "sessions", "sessions.json"), map[string]interface{}{
		"work-key": map[string]interface{}{
			"sessionId": "session-work",
		},
	})
	configPath := filepath.Join(dir, "openclaw.json")
	if err := os.Chmod(configPath, 0400); err != nil {
		t.Fatalf("chmod openclaw.json read-only: %v", err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod openclaw dir read-only: %v", err)
	}
	defer func() {
		_ = os.Chmod(dir, 0700)
		_ = os.Chmod(configPath, 0600)
	}()

	r := gin.New()
	r.DELETE("/openclaw/agents/:id", DeleteOpenClawAgent(cfg))
	req := httptest.NewRequest(http.MethodDelete, "/openclaw/agents/work", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "agents", "work", "sessions", "sessions.json")); err != nil {
		t.Fatalf("sessions should be restored after config write failure: %v", err)
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	list, _ := agents["list"].([]interface{})
	if len(list) != 2 {
		t.Fatalf("agent list should remain unchanged on failure, got %#v", list)
	}
}

func TestDeleteOpenClawAgentRewritesCronTargets(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "default": true},
				map[string]interface{}{"id": "work"},
				map[string]interface{}{"id": "audit"},
			},
		},
		"cron": map[string]interface{}{
			"jobs": []interface{}{
				map[string]interface{}{"id": "cfg_job_1", "sessionTarget": "work"},
				map[string]interface{}{"id": "cfg_job_2", "agentId": "work", "sessionTarget": "main"},
				map[string]interface{}{"id": "cfg_job_3", "agentId": "work", "sessionTarget": "work"},
				map[string]interface{}{"id": "cfg_job_4", "agentId": "audit", "sessionTarget": "work"},
				map[string]interface{}{"id": "cfg_job_5", "agentId": "work"},
			},
		},
	})
	writeJSON(t, filepath.Join(dir, "cron", "jobs.json"), map[string]interface{}{
		"version": 1,
		"jobs": []interface{}{
			map[string]interface{}{"id": "job_1", "sessionTarget": "work"},
			map[string]interface{}{"id": "job_2", "sessionTarget": "main"},
			map[string]interface{}{"id": "job_3", "agentId": "work", "sessionTarget": "main"},
			map[string]interface{}{"id": "job_4", "agentId": "work", "sessionTarget": "work"},
			map[string]interface{}{"id": "job_5", "agentId": "audit", "sessionTarget": "work"},
			map[string]interface{}{"id": "job_6", "agentId": "work"},
		},
	})

	r := gin.New()
	r.DELETE("/openclaw/agents/:id", DeleteOpenClawAgent(cfg))
	req := httptest.NewRequest(http.MethodDelete, "/openclaw/agents/work", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "cron", "jobs.json"))
	if err != nil {
		t.Fatalf("read cron jobs: %v", err)
	}
	var saved map[string]interface{}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("decode cron jobs: %v", err)
	}
	jobs, _ := saved["jobs"].([]interface{})
	if len(jobs) != 6 {
		t.Fatalf("expected six jobs, got %d", len(jobs))
	}
	job1, _ := jobs[0].(map[string]interface{})
	if got := strings.TrimSpace(getString(job1, "sessionTarget")); got != "main" {
		t.Fatalf("expected legacy deleted agent target to normalize to main session, got %q", got)
	}
	if got := strings.TrimSpace(getString(job1, "agentId")); got != "main" {
		t.Fatalf("expected legacy deleted agent target to fallback agentId to main, got %q", got)
	}
	job3, _ := jobs[2].(map[string]interface{})
	if got := strings.TrimSpace(getString(job3, "agentId")); got != "main" {
		t.Fatalf("expected deleted agentId to fallback to main, got %q", got)
	}
	job4, _ := jobs[3].(map[string]interface{})
	if got := strings.TrimSpace(getString(job4, "agentId")); got != "main" {
		t.Fatalf("expected mixed legacy job agentId to fallback to main, got %q", got)
	}
	if got := strings.TrimSpace(getString(job4, "sessionTarget")); got != "main" {
		t.Fatalf("expected mixed legacy job sessionTarget to normalize to main, got %q", got)
	}
	job5, _ := jobs[4].(map[string]interface{})
	if got := strings.TrimSpace(getString(job5, "agentId")); got != "audit" {
		t.Fatalf("expected non-deleted agentId to stay audit, got %q", got)
	}
	if got := strings.TrimSpace(getString(job5, "sessionTarget")); got != "main" {
		t.Fatalf("expected legacy sessionTarget on audit job to normalize to main, got %q", got)
	}
	job6, _ := jobs[5].(map[string]interface{})
	if got := strings.TrimSpace(getString(job6, "agentId")); got != "main" {
		t.Fatalf("expected blank-target deleted agent job to fallback to main, got %q", got)
	}
	if got := strings.TrimSpace(getString(job6, "sessionTarget")); got != "main" {
		t.Fatalf("expected blank-target deleted agent job to normalize sessionTarget to main, got %q", got)
	}
	savedCfg, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	cronCfg, _ := savedCfg["cron"].(map[string]interface{})
	cfgJobs, _ := cronCfg["jobs"].([]interface{})
	if len(cfgJobs) != 5 {
		t.Fatalf("expected five config cron jobs, got %d", len(cfgJobs))
	}
	cfgJob1, _ := cfgJobs[0].(map[string]interface{})
	if got := strings.TrimSpace(getString(cfgJob1, "agentId")); got != "main" {
		t.Fatalf("expected openclaw.json legacy cron job to fallback agentId to main, got %q", got)
	}
	if got := strings.TrimSpace(getString(cfgJob1, "sessionTarget")); got != "main" {
		t.Fatalf("expected openclaw.json legacy cron job to normalize sessionTarget to main, got %q", got)
	}
	cfgJob2, _ := cfgJobs[1].(map[string]interface{})
	if got := strings.TrimSpace(getString(cfgJob2, "agentId")); got != "main" {
		t.Fatalf("expected openclaw.json agentId cron job to fallback to main, got %q", got)
	}
	cfgJob3, _ := cfgJobs[2].(map[string]interface{})
	if got := strings.TrimSpace(getString(cfgJob3, "agentId")); got != "main" {
		t.Fatalf("expected openclaw.json mixed legacy job agentId to fallback to main, got %q", got)
	}
	if got := strings.TrimSpace(getString(cfgJob3, "sessionTarget")); got != "main" {
		t.Fatalf("expected openclaw.json mixed legacy job sessionTarget to normalize to main, got %q", got)
	}
	cfgJob4, _ := cfgJobs[3].(map[string]interface{})
	if got := strings.TrimSpace(getString(cfgJob4, "agentId")); got != "audit" {
		t.Fatalf("expected openclaw.json non-deleted agentId to stay audit, got %q", got)
	}
	if got := strings.TrimSpace(getString(cfgJob4, "sessionTarget")); got != "main" {
		t.Fatalf("expected openclaw.json legacy sessionTarget on audit job to normalize to main, got %q", got)
	}
	cfgJob5, _ := cfgJobs[4].(map[string]interface{})
	if got := strings.TrimSpace(getString(cfgJob5, "agentId")); got != "main" {
		t.Fatalf("expected openclaw.json blank-target deleted agent job to fallback to main, got %q", got)
	}
	if got := strings.TrimSpace(getString(cfgJob5, "sessionTarget")); got != "main" {
		t.Fatalf("expected openclaw.json blank-target deleted agent job to normalize sessionTarget to main, got %q", got)
	}
}

func TestDeleteOpenClawAgentRestoresSessionsWhenCronMirrorWriteFails(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "default": true},
				map[string]interface{}{"id": "work"},
			},
		},
		"cron": map[string]interface{}{
			"jobs": []interface{}{
				map[string]interface{}{"id": "cfg_job_1", "sessionTarget": "work"},
			},
		},
	})
	writeJSON(t, filepath.Join(dir, "cron", "jobs.json"), map[string]interface{}{
		"version": 1,
		"jobs": []interface{}{
			map[string]interface{}{"id": "job_1", "sessionTarget": "work"},
		},
	})
	if err := os.MkdirAll(filepath.Join(dir, "cron", "jobs.json.bak"), 0755); err != nil {
		t.Fatalf("mkdir blocking cron backup dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cron", "jobs.json.bak", "keep"), []byte("1"), 0644); err != nil {
		t.Fatalf("seed blocking cron backup dir: %v", err)
	}
	sessionsDir := filepath.Join(dir, "agents", "work", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("mkdir sessions dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "sessions.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed sessions file: %v", err)
	}

	r := gin.New()
	r.DELETE("/openclaw/agents/:id", DeleteOpenClawAgent(cfg))
	req := httptest.NewRequest(http.MethodDelete, "/openclaw/agents/work", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(sessionsDir, "sessions.json")); err != nil {
		t.Fatalf("expected staged sessions to be restored after cron failure: %v", err)
	}
}

func TestValidateAgentUniquenessNormalizesPaths(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	dataDir := filepath.Join(base, "data", "work")
	cfg := &config.Config{OpenClawDir: base}
	err := validateAgentUniqueness(cfg, []map[string]interface{}{
		{"id": "main", "workspace": dataDir + "/", "agentDir": "agents/main/"},
	}, "work", dataDir, "agents/main", "")
	if err == nil {
		t.Fatalf("expected normalized path conflict")
	}
}

func TestValidateAgentUniquenessRejectsAgentDirOutsideBase(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cfg := &config.Config{OpenClawDir: base}
	external := filepath.Join(t.TempDir(), "isolated-agent")
	err := validateAgentUniqueness(cfg, []map[string]interface{}{{"id": "main"}}, "work", "workspace/work", external, "")
	if err == nil || !strings.Contains(err.Error(), "agentDir 必须位于 OpenClaw 目录内") {
		t.Fatalf("expected external agentDir to be rejected, got %v", err)
	}
}

func TestValidateAgentUniquenessRejectsAbsoluteAliasConflict(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cfg := &config.Config{OpenClawDir: base}
	absDir := filepath.Join(base, "custom", "work-agent")
	err := validateAgentUniqueness(cfg, []map[string]interface{}{
		{"id": "main", "agentDir": absDir},
	}, "work", "", "custom/work-agent", "")
	if err == nil {
		t.Fatalf("expected absolute alias conflict")
	}
}

func TestResolveAgentSessionsDirIgnoresConfiguredExternalAgentDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	external := filepath.Join(t.TempDir(), "external-agent", "agent")
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{"id": "work", "agentDir": external},
			},
		},
	})

	got := resolveAgentSessionsDir(cfg, "work")
	want := filepath.Join(dir, "agents", "work", "sessions")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestGetSessionDetailUsesRecordedSessionFilePath(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "work",
			"list": []interface{}{
				map[string]interface{}{"id": "work"},
			},
		},
	})

	sessionFile := filepath.Join(dir, "agents", "work", "sessions", "s_work.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	if err := os.WriteFile(sessionFile, []byte(`{"type":"assistant","id":"2","message":{"role":"assistant","content":"hello"}}`+"\n"), 0644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	writeJSON(t, filepath.Join(dir, "agents", "work", "sessions", "sessions.json"), map[string]interface{}{
		"work-key": map[string]interface{}{
			"sessionId":   "session-work",
			"chatType":    "group",
			"updatedAt":   float64(2000),
			"sessionFile": sessionFile,
		},
	})

	r := gin.New()
	r.GET("/sessions/:id", GetSessionDetail(cfg))
	req := httptest.NewRequest(http.MethodGet, "/sessions/session-work?agent=work", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK       bool                     `json:"ok"`
		Messages []map[string]interface{} `json:"messages"`
		Error    string                   `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || resp.Error != "" {
		t.Fatalf("expected successful session detail, got %+v", resp)
	}
	if len(resp.Messages) != 1 {
		t.Fatalf("expected 1 message, got %+v", resp.Messages)
	}
}

func TestValidateAgentUniquenessAllowsWorkspaceOutsideBase(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cfg := &config.Config{OpenClawDir: base}
	// workspace 使用 OpenClawDir 外部的绝对路径（如外部硬盘）应当通过
	err := validateAgentUniqueness(cfg, []map[string]interface{}{{"id": "main"}}, "work", "/Volumes/external/workspaces/work", "agents/work", "")
	if err != nil {
		t.Fatalf("workspace outside OpenClawDir should be allowed, got: %v", err)
	}
}

func TestGetOpenClawAgentsFallsBackToEffectiveDefaultWhenConfiguredDefaultInvalid(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "ghost",
			"list": []interface{}{
				map[string]interface{}{"id": "work"},
				map[string]interface{}{"id": "main"},
			},
		},
	})

	r := gin.New()
	r.GET("/openclaw/agents", GetOpenClawAgents(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK     bool `json:"ok"`
		Agents struct {
			Default string                   `json:"default"`
			List    []map[string]interface{} `json:"list"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if resp.Agents.Default != "work" {
		t.Fatalf("expected effective default work, got %q", resp.Agents.Default)
	}
	if len(resp.Agents.List) != 2 || resp.Agents.List[0]["default"] != true {
		t.Fatalf("expected first explicit agent to be marked default, got %#v", resp.Agents.List)
	}
}

func TestPreviewRouteRespectsBindingOrder(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}

	oc := map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "work",
				"comment": "work-group",
				"match": map[string]interface{}{
					"channel": "qq",
					"peer": map[string]interface{}{
						"kind": "group",
						"id":   "123",
					},
				},
			},
			map[string]interface{}{
				"agentId": "main",
				"comment": "fallback-qq",
				"match": map[string]interface{}{
					"channel": "qq",
				},
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), oc)

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))

	body := []byte(`{"meta":{"channel":"qq","peer":"group:123"}}`)
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Agent        string   `json:"agent"`
			MatchedBy    string   `json:"matchedBy"`
			MatchedIndex int      `json:"matchedIndex"`
			Trace        []string `json:"trace"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if resp.Result.Agent != "work" {
		t.Fatalf("expected first binding to match work, got %s", resp.Result.Agent)
	}
	if resp.Result.MatchedBy != "binding.peer" {
		t.Fatalf("expected matchedBy binding.peer, got %s", resp.Result.MatchedBy)
	}
	if resp.Result.MatchedIndex != 0 {
		t.Fatalf("expected matchedIndex 0, got %d", resp.Result.MatchedIndex)
	}
}

func TestSaveCronJobsRejectsUnknownSessionTarget(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
			},
		},
	})

	r := gin.New()
	r.PUT("/system/cron", SaveCronJobs(cfg))

	body := []byte(`{"jobs":[{"id":"job_1","name":"bad","enabled":true,"schedule":{"kind":"cron","expr":"0 9 * * *"},"sessionTarget":"work","wakeMode":"now","payload":{"kind":"agentTurn","message":"hi"},"state":{},"createdAtMs":1}]}`)
	req := httptest.NewRequest(http.MethodPut, "/system/cron", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid sessionTarget, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSaveCronJobsRejectsOrphanAgentDirWhenExplicitListExists(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(dir, "agents", "work"), 0755); err != nil {
		t.Fatalf("mkdir orphan work dir: %v", err)
	}

	r := gin.New()
	r.PUT("/system/cron", SaveCronJobs(cfg))
	body := []byte(`{"jobs":[{"id":"job_1","name":"orphan-target","enabled":true,"schedule":{"kind":"cron","expr":"0 9 * * *"},"sessionTarget":"work","wakeMode":"now","payload":{"kind":"agentTurn","message":"hi"},"state":{},"createdAtMs":1}]}`)
	req := httptest.NewRequest(http.MethodPut, "/system/cron", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for orphan agent dir target, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPreviewRoutePrefersHigherPriorityOverRuleOrder(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "main",
				"match": map[string]interface{}{
					"channel": "qq",
				},
			},
			map[string]interface{}{
				"agentId": "work",
				"match": map[string]interface{}{
					"channel": "qq",
					"peer": map[string]interface{}{
						"kind": "group",
						"id":   "123",
					},
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"qq","peer":"group:123"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Agent        string `json:"agent"`
			MatchedBy    string `json:"matchedBy"`
			MatchedIndex int    `json:"matchedIndex"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "work" {
		t.Fatalf("expected higher-priority peer rule to win, got %s", resp.Result.Agent)
	}
	if !strings.Contains(resp.Result.MatchedBy, "peer") {
		t.Fatalf("matchedBy should indicate peer priority, got %s", resp.Result.MatchedBy)
	}
	if resp.Result.MatchedIndex != 1 {
		t.Fatalf("expected matchedIndex 1 for higher-priority second rule, got %d", resp.Result.MatchedIndex)
	}
}

func TestPreviewRouteChannelOnlyBindingUsesDefaultAccountScope(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "work",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
		"channels": map[string]interface{}{
			"discord": map[string]interface{}{
				"accounts": map[string]interface{}{
					"default": map[string]interface{}{},
					"coding":  map[string]interface{}{},
				},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "main",
				"match": map[string]interface{}{
					"channel": "discord",
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"discord","accountId":"coding"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Agent string   `json:"agent"`
			Trace []string `json:"trace"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "work" {
		t.Fatalf("expected fallback to default agent work when account mismatch, got %s", resp.Result.Agent)
	}
	joined := strings.Join(resp.Result.Trace, "\n")
	if !strings.Contains(joined, "fallback default: work") {
		t.Fatalf("trace should mention default fallback, got: %s", joined)
	}
}

func TestPreviewRouteMissingAccountUsesChannelDefaultAccount(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "work",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
		"channels": map[string]interface{}{
			"discord": map[string]interface{}{
				"accounts": map[string]interface{}{
					"default": map[string]interface{}{},
					"coding":  map[string]interface{}{},
				},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "main",
				"enabled": true,
				"match": map[string]interface{}{
					"channel":   "discord",
					"accountId": "default",
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"discord"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Agent string `json:"agent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "main" {
		t.Fatalf("expected explicit default-account binding to match, got %s", resp.Result.Agent)
	}
}

func TestSaveBindingsRequiresChannelField(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/bindings", SaveOpenClawBindings(cfg))
	body := []byte(`{"bindings":[{"agentId":"main","match":{"peer":{"kind":"group","id":"123"}}}]}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/bindings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when channel is missing, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "channel") {
		t.Fatalf("error should mention channel requirement, got: %s", w.Body.String())
	}
}

func TestSaveBindingsRejectsNonStringMatchArrayItem(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/bindings", SaveOpenClawBindings(cfg))
	body := []byte(`{"bindings":[{"agentId":"main","match":{"channel":"qq","guildId":"guild-1","roles":[1]}}]}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/bindings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when match array contains non-string item, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "仅支持字符串数组") {
		t.Fatalf("error should mention string-only array items, got: %s", w.Body.String())
	}
}

func TestPreviewRouteSkipsDisabledBinding(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "work",
				"enabled": false,
				"match": map[string]interface{}{
					"channel": "qq",
					"peer": map[string]interface{}{
						"kind": "group",
						"id":   "123",
					},
				},
			},
			map[string]interface{}{
				"agentId": "main",
				"match": map[string]interface{}{
					"channel": "qq",
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"qq","peer":"group:123"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Agent        string   `json:"agent"`
			MatchedBy    string   `json:"matchedBy"`
			MatchedIndex int      `json:"matchedIndex"`
			Trace        []string `json:"trace"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "main" {
		t.Fatalf("expected disabled binding to be skipped in favor of main, got %s", resp.Result.Agent)
	}
	if resp.Result.MatchedBy != "binding.channel" || resp.Result.MatchedIndex != 1 {
		t.Fatalf("expected fallback channel binding to win, got matchedBy=%s index=%d", resp.Result.MatchedBy, resp.Result.MatchedIndex)
	}
	if !strings.Contains(strings.Join(resp.Result.Trace, "\n"), "skip bindings[0]: disabled") {
		t.Fatalf("expected trace to mention disabled binding, got: %v", resp.Result.Trace)
	}
}

func TestSaveBindingsPreservesDisabledFlag(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/bindings", SaveOpenClawBindings(cfg))
	body := []byte(`{"bindings":[{"agentId":"work","enabled":false,"match":{"channel":"qq"}}]}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/bindings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	bindings, _ := saved["bindings"].([]interface{})
	if len(bindings) != 1 {
		t.Fatalf("expected one binding, got %#v", saved["bindings"])
	}
	binding, _ := bindings[0].(map[string]interface{})
	if enabled, ok := binding["enabled"].(bool); !ok || enabled {
		t.Fatalf("expected enabled=false to be preserved, got %#v", binding["enabled"])
	}
}

func TestGetOpenClawAgentIdentityAvatarRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	workspace := filepath.Join(dir, "workspace", "main")
	outside := filepath.Join(dir, "outside")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "evil.png"), []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0o644); err != nil {
		t.Fatalf("write evil png: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatalf("symlink escape dir: %v", err)
	}

	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{
					"id":        "main",
					"workspace": workspace,
					"identity": map[string]interface{}{
						"avatar": "escape/evil.png",
					},
				},
			},
		},
	})

	r := gin.New()
	r.GET("/openclaw/agents/:id/identity/avatar", GetOpenClawAgentIdentityAvatar(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/agents/main/identity/avatar", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for symlink escape avatar, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "agent workspace") {
		t.Fatalf("expected workspace containment error, got: %s", w.Body.String())
	}
}

func TestPreviewRouteSupportsPeerObjectMatch(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "team"},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "team",
				"enabled": true,
				"match": map[string]interface{}{
					"channel": "wechat",
					"peer": map[string]interface{}{
						"kind": "group",
						"id":   "8765",
					},
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"wechat","peer":"group:8765"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Agent string `json:"agent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "team" {
		t.Fatalf("expected peer object rule to match team, got %s", resp.Result.Agent)
	}
}

func TestGetSessionsUsesConfiguredDefaultAgent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "work",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
	})

	workSessionFile := filepath.Join(dir, "agents", "work", "sessions", "s_work.jsonl")
	if err := os.MkdirAll(filepath.Dir(workSessionFile), 0755); err != nil {
		t.Fatalf("mkdir work sessions dir: %v", err)
	}
	_ = os.WriteFile(workSessionFile, []byte(`{"type":"assistant","id":"2","message":{"role":"assistant","content":"hello"}}`+"\n"), 0644)
	writeJSON(t, filepath.Join(dir, "agents", "work", "sessions", "sessions.json"), map[string]interface{}{
		"work-key": map[string]interface{}{
			"sessionId":   "session-work",
			"chatType":    "group",
			"updatedAt":   float64(2000),
			"sessionFile": workSessionFile,
		},
	})

	r := gin.New()
	r.GET("/sessions", GetSessions(cfg))
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK       bool          `json:"ok"`
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].AgentID != "work" {
		t.Fatalf("expected default agent work sessions, got %+v", resp.Sessions)
	}
}

func TestGetSessionsFallsBackWhenConfiguredDefaultInvalid(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "ghost",
			"list": []interface{}{
				map[string]interface{}{"id": "work"},
			},
		},
	})

	workSessionFile := filepath.Join(dir, "agents", "work", "sessions", "s_work.jsonl")
	if err := os.MkdirAll(filepath.Dir(workSessionFile), 0755); err != nil {
		t.Fatalf("mkdir work sessions dir: %v", err)
	}
	_ = os.WriteFile(workSessionFile, []byte(`{"type":"assistant","id":"2","message":{"role":"assistant","content":"hello"}}`+"\n"), 0644)
	writeJSON(t, filepath.Join(dir, "agents", "work", "sessions", "sessions.json"), map[string]interface{}{
		"work-key": map[string]interface{}{
			"sessionId":   "session-work",
			"chatType":    "group",
			"updatedAt":   float64(2000),
			"sessionFile": workSessionFile,
		},
	})

	r := gin.New()
	r.GET("/sessions", GetSessions(cfg))
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with fallback agent, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK       bool          `json:"ok"`
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].AgentID != "work" {
		t.Fatalf("expected fallback to existing work agent, got %+v", resp.Sessions)
	}
}

func TestGetSessionsFallbackPreservesExplicitListOrder(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "ghost",
			"list": []interface{}{
				map[string]interface{}{"id": "work"},
				map[string]interface{}{"id": "main"},
			},
		},
	})

	workSessionFile := filepath.Join(dir, "agents", "work", "sessions", "s_work.jsonl")
	if err := os.MkdirAll(filepath.Dir(workSessionFile), 0755); err != nil {
		t.Fatalf("mkdir work sessions dir: %v", err)
	}
	_ = os.WriteFile(workSessionFile, []byte(`{"type":"assistant","id":"2","message":{"role":"assistant","content":"hello"}}`+"\n"), 0644)
	writeJSON(t, filepath.Join(dir, "agents", "work", "sessions", "sessions.json"), map[string]interface{}{
		"work-key": map[string]interface{}{
			"sessionId":   "session-work",
			"chatType":    "group",
			"updatedAt":   float64(2000),
			"sessionFile": workSessionFile,
		},
	})

	mainSessionFile := filepath.Join(dir, "agents", "main", "sessions", "s_main.jsonl")
	if err := os.MkdirAll(filepath.Dir(mainSessionFile), 0755); err != nil {
		t.Fatalf("mkdir main sessions dir: %v", err)
	}
	_ = os.WriteFile(mainSessionFile, []byte(`{"type":"assistant","id":"3","message":{"role":"assistant","content":"world"}}`+"\n"), 0644)
	writeJSON(t, filepath.Join(dir, "agents", "main", "sessions", "sessions.json"), map[string]interface{}{
		"main-key": map[string]interface{}{
			"sessionId":   "session-main",
			"chatType":    "group",
			"updatedAt":   float64(1000),
			"sessionFile": mainSessionFile,
		},
	})

	r := gin.New()
	r.GET("/sessions", GetSessions(cfg))
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK       bool          `json:"ok"`
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].AgentID != "work" {
		t.Fatalf("expected fallback to first explicit agent work, got %+v", resp.Sessions)
	}
}

func TestGetSessionsFallbackIgnoresOrphanAgentDirWhenExplicitListExists(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "ghost",
			"list": []interface{}{
				map[string]interface{}{"id": "work"},
			},
		},
	})

	workSessionFile := filepath.Join(dir, "agents", "work", "sessions", "s_work.jsonl")
	if err := os.MkdirAll(filepath.Dir(workSessionFile), 0755); err != nil {
		t.Fatalf("mkdir work sessions dir: %v", err)
	}
	_ = os.WriteFile(workSessionFile, []byte(`{"type":"assistant","id":"2","message":{"role":"assistant","content":"hello"}}`+"\n"), 0644)
	writeJSON(t, filepath.Join(dir, "agents", "work", "sessions", "sessions.json"), map[string]interface{}{
		"work-key": map[string]interface{}{
			"sessionId":   "session-work",
			"chatType":    "group",
			"updatedAt":   float64(2000),
			"sessionFile": workSessionFile,
		},
	})

	orphanSessionFile := filepath.Join(dir, "agents", "aaa", "sessions", "s_orphan.jsonl")
	if err := os.MkdirAll(filepath.Dir(orphanSessionFile), 0755); err != nil {
		t.Fatalf("mkdir orphan sessions dir: %v", err)
	}
	_ = os.WriteFile(orphanSessionFile, []byte(`{"type":"assistant","id":"3","message":{"role":"assistant","content":"ghost"}}`+"\n"), 0644)
	writeJSON(t, filepath.Join(dir, "agents", "aaa", "sessions", "sessions.json"), map[string]interface{}{
		"aaa-key": map[string]interface{}{
			"sessionId":   "session-orphan",
			"chatType":    "group",
			"updatedAt":   float64(3000),
			"sessionFile": orphanSessionFile,
		},
	})

	r := gin.New()
	r.GET("/sessions", GetSessions(cfg))
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK       bool          `json:"ok"`
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Sessions) != 1 || resp.Sessions[0].AgentID != "work" {
		t.Fatalf("expected fallback to explicit work agent, got %+v", resp.Sessions)
	}
}

func TestPreviewRouteFallsBackToEffectiveDefaultAgent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "ghost",
			"list": []interface{}{
				map[string]interface{}{"id": "work"},
			},
		},
		"bindings": []interface{}{},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"qq","peer":"group:123"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Agent string `json:"agent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "work" {
		t.Fatalf("expected preview fallback to effective default work, got %s", resp.Result.Agent)
	}
}

func TestPreviewRouteWildcardAccountBindingBeatsChannelCatchAll(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
		"channels": map[string]interface{}{
			"feishu": map[string]interface{}{
				"defaultAccount": "default",
				"accounts": map[string]interface{}{
					"default": map[string]interface{}{
						"appId":     "cli_demo",
						"appSecret": "secret",
					},
				},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "main",
				"match": map[string]interface{}{
					"channel": "feishu",
				},
			},
			map[string]interface{}{
				"agentId": "work",
				"match": map[string]interface{}{
					"channel":   "feishu",
					"accountId": "*",
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"feishu"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Agent        string `json:"agent"`
			MatchedBy    string `json:"matchedBy"`
			MatchedIndex int    `json:"matchedIndex"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "work" {
		t.Fatalf("expected wildcard-account binding to beat channel catch-all, got %s", resp.Result.Agent)
	}
	if resp.Result.MatchedBy != "binding.account.wildcard" {
		t.Fatalf("expected matchedBy binding.account.wildcard, got %s", resp.Result.MatchedBy)
	}
	if resp.Result.MatchedIndex != 1 {
		t.Fatalf("expected matchedIndex 1, got %d", resp.Result.MatchedIndex)
	}
}

func TestPreviewRouteLegacyPeerWildcardStringMatchesAnyGroup(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "work",
				"match": map[string]interface{}{
					"channel": "qq",
					"peer":    "group:*",
				},
			},
			map[string]interface{}{
				"agentId": "main",
				"match": map[string]interface{}{
					"channel": "qq",
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"qq","peer":"group:88001"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK     bool `json:"ok"`
		Result struct {
			Agent     string `json:"agent"`
			MatchedBy string `json:"matchedBy"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "work" {
		t.Fatalf("expected legacy peer wildcard to match work, got %s", resp.Result.Agent)
	}
	if resp.Result.MatchedBy != "binding.peer" {
		t.Fatalf("expected matchedBy binding.peer, got %s", resp.Result.MatchedBy)
	}
}

func TestPreviewRouteLegacySenderAndParentPeerBindings(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "sender-agent"},
				map[string]interface{}{"id": "thread-agent"},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "sender-agent",
				"match": map[string]interface{}{
					"channel": "qq",
					"sender":  "alice",
				},
			},
			map[string]interface{}{
				"agentId": "thread-agent",
				"match": map[string]interface{}{
					"channel":    "qq",
					"parentPeer": "thread:release-notes",
				},
			},
			map[string]interface{}{
				"agentId": "main",
				"match": map[string]interface{}{
					"channel": "qq",
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))

	reqSender := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"qq","sender":"alice"}}`)))
	reqSender.Header.Set("Content-Type", "application/json")
	wSender := httptest.NewRecorder()
	r.ServeHTTP(wSender, reqSender)
	if wSender.Code != http.StatusOK {
		t.Fatalf("sender preview expected 200, got %d: %s", wSender.Code, wSender.Body.String())
	}
	var senderResp struct {
		Result struct {
			Agent     string `json:"agent"`
			MatchedBy string `json:"matchedBy"`
		} `json:"result"`
	}
	if err := json.Unmarshal(wSender.Body.Bytes(), &senderResp); err != nil {
		t.Fatalf("decode sender preview: %v", err)
	}
	if senderResp.Result.Agent != "sender-agent" || senderResp.Result.MatchedBy != "binding.sender" {
		t.Fatalf("expected sender rule to win, got agent=%s matchedBy=%s", senderResp.Result.Agent, senderResp.Result.MatchedBy)
	}

	reqParent := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"qq","parentPeer":"thread:release-notes"}}`)))
	reqParent.Header.Set("Content-Type", "application/json")
	wParent := httptest.NewRecorder()
	r.ServeHTTP(wParent, reqParent)
	if wParent.Code != http.StatusOK {
		t.Fatalf("parentPeer preview expected 200, got %d: %s", wParent.Code, wParent.Body.String())
	}
	var parentResp struct {
		Result struct {
			Agent     string `json:"agent"`
			MatchedBy string `json:"matchedBy"`
		} `json:"result"`
	}
	if err := json.Unmarshal(wParent.Body.Bytes(), &parentResp); err != nil {
		t.Fatalf("decode parentPeer preview: %v", err)
	}
	if parentResp.Result.Agent != "thread-agent" || parentResp.Result.MatchedBy != "binding.peer.parent" {
		t.Fatalf("expected parentPeer rule to win, got agent=%s matchedBy=%s", parentResp.Result.Agent, parentResp.Result.MatchedBy)
	}
}

func TestPreviewRouteLegacyPeerParentPeerFallback(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "thread-agent"},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "thread-agent",
				"match": map[string]interface{}{
					"channel":    "qq",
					"parentPeer": "thread:release-notes",
				},
			},
			map[string]interface{}{
				"agentId": "main",
				"match": map[string]interface{}{
					"channel": "qq",
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"qq","peer":"thread:release-notes"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Result struct {
			Agent     string `json:"agent"`
			MatchedBy string `json:"matchedBy"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "thread-agent" || resp.Result.MatchedBy != "binding.peer.parent" {
		t.Fatalf("expected peer input to fall back to parentPeer rule, got agent=%s matchedBy=%s", resp.Result.Agent, resp.Result.MatchedBy)
	}
}

func TestPreviewRouteParentPeerDoesNotPromotePeerTier(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "peer-agent"},
				map[string]interface{}{"id": "parent-agent"},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "peer-agent",
				"match": map[string]interface{}{
					"channel": "qq",
					"peer":    "thread:release-notes",
				},
			},
			map[string]interface{}{
				"agentId": "parent-agent",
				"match": map[string]interface{}{
					"channel":    "qq",
					"parentPeer": "thread:release-notes",
				},
			},
			map[string]interface{}{
				"agentId": "main",
				"match": map[string]interface{}{
					"channel": "qq",
				},
			},
		},
	})

	r := gin.New()
	r.POST("/route/preview", PreviewOpenClawRoute(cfg))
	req := httptest.NewRequest(http.MethodPost, "/route/preview", bytes.NewReader([]byte(`{"meta":{"channel":"qq","parentPeer":"thread:release-notes"}}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Result struct {
			Agent     string `json:"agent"`
			MatchedBy string `json:"matchedBy"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result.Agent != "parent-agent" || resp.Result.MatchedBy != "binding.peer.parent" {
		t.Fatalf("expected parentPeer-only input to stay on parent tier, got agent=%s matchedBy=%s", resp.Result.Agent, resp.Result.MatchedBy)
	}
}

func TestSaveBindingsPreservesLegacyMatchFields(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/bindings", SaveOpenClawBindings(cfg))
	body := []byte(`{"bindings":[{"agentId":"work","match":{"channel":"qq","sender":"alice","peer":"group:*","parentPeer":"thread:release-notes"}}]}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/bindings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "openclaw.json"))
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("decode openclaw.json: %v", err)
	}
	bindings, _ := stored["bindings"].([]interface{})
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	binding, _ := bindings[0].(map[string]interface{})
	match, _ := binding["match"].(map[string]interface{})
	if got := strings.TrimSpace(toString(match["sender"])); got != "alice" {
		t.Fatalf("expected sender to be preserved, got %q", got)
	}
	if got := strings.TrimSpace(toString(match["peer"])); got != "group:*" {
		t.Fatalf("expected peer wildcard string to be preserved, got %#v", match["peer"])
	}
	if got := strings.TrimSpace(toString(match["parentPeer"])); got != "thread:release-notes" {
		t.Fatalf("expected parentPeer string to be preserved, got %#v", match["parentPeer"])
	}
}

func TestGetBindingsPromotesLegacyNameAlias(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
			},
		},
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "main",
				"name":    "Legacy Label",
				"match": map[string]interface{}{
					"channel": "qq",
				},
			},
		},
	})

	r := gin.New()
	r.GET("/openclaw/bindings", GetOpenClawBindings(cfg))
	req := httptest.NewRequest(http.MethodGet, "/openclaw/bindings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		OK       bool                     `json:"ok"`
		Bindings []map[string]interface{} `json:"bindings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Bindings) != 1 {
		t.Fatalf("expected one binding, got %#v", resp.Bindings)
	}
	if got := strings.TrimSpace(getString(resp.Bindings[0], "comment")); got != "Legacy Label" {
		t.Fatalf("expected legacy name alias to surface as comment, got %q", got)
	}
	if _, ok := resp.Bindings[0]["name"]; ok {
		t.Fatalf("expected legacy name key to be normalized away in API response, got %#v", resp.Bindings[0]["name"])
	}
}

func TestSaveBindingsAcceptsLegacyNameAlias(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/bindings", SaveOpenClawBindings(cfg))
	body := []byte(`{"bindings":[{"agentId":"main","name":"Legacy Label","match":{"channel":"qq"}}]}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/bindings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	bindings, _ := saved["bindings"].([]interface{})
	if len(bindings) != 1 {
		t.Fatalf("expected one saved binding, got %#v", saved["bindings"])
	}
	binding, _ := bindings[0].(map[string]interface{})
	if got := strings.TrimSpace(getString(binding, "comment")); got != "Legacy Label" {
		t.Fatalf("expected legacy name alias to be written as comment, got %q", got)
	}
	if _, ok := binding["name"]; ok {
		t.Fatalf("expected saved binding to drop legacy name key, got %#v", binding["name"])
	}
}

func TestSaveBindingsRemovesLegacyAgentsBindingsKey(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
			},
			"bindings": []interface{}{
				map[string]interface{}{
					"agentId": "main",
					"match": map[string]interface{}{
						"channel": "legacy",
					},
				},
			},
		},
	})

	r := gin.New()
	r.PUT("/openclaw/bindings", SaveOpenClawBindings(cfg))
	body := []byte(`{"bindings":[{"agentId":"main","match":{"channel":"qq"}}]}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/bindings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read openclaw.json: %v", err)
	}
	agents, _ := saved["agents"].(map[string]interface{})
	if _, ok := agents["bindings"]; ok {
		t.Fatalf("expected legacy agents.bindings to be removed, got %#v", agents["bindings"])
	}
	bindings, _ := saved["bindings"].([]interface{})
	if len(bindings) != 1 {
		t.Fatalf("expected one top-level binding, got %#v", saved["bindings"])
	}
	binding, _ := bindings[0].(map[string]interface{})
	match, _ := binding["match"].(map[string]interface{})
	if got := strings.TrimSpace(getString(match, "channel")); got != "qq" {
		t.Fatalf("expected saved binding channel=qq, got %#v", match["channel"])
	}
}

func TestGetBindingsFromConfigFallsBackToLegacyWhenTopLevelBindingsEmpty(t *testing.T) {
	t.Parallel()

	ocConfig := map[string]interface{}{
		"bindings": []interface{}{},
	}
	agentsCfg := map[string]interface{}{
		"bindings": []interface{}{
			map[string]interface{}{
				"agentId": "main",
				"match":   map[string]interface{}{"channel": "qq"},
			},
		},
	}

	bindings := getBindingsFromConfig(ocConfig, agentsCfg)
	if len(bindings) != 1 {
		t.Fatalf("expected legacy agents.bindings fallback, got %#v", bindings)
	}
	if got := extractBindingAgentID(bindings[0]); got != "main" {
		t.Fatalf("unexpected binding fallback payload: %#v", bindings[0])
	}
}

func TestSaveCronJobsFillsSessionTargetWithDefaultAgent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "work",
			"list": []interface{}{
				map[string]interface{}{"id": "main"},
				map[string]interface{}{"id": "work"},
			},
		},
	})

	r := gin.New()
	r.PUT("/system/cron", SaveCronJobs(cfg))
	body := []byte(`{"jobs":[{"id":"job_1","name":"default-target","enabled":true,"schedule":{"kind":"cron","expr":"0 9 * * *"},"sessionTarget":"","wakeMode":"now","payload":{"kind":"agentTurn","message":"hi"},"state":{},"createdAtMs":1}]}`)
	req := httptest.NewRequest(http.MethodPut, "/system/cron", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(dir, "cron", "jobs.json"))
	if err != nil {
		t.Fatalf("read cron jobs: %v", err)
	}
	var saved map[string]interface{}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("decode cron jobs: %v", err)
	}
	jobs, _ := saved["jobs"].([]interface{})
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
	job, _ := jobs[0].(map[string]interface{})
	if got := strings.TrimSpace(getString(job, "sessionTarget")); got != "main" {
		t.Fatalf("expected sessionTarget normalised to \"main\" (official default), got %q", got)
	}
}

func TestSaveCronJobsFallsBackWhenConfiguredDefaultInvalid(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "ghost",
			"list": []interface{}{
				map[string]interface{}{"id": "work"},
			},
		},
	})

	r := gin.New()
	r.PUT("/system/cron", SaveCronJobs(cfg))
	body := []byte(`{"jobs":[{"id":"job_1","name":"fallback-target","enabled":true,"schedule":{"kind":"cron","expr":"0 9 * * *"},"sessionTarget":"","wakeMode":"now","payload":{"kind":"agentTurn","message":"hi"},"state":{},"createdAtMs":1}]}`)
	req := httptest.NewRequest(http.MethodPut, "/system/cron", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with fallback sessionTarget, got %d: %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(dir, "cron", "jobs.json"))
	if err != nil {
		t.Fatalf("read cron jobs: %v", err)
	}
	var saved map[string]interface{}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("decode cron jobs: %v", err)
	}
	jobs, _ := saved["jobs"].([]interface{})
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
	job, _ := jobs[0].(map[string]interface{})
	if got := strings.TrimSpace(getString(job, "sessionTarget")); got != "main" {
		t.Fatalf("expected sessionTarget normalised to \"main\" (official default), got %q", got)
	}
}

func writeJSON(t *testing.T, path string, data any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
