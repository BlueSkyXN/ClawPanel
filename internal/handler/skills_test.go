package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zhaoxinyi02/ClawPanel/internal/config"
)

func TestGetSkillsUsesWorkspacePrecedenceAndSkillEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	openClawDir := filepath.Join(root, "openclaw")
	openClawApp := filepath.Join(root, "app")
	workspace := filepath.Join(root, "workspaces", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawApp: openClawApp, OpenClawWork: workspace}

	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
		"plugins": map[string]interface{}{
			"entries": map[string]interface{}{
				"ghost-plugin":   map[string]interface{}{"enabled": false},
				"disabled-tools": map[string]interface{}{"enabled": false},
			},
			"installs": map[string]interface{}{
				"ghost-plugin": map[string]interface{}{"version": "0.1.0"},
			},
		},
		"skills": map[string]interface{}{
			"load": map[string]interface{}{
				"extraDirs": []interface{}{filepath.Join(root, "extra-skills")},
			},
			"entries": map[string]interface{}{
				"workspace-custom": map[string]interface{}{"enabled": false},
			},
			"blocklist": []interface{}{"legacy-blocked"},
		},
	})

	writeSkillFixture(t, filepath.Join(root, "extra-skills", "shared-skill"), "Shared From Extra", "extra copy", "")
	writeSkillFixture(t, filepath.Join(root, "app", "skills", "shared-skill"), "Shared From Bundled", "bundled copy", "")
	writeSkillFixture(t, filepath.Join(openClawDir, "skills", "legacy-blocked"), "Legacy Blocked", "managed blocked skill", "")
	writeSkillFixture(t, filepath.Join(home, ".agents", "skills", "shared-skill"), "Shared From Home", "personal agents copy", "")
	writeSkillFixture(t, filepath.Join(workspace, ".agents", "skills", "shared-skill"), "Shared From Project", "project agents copy", "")
	writeSkillFixture(t, filepath.Join(workspace, "skills", "shared-skill"), "Shared From Workspace", "workspace wins", "")
	writeSkillFixture(t, filepath.Join(workspace, "skills", "custom-skill"), "Workspace Custom", "workspace custom skill", `
metadata:
  openclaw:
    skillKey: workspace-custom
    requires:
      env:
        - OPENAI_API_KEY
`)
	writeJSON(t, filepath.Join(workspace, "skills", "custom-skill", "package.json"), map[string]interface{}{
		"name":    "workspace-custom",
		"version": "2.0.0",
	})
	writeSkillFixture(t, filepath.Join(workspace, "skills", "alias-skill-dir"), "Alias Skill", "alias metadata skill", `
metadata:
  clawdis:
    skillKey: alias-skill
    requires:
      env:
        - ALIAS_KEY
`)
	writeSkillFixture(t, filepath.Join(openClawApp, "extensions", "feishu-tools", "skills", "feishu-card"), "Feishu Card", "plugin skill", "")
	writeSkillFixture(t, filepath.Join(openClawApp, "extensions", "feishu-tools", "skills", "shared-skill"), "Plugin Shared", "plugin should not override workspace", "")
	writeJSON(t, filepath.Join(openClawApp, "extensions", "feishu-tools", "openclaw.plugin.json"), map[string]interface{}{
		"name":        "Feishu Tools",
		"description": "Plugin-provided skills",
		"skills":      []interface{}{"skills"},
	})
	writeSkillFixture(t, filepath.Join(openClawApp, "extensions", "disabled-tools", "skills", "disabled-skill"), "Disabled Plugin Skill", "should stay hidden", "")
	writeJSON(t, filepath.Join(openClawApp, "extensions", "disabled-tools", "openclaw.plugin.json"), map[string]interface{}{
		"name":        "Disabled Tools",
		"description": "Disabled plugin-provided skills",
		"skills":      []interface{}{"skills"},
	})
	writeSkillFixture(t, filepath.Join(openClawApp, "extensions", "escaped-skill"), "Escaped Skill", "should not be scanned", "")
	writeJSON(t, filepath.Join(openClawApp, "extensions", "bad-plugin", "openclaw.plugin.json"), map[string]interface{}{
		"name":        "Bad Plugin",
		"description": "Attempts to escape plugin root",
		"skills":      []interface{}{"../escaped-skill"},
	})

	r := gin.New()
	r.GET("/system/skills", GetSkills(cfg))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/system/skills?agentId=main", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK      bool         `json:"ok"`
		Skills  []skillInfo  `json:"skills"`
		Plugins []pluginInfo `json:"plugins"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok response")
	}

	shared := findSkillByID(resp.Skills, "shared-skill")
	if shared == nil {
		t.Fatalf("expected shared-skill in response")
	}
	if shared.Source != "workspace" {
		t.Fatalf("expected workspace source to win, got %q", shared.Source)
	}
	if shared.Description != "workspace wins" {
		t.Fatalf("expected workspace description to win, got %q", shared.Description)
	}

	custom := findSkillByID(resp.Skills, "custom-skill")
	if custom == nil {
		t.Fatalf("expected custom-skill in response")
	}
	if custom.SkillKey != "workspace-custom" {
		t.Fatalf("expected workspace-custom skillKey, got %q", custom.SkillKey)
	}
	if custom.Version != "2.0.0" {
		t.Fatalf("expected custom skill version 2.0.0, got %q", custom.Version)
	}
	if custom.Enabled {
		t.Fatalf("expected custom skill disabled by skills.entries override")
	}
	if env := asStringSlice(custom.Requires["env"]); len(env) != 1 || env[0] != "OPENAI_API_KEY" {
		t.Fatalf("expected OPENAI_API_KEY requirement, got %#v", custom.Requires)
	}
	aliasSkill := findSkillByID(resp.Skills, "alias-skill-dir")
	if aliasSkill == nil {
		t.Fatalf("expected alias-skill-dir in response")
	}
	if aliasSkill.SkillKey != "alias-skill" {
		t.Fatalf("expected clawdis alias skillKey alias-skill, got %q", aliasSkill.SkillKey)
	}
	if env := asStringSlice(aliasSkill.Requires["env"]); len(env) != 1 || env[0] != "ALIAS_KEY" {
		t.Fatalf("expected ALIAS_KEY requirement from clawdis metadata, got %#v", aliasSkill.Requires)
	}

	blocked := findSkillByID(resp.Skills, "legacy-blocked")
	if blocked == nil {
		t.Fatalf("expected legacy-blocked skill in response")
	}
	if blocked.Enabled {
		t.Fatalf("expected legacy-blocked disabled by blocklist fallback")
	}
	if findSkillByID(resp.Skills, "feishu-card") == nil {
		t.Fatalf("expected plugin-provided skill to be discovered from extensions/")
	}
	if findSkillByID(resp.Skills, "escaped-skill") != nil {
		t.Fatalf("expected plugin skill path escape to be ignored")
	}
	if findSkillByID(resp.Skills, "disabled-skill") != nil {
		t.Fatalf("expected disabled plugin skill to stay hidden")
	}
	if !hasPluginID(resp.Plugins, "feishu-tools") {
		t.Fatalf("expected plugin discovery from extensions/, got %#v", resp.Plugins)
	}
	if !hasPlugin(resp.Plugins, "ghost-plugin", "config", "0.1.0") {
		t.Fatalf("expected plugins.entries fallback item, got %#v", resp.Plugins)
	}
}

func TestToggleSkillWritesEntriesAndRemovesLegacyBlocklist(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{OpenClawDir: dir}
	writeJSON(t, filepath.Join(dir, "openclaw.json"), map[string]interface{}{
		"skills": map[string]interface{}{
			"blocklist": []interface{}{"translator-dir"},
		},
	})

	r := gin.New()
	r.PUT("/system/skills/:id/toggle", ToggleSkill(cfg))
	body := bytes.NewReader([]byte(`{"enabled":true,"aliases":["translator-dir"]}`))
	req := httptest.NewRequest(http.MethodPut, "/system/skills/translator/toggle", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	saved, err := cfg.ReadOpenClawJSON()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	skillsCfg := asMapAny(saved["skills"])
	entry := asMapAny(asMapAny(skillsCfg["entries"])["translator"])
	if enabled, _ := entry["enabled"].(bool); !enabled {
		t.Fatalf("expected skills.entries.translator.enabled=true, got %#v", entry)
	}
	if blocklist := asStringSlice(skillsCfg["blocklist"]); len(blocklist) != 0 {
		t.Fatalf("expected legacy blocklist entry removed, got %#v", blocklist)
	}
}

func TestSearchAndInstallClawHubUseOfficialAPIContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/search":
			if got := r.URL.Query().Get("q"); got != "weather" {
				t.Fatalf("expected q=weather, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{
					"slug":        "weather",
					"displayName": "Weather",
					"summary":     "Public weather skill",
					"version":     "1.2.0",
					"updatedAt":   123,
				}},
			})
		case r.URL.Path == "/api/v1/skills/weather":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"skill": map[string]interface{}{
					"slug":        "weather",
					"displayName": "Weather",
					"summary":     "Public weather skill",
					"moderation":  map[string]interface{}{"isMalwareBlocked": false},
				},
				"latestVersion": map[string]interface{}{"version": "1.2.0"},
			})
		case r.URL.Path == "/api/v1/download":
			if got := r.URL.Query().Get("slug"); got != "weather" {
				t.Fatalf("expected download slug weather, got %q", got)
			}
			if got := r.URL.Query().Get("version"); got != "1.2.0" {
				t.Fatalf("expected download version 1.2.0, got %q", got)
			}
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(buildZipFixture(t, map[string]string{
				"weather-1.2.0/SKILL.md":     "---\nname: Weather\ndescription: Public weather skill\n---\nUse weather data.\n",
				"weather-1.2.0/package.json": `{\"name\":\"weather\",\"description\":\"Public weather skill\"}`,
			}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	registryURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	registryURL.User = url.UserPassword("alice", "secret")
	t.Setenv("CLAWHUB_SITE", registryURL.String())

	r := gin.New()
	r.GET("/system/skills", GetSkills(cfg))
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	r.POST("/system/clawhub/install", InstallClawHubSkill(cfg))

	searchReq := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	searchW := httptest.NewRecorder()
	r.ServeHTTP(searchW, searchReq)
	if searchW.Code != http.StatusOK {
		t.Fatalf("search expected 200, got %d: %s", searchW.Code, searchW.Body.String())
	}
	var searchResp struct {
		OK           bool               `json:"ok"`
		RegistryBase string             `json:"registryBase"`
		Skills       []clawHubSkillItem `json:"skills"`
	}
	if err := json.Unmarshal(searchW.Body.Bytes(), &searchResp); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if len(searchResp.Skills) != 1 || searchResp.Skills[0].Installed {
		t.Fatalf("expected one not-installed search result, got %#v", searchResp.Skills)
	}
	if got := strings.TrimSpace(searchResp.RegistryBase); got != server.URL {
		t.Fatalf("expected public registry base %q, got %q", server.URL, got)
	}

	installReq := httptest.NewRequest(http.MethodPost, "/system/clawhub/install", bytes.NewReader([]byte(`{"skillId":"weather","agentId":"main"}`)))
	installReq.Header.Set("Content-Type", "application/json")
	installW := httptest.NewRecorder()
	r.ServeHTTP(installW, installReq)
	if installW.Code != http.StatusOK {
		t.Fatalf("install expected 200, got %d: %s", installW.Code, installW.Body.String())
	}

	lockRaw, err := os.ReadFile(filepath.Join(workspace, ".clawhub", "lock.json"))
	if err != nil {
		t.Fatalf("read lock.json: %v", err)
	}
	var lockPayload map[string]interface{}
	if err := json.Unmarshal(lockRaw, &lockPayload); err != nil {
		t.Fatalf("decode lock.json: %v", err)
	}
	if got := int(lockPayload["version"].(float64)); got != 1 {
		t.Fatalf("expected lock version 1, got %d", got)
	}
	skillsMap := asMapAny(lockPayload["skills"])
	weatherLock := asMapAny(skillsMap["weather"])
	if got := strings.TrimSpace(getString(weatherLock, "version")); got != "1.2.0" {
		t.Fatalf("expected lock version 1.2.0, got %q", got)
	}
	if _, ok := weatherLock["installedAt"].(float64); !ok {
		t.Fatalf("expected numeric installedAt, got %#v", weatherLock["installedAt"])
	}
	if _, err := os.Stat(filepath.Join(workspace, "skills", "weather", "SKILL.md")); err != nil {
		t.Fatalf("expected extracted skill files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "skills", "weather", "weather-1.2.0", "SKILL.md")); err == nil {
		t.Fatalf("expected wrapper directory to be flattened during extraction")
	}
	originRaw, err := os.ReadFile(filepath.Join(workspace, "skills", "weather", ".clawhub", "origin.json"))
	if err != nil {
		t.Fatalf("read origin.json: %v", err)
	}
	var originPayload map[string]interface{}
	if err := json.Unmarshal(originRaw, &originPayload); err != nil {
		t.Fatalf("decode origin.json: %v", err)
	}
	if got := strings.TrimSpace(getString(originPayload, "slug")); got != "weather" {
		t.Fatalf("expected origin slug weather, got %q", got)
	}
	if got := strings.TrimSpace(getString(originPayload, "registry")); got != server.URL {
		t.Fatalf("expected origin registry %q, got %q", server.URL, got)
	}

	searchAfterReq := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	searchAfterW := httptest.NewRecorder()
	r.ServeHTTP(searchAfterW, searchAfterReq)
	if searchAfterW.Code != http.StatusOK {
		t.Fatalf("search after install expected 200, got %d: %s", searchAfterW.Code, searchAfterW.Body.String())
	}
	var searchAfterResp struct {
		OK           bool               `json:"ok"`
		RegistryBase string             `json:"registryBase"`
		Skills       []clawHubSkillItem `json:"skills"`
	}
	if err := json.Unmarshal(searchAfterW.Body.Bytes(), &searchAfterResp); err != nil {
		t.Fatalf("decode search-after response: %v", err)
	}
	if len(searchAfterResp.Skills) != 1 || !searchAfterResp.Skills[0].Installed || searchAfterResp.Skills[0].InstalledVersion != "1.2.0" {
		t.Fatalf("expected installed search result after install, got %#v", searchAfterResp.Skills)
	}

	skillsReq := httptest.NewRequest(http.MethodGet, "/system/skills?agentId=main", nil)
	skillsW := httptest.NewRecorder()
	r.ServeHTTP(skillsW, skillsReq)
	if skillsW.Code != http.StatusOK {
		t.Fatalf("skills expected 200, got %d: %s", skillsW.Code, skillsW.Body.String())
	}
	var skillsResp struct {
		Skills []skillInfo `json:"skills"`
	}
	if err := json.Unmarshal(skillsW.Body.Bytes(), &skillsResp); err != nil {
		t.Fatalf("decode skills response: %v", err)
	}
	localWeather := findSkillByID(skillsResp.Skills, "weather")
	if localWeather == nil || localWeather.Version != "1.2.0" {
		t.Fatalf("expected local installed skill version 1.2.0, got %#v", localWeather)
	}
}

func TestSearchClawHubMarksExistingSkillDirectoryInstalled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	writeSkillFixture(t, filepath.Join(workspace, "skills", "weather"), "Weather", "manual install", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{
				"slug":        "weather",
				"displayName": "Weather",
				"summary":     "Public weather skill",
				"version":     "1.2.0",
			}},
		})
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	req := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Skills []clawHubSkillItem `json:"skills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Skills) != 1 || !resp.Skills[0].Installed {
		t.Fatalf("expected existing skill dir to be marked installed, got %#v", resp.Skills)
	}
}

func TestSearchClawHubUsesSiteForPublicLinksWhenRegistryOverrideSet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{
				"slug":        "weather",
				"displayName": "Weather",
				"summary":     "Public weather skill",
				"version":     "1.2.0",
			}},
		})
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_REGISTRY", server.URL)
	t.Setenv("CLAWHUB_SITE", "https://clawhub.example.com")

	r := gin.New()
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	req := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		RegistryBase string             `json:"registryBase"`
		Skills       []clawHubSkillItem `json:"skills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := strings.TrimSpace(resp.RegistryBase); got != "https://clawhub.example.com" {
		t.Fatalf("expected public site base, got %q", got)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("expected one search result, got %#v", resp.Skills)
	}
}

func TestSearchClawHubRejectsInvalidRegistryScheme(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	t.Setenv("CLAWHUB_SITE", "javascript:alert(1)")

	r := gin.New()
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	req := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for invalid registry scheme, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSearchClawHubDoesNotExposeRegistryCredentialsOnTransportError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	t.Setenv("CLAWHUB_SITE", "https://alice:secret@example.com")
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}),
	}
	defer func() { clawHubHTTPClient = oldClient }()

	r := gin.New()
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	req := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for transport failure, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "alice") || strings.Contains(body, "secret") {
		t.Fatalf("expected response body to redact registry credentials, got %s", body)
	}
	if !strings.Contains(body, "request failed") {
		t.Fatalf("expected sanitized transport error message, got %s", body)
	}
}

func TestSearchClawHubDoesNotMarkManagedSkillInstalled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	writeSkillFixture(t, filepath.Join(openClawDir, "skills", "weather"), "Weather", "managed install", "")
	writeJSON(t, filepath.Join(openClawDir, "skills", "weather", "package.json"), map[string]interface{}{
		"name":    "weather",
		"version": "9.9.9",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{
				"slug":        "weather",
				"displayName": "Weather",
				"summary":     "Public weather skill",
				"version":     "1.2.0",
			}},
		})
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	req := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Skills []clawHubSkillItem `json:"skills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Skills) != 1 || resp.Skills[0].Installed || resp.Skills[0].InstalledVersion != "" {
		t.Fatalf("expected managed skill to remain installable in workspace, got %#v", resp.Skills)
	}
}

func TestSearchClawHubIgnoresStaleLockWithoutSkillDirectory(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	writeJSON(t, filepath.Join(workspace, ".clawhub", "lock.json"), map[string]interface{}{
		"version": 1,
		"skills": map[string]interface{}{
			"weather": map[string]interface{}{"version": "0.0.1", "installedAt": 1},
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{
				"slug":        "weather",
				"displayName": "Weather",
				"summary":     "Public weather skill",
				"version":     "1.2.0",
			}},
		})
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	req := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Skills []clawHubSkillItem `json:"skills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Skills) != 1 || resp.Skills[0].Installed || resp.Skills[0].InstalledVersion != "" {
		t.Fatalf("expected stale lock without skill dir to be ignored, got %#v", resp.Skills)
	}
}

func TestSearchClawHubIgnoresManagedSkillWhenOnlyStaleLockExists(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	writeSkillFixture(t, filepath.Join(openClawDir, "skills", "weather"), "Weather", "managed install", "")
	writeJSON(t, filepath.Join(openClawDir, "skills", "weather", "package.json"), map[string]interface{}{
		"name":    "weather",
		"version": "9.9.9",
	})
	writeJSON(t, filepath.Join(workspace, ".clawhub", "lock.json"), map[string]interface{}{
		"version": 1,
		"skills": map[string]interface{}{
			"weather": map[string]interface{}{"version": "0.0.1", "installedAt": 1},
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{
				"slug":        "weather",
				"displayName": "Weather",
				"summary":     "Public weather skill",
				"version":     "1.2.0",
			}},
		})
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	req := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Skills []clawHubSkillItem `json:"skills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Skills) != 1 || resp.Skills[0].Installed || resp.Skills[0].InstalledVersion != "" {
		t.Fatalf("expected stale workspace lock plus managed skill to remain not installed, got %#v", resp.Skills)
	}
}

func TestSkillsHandlersRejectInvalidAgentID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := resolvedTempDir(t)
	cfg := &config.Config{OpenClawDir: filepath.Join(dir, "openclaw"), OpenClawWork: filepath.Join(dir, "workspace", "main")}
	writeJSON(t, filepath.Join(cfg.OpenClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": filepath.Join(dir, "workspace", "main")},
			},
		},
	})

	r := gin.New()
	r.GET("/system/skills", GetSkills(cfg))
	r.GET("/system/clawhub/search", SearchClawHub(cfg))

	for _, path := range []string{
		"/system/skills?agentId=../../etc",
		"/system/clawhub/search?agentId=../../etc",
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d: %s", path, w.Code, w.Body.String())
		}
	}
}

func TestInstallClawHubSkillRejectsInvalidSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := resolvedTempDir(t)
	cfg := &config.Config{OpenClawDir: filepath.Join(dir, "openclaw"), OpenClawWork: filepath.Join(dir, "workspace", "main")}
	writeJSON(t, filepath.Join(cfg.OpenClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": filepath.Join(dir, "workspace", "main")},
			},
		},
	})

	r := gin.New()
	r.POST("/system/clawhub/install", InstallClawHubSkill(cfg))
	req := httptest.NewRequest(http.MethodPost, "/system/clawhub/install", bytes.NewReader([]byte(`{"skillId":".","agentId":"main"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid skill slug, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstallClawHubSkillAllowsWorkspaceOverrideOfManagedSkill(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	writeSkillFixture(t, filepath.Join(openClawDir, "skills", "weather"), "Weather", "managed install", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/weather":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"skill": map[string]interface{}{
					"slug":        "weather",
					"displayName": "Weather",
					"summary":     "Public weather skill",
					"moderation":  map[string]interface{}{"isMalwareBlocked": false},
				},
				"latestVersion": map[string]interface{}{"version": "1.2.0"},
			})
		case "/api/v1/download":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(buildZipFixture(t, map[string]string{
				"SKILL.md":     "---\nname: Weather\ndescription: Public weather skill\n---\nUse weather data.\n",
				"package.json": `{"name":"weather","description":"Public weather skill"}`,
			}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.POST("/system/clawhub/install", InstallClawHubSkill(cfg))
	req := httptest.NewRequest(http.MethodPost, "/system/clawhub/install", bytes.NewReader([]byte(`{"skillId":"weather","agentId":"main"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for workspace override of managed skill, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "skills", "weather", "SKILL.md")); err != nil {
		t.Fatalf("expected workspace-local skill install, got %v", err)
	}
}

func TestInstallClawHubSkillRejectsSymlinkedSkillsRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink handling differs on Windows")
	}
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	outside := filepath.Join(root, "outside")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "skills")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/weather":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"skill": map[string]interface{}{
					"slug":        "weather",
					"displayName": "Weather",
					"summary":     "Public weather skill",
					"moderation":  map[string]interface{}{"isMalwareBlocked": false},
				},
				"latestVersion": map[string]interface{}{"version": "1.2.0"},
			})
		case "/api/v1/download":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(buildZipFixture(t, map[string]string{
				"SKILL.md":     "---\nname: Weather\ndescription: Public weather skill\n---\nUse weather data.\n",
				"package.json": `{"name":"weather","description":"Public weather skill"}`,
			}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.POST("/system/clawhub/install", InstallClawHubSkill(cfg))
	req := httptest.NewRequest(http.MethodPost, "/system/clawhub/install", bytes.NewReader([]byte(`{"skillId":"weather","agentId":"main"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for symlinked skills root, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "weather", "SKILL.md")); err == nil {
		t.Fatalf("expected install to avoid writing through symlinked skills root")
	}
}

func TestSearchClawHubRejectsSymlinkedSkillsRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink handling differs on Windows")
	}
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	outside := filepath.Join(root, "outside")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(outside, "weather"), 0755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeSkillFixture(t, filepath.Join(outside, "weather"), "Weather", "outside skill", "")
	if err := os.Symlink(outside, filepath.Join(workspace, "skills")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{
				"slug":        "weather",
				"displayName": "Weather",
				"summary":     "Public weather skill",
				"version":     "1.2.0",
			}},
		})
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	req := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for symlinked skills root, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstallClawHubSkillRejectsSymlinkedClawHubStateDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink handling differs on Windows")
	}
	gin.SetMode(gin.TestMode)

	root := resolvedTempDir(t)
	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	outside := filepath.Join(root, "outside-state")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(workspace, "skills"), 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("mkdir outside state: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, ".clawhub")); err != nil {
		t.Fatalf("create .clawhub symlink: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/weather":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"skill": map[string]interface{}{
					"slug":        "weather",
					"displayName": "Weather",
					"summary":     "Public weather skill",
					"moderation":  map[string]interface{}{"isMalwareBlocked": false},
				},
				"latestVersion": map[string]interface{}{"version": "1.2.0"},
			})
		case "/api/v1/download":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(buildZipFixture(t, map[string]string{
				"SKILL.md":     "---\nname: Weather\ndescription: Public weather skill\n---\nUse weather data.\n",
				"package.json": `{"name":"weather","description":"Public weather skill"}`,
			}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.POST("/system/clawhub/install", InstallClawHubSkill(cfg))
	req := httptest.NewRequest(http.MethodPost, "/system/clawhub/install", bytes.NewReader([]byte(`{"skillId":"weather","agentId":"main"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for symlinked .clawhub dir, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "lock.json")); err == nil {
		t.Fatalf("expected install to avoid writing through symlinked .clawhub dir")
	}
}

func TestClawHubHandlersRejectSymlinkedWorkspaceAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink handling differs on Windows")
	}
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	openClawDir := filepath.Join(root, "openclaw")
	managedRoot := filepath.Join(root, "managed", "workspaces")
	realRoot := filepath.Join(root, "real-workspaces")
	workspace := filepath.Join(managedRoot, "shared", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})
	if err := os.MkdirAll(filepath.Join(realRoot, "shared"), 0755); err != nil {
		t.Fatalf("mkdir real root: %v", err)
	}
	if err := os.MkdirAll(managedRoot, 0755); err != nil {
		t.Fatalf("mkdir managed root: %v", err)
	}
	if err := os.Symlink(filepath.Join(realRoot, "shared"), filepath.Join(managedRoot, "shared")); err != nil {
		t.Fatalf("create ancestor symlink: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/weather":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"skill": map[string]interface{}{
					"slug":        "weather",
					"displayName": "Weather",
					"summary":     "Public weather skill",
					"moderation":  map[string]interface{}{"isMalwareBlocked": false},
				},
				"latestVersion": map[string]interface{}{"version": "1.2.0"},
			})
		case "/api/v1/download":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(buildZipFixture(t, map[string]string{
				"SKILL.md":     "---\nname: Weather\ndescription: Public weather skill\n---\nUse weather data.\n",
				"package.json": `{"name":"weather","description":"Public weather skill"}`,
			}))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.POST("/system/clawhub/install", InstallClawHubSkill(cfg))

	req := httptest.NewRequest(http.MethodPost, "/system/clawhub/install", bytes.NewReader([]byte(`{"skillId":"weather","agentId":"main"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for workspace ancestor symlink, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(realRoot, "shared", "main", "skills", "weather", "SKILL.md")); err == nil {
		t.Fatalf("expected install to avoid writing through symlinked workspace ancestor")
	}
}

func TestSearchClawHubAllowsTrustedTempDirAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil || resolvedRoot == "" || resolvedRoot == root {
		t.Skip("temp dir does not use a trusted system alias on this platform")
	}

	openClawDir := filepath.Join(root, "openclaw")
	workspace := filepath.Join(root, "workspace", "main")
	cfg := &config.Config{OpenClawDir: openClawDir, OpenClawWork: workspace}
	writeJSON(t, filepath.Join(openClawDir, "openclaw.json"), map[string]interface{}{
		"agents": map[string]interface{}{
			"default": "main",
			"list": []interface{}{
				map[string]interface{}{"id": "main", "workspace": workspace},
			},
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{{
				"slug":        "weather",
				"displayName": "Weather",
				"summary":     "Public weather skill",
				"version":     "1.2.0",
			}},
		})
	}))
	defer server.Close()
	oldClient := clawHubHTTPClient
	clawHubHTTPClient = server.Client()
	defer func() { clawHubHTTPClient = oldClient }()
	t.Setenv("CLAWHUB_SITE", server.URL)

	r := gin.New()
	r.GET("/system/clawhub/search", SearchClawHub(cfg))
	req := httptest.NewRequest(http.MethodGet, "/system/clawhub/search?q=weather&agentId=main", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for trusted temp alias workspace, got %d: %s", w.Code, w.Body.String())
	}
}

func writeSkillFixture(t *testing.T, dir, name, description, extraFrontmatter string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	frontmatter := strings.TrimSpace(extraFrontmatter)
	if frontmatter != "" {
		frontmatter = "\n" + frontmatter
	}
	content := "---\nname: " + name + "\ndescription: " + description + frontmatter + "\n---\n" + description + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func buildZipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func findSkillByID(skills []skillInfo, id string) *skillInfo {
	for i := range skills {
		if skills[i].ID == id {
			return &skills[i]
		}
	}
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func hasPluginID(plugins []pluginInfo, id string) bool {
	for _, plugin := range plugins {
		if plugin.ID == id {
			return true
		}
	}
	return false
}

func hasPlugin(plugins []pluginInfo, id, source, version string) bool {
	for _, plugin := range plugins {
		if plugin.ID == id && plugin.Source == source && plugin.Version == version {
			return true
		}
	}
	return false
}

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if real, err := filepath.EvalSymlinks(dir); err == nil && real != "" {
		return real
	}
	return dir
}
