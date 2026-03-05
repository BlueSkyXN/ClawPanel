package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
				"name":    "work-group",
				"enabled": true,
				"agentId": "work",
				"match": map[string]interface{}{
					"channel": "qq",
					"peer":    "group:*",
				},
			},
			map[string]interface{}{
				"name":    "fallback-qq",
				"enabled": true,
				"agentId": "main",
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
			Agent     string   `json:"agent"`
			MatchedBy string   `json:"matchedBy"`
			Trace     []string `json:"trace"`
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
	if !strings.HasPrefix(resp.Result.MatchedBy, "bindings[0]") {
		t.Fatalf("expected matchedBy to reference first binding, got %s", resp.Result.MatchedBy)
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
				"enabled": true,
				"match": map[string]interface{}{
					"channel": "qq",
				},
			},
			map[string]interface{}{
				"agentId": "work",
				"enabled": true,
				"match": map[string]interface{}{
					"channel": "qq",
					"peer":    "group:*",
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
			Agent     string `json:"agent"`
			MatchedBy string `json:"matchedBy"`
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
	body := []byte(`{"bindings":[{"agentId":"main","enabled":true,"match":{"peer":"group:*"}}]}`)
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
	body := []byte(`{"bindings":[{"agentId":"main","enabled":true,"match":{"channel":[1]}}]}`)
	req := httptest.NewRequest(http.MethodPut, "/openclaw/bindings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when match array contains non-string item, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "仅支持字符串") {
		t.Fatalf("error should mention string-only array items, got: %s", w.Body.String())
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
	if got := strings.TrimSpace(getString(job, "sessionTarget")); got != "work" {
		t.Fatalf("expected sessionTarget filled with default work, got %q", got)
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
	if got := strings.TrimSpace(getString(job, "sessionTarget")); got != "work" {
		t.Fatalf("expected sessionTarget fallback to work, got %q", got)
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
