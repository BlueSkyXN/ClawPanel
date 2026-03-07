package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zhaoxinyi02/ClawPanel/internal/config"
)

const (
	// RegistryURL is the official plugin registry
	RegistryURL = "https://raw.githubusercontent.com/zhaoxinyi02/ClawPanel-Plugins/main/registry.json"
	// RegistryMirrorURL is the China mirror
	RegistryMirrorURL = "http://39.102.53.188:16198/clawpanel/plugins/registry.json"
)

// PluginMeta represents a plugin's metadata (plugin.json)
type PluginMeta struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Author       string            `json:"author"`
	Description  string            `json:"description"`
	Homepage     string            `json:"homepage,omitempty"`
	Repository   string            `json:"repository,omitempty"`
	License      string            `json:"license,omitempty"`
	Category     string            `json:"category,omitempty"` // basic, ai, message, fun, tool
	Tags         []string          `json:"tags,omitempty"`
	Icon         string            `json:"icon,omitempty"`
	MinOpenClaw  string            `json:"minOpenClaw,omitempty"`
	MinPanel     string            `json:"minPanel,omitempty"`
	EntryPoint   string            `json:"entryPoint,omitempty"`   // main script file
	ConfigSchema json.RawMessage   `json:"configSchema,omitempty"` // JSON Schema for config
	Dependencies map[string]string `json:"dependencies,omitempty"`
	Permissions  []string          `json:"permissions,omitempty"`
}

// InstalledPlugin represents a plugin installed on disk
type InstalledPlugin struct {
	PluginMeta
	Enabled     bool                   `json:"enabled"`
	InstalledAt string                 `json:"installedAt"`
	UpdatedAt   string                 `json:"updatedAt,omitempty"`
	Source      string                 `json:"source"` // registry, local, github
	Dir         string                 `json:"dir"`
	Config      map[string]interface{} `json:"config,omitempty"`
	LogLines    []string               `json:"logLines,omitempty"`
}

// RegistryPlugin represents a plugin in the registry
type RegistryPlugin struct {
	PluginMeta
	Downloads     int    `json:"downloads,omitempty"`
	Stars         int    `json:"stars,omitempty"`
	DownloadURL   string `json:"downloadUrl,omitempty"`
	GitURL        string `json:"gitUrl,omitempty"`
	InstallSubDir string `json:"installSubDir,omitempty"` // subdirectory within git repo to install
	NpmPackage    string `json:"npmPackage,omitempty"`    // npm package name e.g. @openclaw/feishu
	Screenshot    string `json:"screenshot,omitempty"`
	Readme        string `json:"readme,omitempty"`
}

// Registry represents the plugin registry
type Registry struct {
	Version   string           `json:"version"`
	UpdatedAt string           `json:"updatedAt"`
	Plugins   []RegistryPlugin `json:"plugins"`
}

// Manager handles plugin lifecycle
type Manager struct {
	cfg        *config.Config
	plugins    map[string]*InstalledPlugin
	registry   *Registry
	mu         sync.RWMutex
	pluginsDir string
	configFile string
}

// NewManager creates a plugin manager
func NewManager(cfg *config.Config) *Manager {
	pluginsDir := filepath.Join(cfg.OpenClawDir, "extensions")
	if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
		os.MkdirAll(pluginsDir, 0755)
	}
	m := &Manager{
		cfg:        cfg,
		plugins:    make(map[string]*InstalledPlugin),
		pluginsDir: pluginsDir,
		configFile: filepath.Join(cfg.DataDir, "plugins.json"),
	}
	m.loadPluginsState()
	m.scanInstalledPlugins()
	return m
}

// GetPluginsDir returns the plugins directory
func (m *Manager) GetPluginsDir() string {
	return m.pluginsDir
}

// ListInstalled returns all installed plugins
func (m *Manager) ListInstalled() []*InstalledPlugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*InstalledPlugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		result = append(result, p)
	}
	return result
}

// GetPlugin returns a specific installed plugin
func (m *Manager) GetPlugin(id string) *InstalledPlugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.plugins[id]
}

// FetchRegistry fetches the plugin registry from server
func (m *Manager) FetchRegistry() (*Registry, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	bundled := m.loadBundledRegistry()

	// Try mirror first (faster in China), then GitHub
	urls := []string{RegistryMirrorURL, RegistryURL}
	var lastErr error
	for _, url := range urls {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
			continue
		}
		var reg Registry
		if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
			lastErr = fmt.Errorf("parse registry: %v", err)
			continue
		}
		reg = mergeRegistries(reg, bundled)
		m.mu.Lock()
		m.registry = &reg
		m.mu.Unlock()
		// Cache to disk
		m.cacheRegistry(&reg)
		return &reg, nil
	}

	if bundled != nil {
		m.mu.Lock()
		m.registry = bundled
		m.mu.Unlock()
		m.cacheRegistry(bundled)
		return bundled, nil
	}

	// Try cached registry
	if cached := m.loadCachedRegistry(); cached != nil {
		merged := mergeRegistries(*cached, bundled)
		return &merged, nil
	}

	return nil, fmt.Errorf("获取插件仓库失败: %v", lastErr)
}

// GetRegistry returns the cached registry or fetches it
func (m *Manager) GetRegistry() *Registry {
	m.mu.RLock()
	reg := m.registry
	m.mu.RUnlock()
	if reg != nil {
		return reg
	}
	if cached := m.loadCachedRegistry(); cached != nil {
		bundled := m.loadBundledRegistry()
		merged := mergeRegistries(*cached, bundled)
		m.mu.Lock()
		m.registry = &merged
		m.mu.Unlock()
		return &merged
	}
	if bundled := m.loadBundledRegistry(); bundled != nil {
		m.mu.Lock()
		m.registry = bundled
		m.mu.Unlock()
		return bundled
	}
	return &Registry{Plugins: []RegistryPlugin{}}
}

type pluginInstallStrategy struct {
	kind   string
	target string
}

func resolvePluginInstallStrategy(regPlugin *RegistryPlugin, source string) pluginInstallStrategy {
	source = strings.TrimSpace(source)
	if source != "" {
		if strings.HasPrefix(source, "@") || (!strings.Contains(source, "/") && !strings.HasSuffix(source, ".git") && source != "") {
			return pluginInstallStrategy{kind: "npm", target: source}
		}
		return pluginInstallStrategy{kind: "download", target: source}
	}
	if regPlugin == nil {
		return pluginInstallStrategy{}
	}
	if regPlugin.DownloadURL != "" {
		return pluginInstallStrategy{kind: "download", target: regPlugin.DownloadURL}
	}
	if regPlugin.GitURL != "" {
		return pluginInstallStrategy{kind: "download", target: regPlugin.GitURL}
	}
	if regPlugin.NpmPackage != "" {
		return pluginInstallStrategy{kind: "npm", target: regPlugin.NpmPackage}
	}
	return pluginInstallStrategy{}
}

// Install installs a plugin from registry or URL
func (m *Manager) Install(pluginID string, source string) error {
	// Find plugin in registry
	reg := m.GetRegistry()
	var regPlugin *RegistryPlugin
	for i := range reg.Plugins {
		if reg.Plugins[i].ID == pluginID {
			regPlugin = &reg.Plugins[i]
			break
		}
	}

	if regPlugin == nil && source == "" {
		return fmt.Errorf("插件 %s 不在仓库中，请提供安装源", pluginID)
	}

	strategy := resolvePluginInstallStrategy(regPlugin, source)
	if strategy.kind == "npm" {
		npmPkg := strategy.target
		// Install via npm global
		if err := m.installFromNpm(npmPkg); err != nil {
			return fmt.Errorf("npm 安装失败: %v", err)
		}
		// Find where npm installed it
		npmRoot := ""
		if out, err := exec.Command("npm", "root", "-g").Output(); err == nil {
			npmRoot = strings.TrimSpace(string(out))
		}
		pkgName := npmPkg
		if idx := strings.LastIndex(pkgName, "/"); idx >= 0 {
			pkgName = pkgName[idx+1:]
		}
		installedDir := ""
		if npmRoot != "" {
			// For scoped packages like @openclaw/feishu, dir is @openclaw/feishu
			installedDir = filepath.Join(npmRoot, npmPkg)
			if _, err := os.Stat(installedDir); err != nil {
				installedDir = filepath.Join(npmRoot, pkgName)
			}
		}
		meta := &PluginMeta{ID: pluginID, Name: pluginID}
		if regPlugin != nil {
			meta = &regPlugin.PluginMeta
		}
		if installedDir == "" {
			installedDir = npmPkg
		}
		installed := &InstalledPlugin{
			PluginMeta:  *meta,
			Enabled:     true,
			InstalledAt: time.Now().Format(time.RFC3339),
			Source:      "npm",
			Dir:         installedDir,
		}
		m.mu.Lock()
		m.plugins[meta.ID] = installed
		m.mu.Unlock()
		m.savePluginsState()
		if err := m.syncOpenClawPluginState(meta.ID, installedDir, installed.Enabled, installed.Source, meta.Version); err != nil {
			return err
		}
		return nil
	}

	// Determine download URL (git/archive)
	downloadURL := strategy.target

	if downloadURL == "" {
		return fmt.Errorf("无法确定插件 %s 的安装方式，请提供 npm 包名或下载地址", pluginID)
	}

	pluginDir := filepath.Join(m.pluginsDir, pluginID)

	// Check if already installed
	if _, err := os.Stat(pluginDir); err == nil {
		return fmt.Errorf("插件 %s 已安装，请先卸载或使用更新功能", pluginID)
	}

	// Determine installSubDir from registry
	installSubDir := ""
	if regPlugin != nil {
		installSubDir = regPlugin.InstallSubDir
	}

	// Install based on source type
	if strings.HasSuffix(downloadURL, ".git") || strings.Contains(downloadURL, "github.com") || strings.Contains(downloadURL, "gitee.com") {
		if installSubDir != "" {
			// Clone full repo to temp dir, then copy subdirectory
			tmpDir, err := os.MkdirTemp("", "clawpanel-plugin-*")
			if err != nil {
				return fmt.Errorf("创建临时目录失败: %v", err)
			}
			defer os.RemoveAll(tmpDir)
			if err := m.installFromGit(downloadURL, tmpDir); err != nil {
				return fmt.Errorf("Git 安装失败: %v", err)
			}
			subPath := filepath.Join(tmpDir, filepath.FromSlash(installSubDir))
			if _, err := os.Stat(subPath); err != nil {
				return fmt.Errorf("子目录 %s 在仓库中不存在", installSubDir)
			}
			if err := copyDir(subPath, pluginDir); err != nil {
				os.RemoveAll(pluginDir)
				return fmt.Errorf("复制插件目录失败: %v", err)
			}
		} else {
			if err := m.installFromGit(downloadURL, pluginDir); err != nil {
				os.RemoveAll(pluginDir)
				return fmt.Errorf("Git 安装失败: %v", err)
			}
		}
	} else if strings.HasSuffix(downloadURL, ".zip") || strings.HasSuffix(downloadURL, ".tar.gz") {
		// Download archive
		if err := m.installFromArchive(downloadURL, pluginDir); err != nil {
			os.RemoveAll(pluginDir)
			return fmt.Errorf("下载安装失败: %v", err)
		}
	} else {
		// Try git clone as fallback
		if err := m.installFromGit(downloadURL, pluginDir); err != nil {
			os.RemoveAll(pluginDir)
			return fmt.Errorf("安装失败: %v", err)
		}
	}

	// Read plugin metadata
	meta, err := m.readPluginMeta(pluginDir)
	if err != nil {
		// If no plugin.json, create a minimal one
		meta = &PluginMeta{
			ID:   pluginID,
			Name: pluginID,
		}
		if regPlugin != nil {
			meta = &regPlugin.PluginMeta
		}
	}

	// Install npm dependencies if package.json exists
	if _, err := os.Stat(filepath.Join(pluginDir, "package.json")); err == nil {
		cmd := exec.Command("npm", "install", "--production", "--registry=https://registry.npmmirror.com")
		cmd.Dir = pluginDir
		cmd.Run()
	}

	// Register installed plugin
	installed := &InstalledPlugin{
		PluginMeta:  *meta,
		Enabled:     true,
		InstalledAt: time.Now().Format(time.RFC3339),
		Source:      "registry",
		Dir:         pluginDir,
	}
	if source != "" {
		installed.Source = "custom"
	}

	m.mu.Lock()
	m.plugins[meta.ID] = installed
	m.mu.Unlock()
	m.savePluginsState()
	if err := m.syncOpenClawPluginState(meta.ID, pluginDir, installed.Enabled, installed.Source, meta.Version); err != nil {
		return err
	}

	return nil
}

// InstallLocal installs a plugin from a local directory
func (m *Manager) InstallLocal(srcDir string) error {
	meta, err := m.readPluginMeta(srcDir)
	if err != nil {
		return fmt.Errorf("读取插件信息失败: %v", err)
	}

	pluginDir := filepath.Join(m.pluginsDir, meta.ID)
	if _, err := os.Stat(pluginDir); err == nil {
		return fmt.Errorf("插件 %s 已安装", meta.ID)
	}

	// Copy directory
	if err := copyDir(srcDir, pluginDir); err != nil {
		return fmt.Errorf("复制插件失败: %v", err)
	}

	installed := &InstalledPlugin{
		PluginMeta:  *meta,
		Enabled:     true,
		InstalledAt: time.Now().Format(time.RFC3339),
		Source:      "local",
		Dir:         pluginDir,
	}

	m.mu.Lock()
	m.plugins[meta.ID] = installed
	m.mu.Unlock()
	m.savePluginsState()

	return nil
}

// Uninstall removes a plugin
func (m *Manager) Uninstall(pluginID string) error {
	m.mu.Lock()
	p, ok := m.plugins[pluginID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("插件 %s 未安装", pluginID)
	}
	delete(m.plugins, pluginID)
	m.mu.Unlock()

	// Remove plugin directory
	if p.Dir != "" {
		os.RemoveAll(p.Dir)
	}

	m.savePluginsState()
	if err := m.removeOpenClawPluginState(pluginID); err != nil {
		return err
	}
	return nil
}

// Enable enables a plugin
func (m *Manager) Enable(pluginID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.plugins[pluginID]
	if !ok {
		return fmt.Errorf("插件 %s 未安装", pluginID)
	}
	p.Enabled = true
	m.savePluginsStateUnlocked()
	if err := m.syncOpenClawPluginState(p.ID, p.Dir, true, p.Source, p.Version); err != nil {
		return err
	}
	return nil
}

// Disable disables a plugin
func (m *Manager) Disable(pluginID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.plugins[pluginID]
	if !ok {
		return fmt.Errorf("插件 %s 未安装", pluginID)
	}
	p.Enabled = false
	m.savePluginsStateUnlocked()
	if err := m.syncOpenClawPluginState(p.ID, p.Dir, false, p.Source, p.Version); err != nil {
		return err
	}
	return nil
}

// UpdateConfig updates a plugin's configuration
func (m *Manager) UpdateConfig(pluginID string, cfg map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.plugins[pluginID]
	if !ok {
		return fmt.Errorf("插件 %s 未安装", pluginID)
	}
	p.Config = cfg

	// Also write config.json to plugin directory
	configPath := filepath.Join(p.Dir, "config.json")
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath, data, 0644)

	m.savePluginsStateUnlocked()
	return nil
}

// GetConfig returns a plugin's configuration
func (m *Manager) GetConfig(pluginID string) (map[string]interface{}, json.RawMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[pluginID]
	if !ok {
		return nil, nil, fmt.Errorf("插件 %s 未安装", pluginID)
	}

	// Try to read config from plugin dir
	cfg := p.Config
	if cfg == nil {
		configPath := filepath.Join(p.Dir, "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			json.Unmarshal(data, &cfg)
		}
	}
	if cfg == nil {
		cfg = map[string]interface{}{}
	}

	return cfg, p.ConfigSchema, nil
}

// GetPluginLogs returns recent log lines for a plugin
func (m *Manager) GetPluginLogs(pluginID string) ([]string, error) {
	m.mu.RLock()
	p, ok := m.plugins[pluginID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("插件 %s 未安装", pluginID)
	}

	// Read log file if exists
	logPath := filepath.Join(p.Dir, "plugin.log")
	if data, err := os.ReadFile(logPath); err == nil {
		lines := strings.Split(string(data), "\n")
		if len(lines) > 200 {
			lines = lines[len(lines)-200:]
		}
		return lines, nil
	}

	return p.LogLines, nil
}

// Update updates a plugin to the latest version
func (m *Manager) Update(pluginID string) error {
	m.mu.RLock()
	p, ok := m.plugins[pluginID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("插件 %s 未安装", pluginID)
	}

	if p.Dir == "" {
		return fmt.Errorf("插件目录未知")
	}

	// If it's a git repo, do git pull
	gitDir := filepath.Join(p.Dir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		cmd := exec.Command("git", "pull", "--rebase")
		cmd.Dir = p.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git pull 失败: %s %v", string(out), err)
		}

		// Re-read metadata
		if meta, err := m.readPluginMeta(p.Dir); err == nil {
			m.mu.Lock()
			p.PluginMeta = *meta
			p.UpdatedAt = time.Now().Format(time.RFC3339)
			m.mu.Unlock()
			m.savePluginsState()
		}

		// Re-install npm deps
		if _, err := os.Stat(filepath.Join(p.Dir, "package.json")); err == nil {
			cmd := exec.Command("npm", "install", "--production", "--registry=https://registry.npmmirror.com")
			cmd.Dir = p.Dir
			cmd.Run()
		}

		return nil
	}

	// Otherwise, uninstall and reinstall from registry
	source := p.Source
	if err := m.Uninstall(pluginID); err != nil {
		return err
	}
	if source == "registry" {
		return m.Install(pluginID, "")
	}
	return fmt.Errorf("非 Git 仓库插件无法自动更新，请手动卸载重装")
}

// CheckConflicts checks for potential conflicts before installing
func (m *Manager) CheckConflicts(pluginID string) []string {
	var conflicts []string
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.plugins[pluginID]; exists {
		conflicts = append(conflicts, fmt.Sprintf("插件 %s 已安装", pluginID))
	}

	return conflicts
}

// --- Internal methods ---

func (m *Manager) scanInstalledPlugins() {
	entries, err := os.ReadDir(m.pluginsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginDir := filepath.Join(m.pluginsDir, entry.Name())
		meta, err := m.readPluginMeta(pluginDir)
		if err != nil {
			continue
		}

		enabled := true
		source := "local"
		version := meta.Version
		m.mu.Lock()
		if _, exists := m.plugins[meta.ID]; !exists {
			m.plugins[meta.ID] = &InstalledPlugin{
				PluginMeta:  *meta,
				Enabled:     true,
				InstalledAt: time.Now().Format(time.RFC3339),
				Source:      "local",
				Dir:         pluginDir,
			}
		} else {
			// Update dir path and metadata from disk
			m.plugins[meta.ID].Dir = pluginDir
			m.plugins[meta.ID].PluginMeta = *meta
			enabled = m.plugins[meta.ID].Enabled
			source = m.plugins[meta.ID].Source
		}
		m.mu.Unlock()
		if err := m.syncOpenClawPluginState(meta.ID, pluginDir, enabled, source, version); err != nil {
			continue
		}
	}
}

func (m *Manager) readPluginMeta(dir string) (*PluginMeta, error) {
	metaPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		// Try package.json as fallback
		pkgPath := filepath.Join(dir, "package.json")
		data, err = os.ReadFile(pkgPath)
		if err != nil {
			return nil, fmt.Errorf("no plugin.json or package.json found")
		}
	}
	var meta PluginMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.ID == "" {
		meta.ID = filepath.Base(dir)
	}
	if meta.Name == "" {
		meta.Name = meta.ID
	}
	return &meta, nil
}

func (m *Manager) loadPluginsState() {
	data, err := os.ReadFile(m.configFile)
	if err != nil {
		return
	}
	var plugins map[string]*InstalledPlugin
	if json.Unmarshal(data, &plugins) == nil {
		m.plugins = plugins
	}
}

func (m *Manager) savePluginsState() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.savePluginsStateUnlocked()
}

func (m *Manager) savePluginsStateUnlocked() {
	data, _ := json.MarshalIndent(m.plugins, "", "  ")
	os.WriteFile(m.configFile, data, 0644)
}

func (m *Manager) cacheRegistry(reg *Registry) {
	data, _ := json.MarshalIndent(reg, "", "  ")
	os.WriteFile(filepath.Join(m.cfg.DataDir, "plugin-registry-cache.json"), data, 0644)
}

func (m *Manager) loadCachedRegistry() *Registry {
	data, err := os.ReadFile(filepath.Join(m.cfg.DataDir, "plugin-registry-cache.json"))
	if err != nil {
		return nil
	}
	var reg Registry
	if json.Unmarshal(data, &reg) == nil {
		return &reg
	}
	return nil
}

func (m *Manager) loadBundledRegistry() *Registry {
	exePath, err := os.Executable()
	if err != nil {
		return nil
	}
	registryPath := filepath.Join(filepath.Dir(filepath.Dir(exePath)), "plugins", "registry.json")
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return nil
	}
	var reg Registry
	if json.Unmarshal(data, &reg) == nil {
		return &reg
	}
	return nil
}

func mergeRegistries(primary Registry, bundled *Registry) Registry {
	if bundled == nil {
		return primary
	}
	merged := primary
	index := make(map[string]int, len(merged.Plugins))
	for i, plugin := range merged.Plugins {
		index[plugin.ID] = i
	}
	for _, plugin := range bundled.Plugins {
		if idx, ok := index[plugin.ID]; ok {
			merged.Plugins[idx] = plugin
		} else {
			merged.Plugins = append(merged.Plugins, plugin)
		}
	}
	if bundled.Version != "" {
		merged.Version = bundled.Version
	}
	if bundled.UpdatedAt != "" {
		merged.UpdatedAt = bundled.UpdatedAt
	}
	return merged
}

func (m *Manager) syncOpenClawPluginState(pluginID, installPath string, enabled bool, source string, version string) error {
	ocConfig, err := m.cfg.ReadOpenClawJSON()
	if err != nil || ocConfig == nil {
		ocConfig = map[string]interface{}{}
	}
	pl, _ := ocConfig["plugins"].(map[string]interface{})
	if pl == nil {
		pl = map[string]interface{}{}
		ocConfig["plugins"] = pl
	}
	ent, _ := pl["entries"].(map[string]interface{})
	if ent == nil {
		ent = map[string]interface{}{}
		pl["entries"] = ent
	}
	ent[pluginID] = map[string]interface{}{"enabled": enabled}

	ins, _ := pl["installs"].(map[string]interface{})
	if ins == nil {
		ins = map[string]interface{}{}
		pl["installs"] = ins
	}
	item, _ := ins[pluginID].(map[string]interface{})
	if item == nil {
		item = map[string]interface{}{}
		ins[pluginID] = item
	}
	if installPath != "" {
		item["installPath"] = installPath
	}
	if normalized := normalizeOpenClawInstallSource(source); normalized != "" {
		item["source"] = normalized
	}
	if version != "" {
		item["version"] = version
	}
	if _, ok := item["installedAt"]; !ok {
		item["installedAt"] = time.Now().UTC().Format(time.RFC3339)
	}
	return m.cfg.WriteOpenClawJSON(ocConfig)
}

func normalizeOpenClawInstallSource(source string) string {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case "npm":
		return "npm"
	case "archive":
		return "archive"
	case "path", "local", "registry", "custom", "github", "git":
		return "path"
	default:
		return "path"
	}
}

func (m *Manager) removeOpenClawPluginState(pluginID string) error {
	ocConfig, err := m.cfg.ReadOpenClawJSON()
	if err != nil || ocConfig == nil {
		return err
	}
	pl, _ := ocConfig["plugins"].(map[string]interface{})
	if pl == nil {
		return nil
	}
	if ent, ok := pl["entries"].(map[string]interface{}); ok {
		delete(ent, pluginID)
	}
	if ins, ok := pl["installs"].(map[string]interface{}); ok {
		delete(ins, pluginID)
	}
	return m.cfg.WriteOpenClawJSON(ocConfig)
}

func (m *Manager) installFromNpm(pkgName string) error {
	cmd := exec.Command("npm", "install", "-g", pkgName+"@latest", "--registry=https://registry.npmmirror.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Retry without mirror
		cmd2 := exec.Command("npm", "install", "-g", pkgName+"@latest")
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("%s\n%s", string(out), string(out2))
		}
	}
	return nil
}

func (m *Manager) installFromGit(gitURL, dest string) error {
	cmd := exec.Command("git", "clone", "--depth=1", gitURL, dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

func (m *Manager) installFromArchive(url, dest string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	os.MkdirAll(dest, 0755)
	tmpFile := filepath.Join(dest, "plugin-archive.tmp")
	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	io.Copy(f, resp.Body)
	f.Close()

	// Extract based on extension
	if strings.HasSuffix(url, ".zip") {
		cmd := exec.Command("unzip", "-o", tmpFile, "-d", dest)
		cmd.Run()
	} else if strings.HasSuffix(url, ".tar.gz") {
		cmd := exec.Command("tar", "-xzf", tmpFile, "-C", dest)
		cmd.Run()
	}

	os.Remove(tmpFile)
	return nil
}

// copyDir copies a directory recursively
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}
