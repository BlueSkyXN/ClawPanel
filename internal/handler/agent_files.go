package handler

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zhaoxinyi02/ClawPanel/internal/config"
)

type agentCoreFileEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Size     int64  `json:"size"`
	Modified string `json:"modified,omitempty"`
	Content  string `json:"content"`
}

type agentCoreWorkspaceLocation struct {
	Display string
	Root    string
	Rel     string
	Safe    string
}

var agentCoreFileNames = []string{
	"AGENTS.md",
	"SOUL.md",
	"TOOLS.md",
	"IDENTITY.md",
	"USER.md",
	"HEARTBEAT.md",
	"BOOT.md",
	"BOOTSTRAP.md",
	"MEMORY.md",
}

var errAgentCoreFileSymlink = errors.New("核心文件不能是符号链接")
var errAgentCoreFileWorkspaceSymlink = errors.New("agent workspace 路径不能包含符号链接")
var errAgentCoreFileWorkspaceOutsideRoots = errors.New("agent workspace 必须位于受管工作区目录下")
var errAgentCoreFileUnsupportedPlatform = errors.New("当前平台暂不支持受保护的 core-files 访问")

const agentCoreFileMaxBytes int64 = 512 * 1024

func resolveAgentWorkspacePath(cfg *config.Config, agentID string) string {
	ocConfig, _ := cfg.ReadOpenClawJSON()
	if ocConfig == nil {
		ocConfig = map[string]interface{}{}
	}

	for _, item := range materializeAgentList(cfg, ocConfig) {
		if strings.TrimSpace(toString(item["id"])) != agentID {
			continue
		}
		workspace := strings.TrimSpace(toString(item["workspace"]))
		if workspace == "" {
			break
		}
		if filepath.IsAbs(workspace) {
			return workspace
		}
		return filepath.Join(filepath.Dir(cfg.OpenClawDir), workspace)
	}

	candidates := []string{
		filepath.Join(filepath.Dir(cfg.OpenClawDir), "workspaces", agentID),
	}
	if cfg.OpenClawWork != "" {
		candidates = append(candidates, filepath.Join(cfg.OpenClawWork, agentID))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func isAllowedAgentCoreFile(name string) bool {
	for _, allowed := range agentCoreFileNames {
		if allowed == name {
			return true
		}
	}
	return false
}

func pathEscapesBase(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func evalPathWithExistingPrefix(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(absPath)
	var missing []string
	for {
		if _, err := os.Lstat(current); err == nil {
			real, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			real, err = filepath.Abs(real)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				real = filepath.Join(real, missing[i])
			}
			return real, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absPath, nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func splitPathSegments(path string) []string {
	clean := filepath.Clean(path)
	if clean == "." || clean == "" {
		return nil
	}
	return strings.Split(clean, string(os.PathSeparator))
}

func managedAgentWorkspaceRoots(cfg *config.Config) []string {
	baseRoot := filepath.Dir(cfg.OpenClawDir)
	roots := []string{
		filepath.Join(baseRoot, "workspaces"),
		filepath.Join(cfg.OpenClawDir, "workspaces"),
	}
	if cfg.OpenClawWork != "" {
		roots = append(roots, cfg.OpenClawWork)
	}
	// 将 agents.list 中显式配置的绝对 workspace 的父目录也视为受管根，
	// 支持 workspace 放在外部硬盘等 OpenClawDir 之外的位置。
	ocConfig, _ := cfg.ReadOpenClawJSON()
	for _, item := range parseAgentsListFromConfig(ocConfig) {
		ws := strings.TrimSpace(toString(item["workspace"]))
		if ws == "" || !filepath.IsAbs(ws) {
			continue
		}
		parent := filepath.Dir(filepath.Clean(ws))
		dup := false
		for _, r := range roots {
			if filepath.Clean(r) == parent {
				dup = true
				break
			}
		}
		if !dup {
			roots = append(roots, parent)
		}
	}
	return roots
}

func resolveAgentCoreWorkspaceRoots(cfg *config.Config, workspace string) (string, string, string, error) {
	absWorkspace := workspace
	if !filepath.IsAbs(absWorkspace) {
		absWorkspace = filepath.Join(filepath.Dir(cfg.OpenClawDir), absWorkspace)
	}
	absWorkspace, err := filepath.Abs(absWorkspace)
	if err != nil {
		return "", "", "", err
	}

	for _, root := range managedAgentWorkspaceRoots(cfg) {
		if root == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if pathEscapesBase(absRoot, absWorkspace) {
			continue
		}
		realRoot, err := evalPathWithExistingPrefix(absRoot)
		if err != nil {
			continue
		}
		if filepath.Clean(realRoot) != filepath.Clean(absRoot) {
			return "", "", "", errAgentCoreFileWorkspaceSymlink
		}
		rel, err := filepath.Rel(absRoot, absWorkspace)
		if err != nil {
			continue
		}
		return absWorkspace, absRoot, filepath.Clean(rel), nil
	}

	return "", "", "", errAgentCoreFileWorkspaceOutsideRoots
}

func ensureAgentWorkspaceComponentsSafe(root, rel string) error {
	current := filepath.Clean(root)
	if info, err := os.Lstat(current); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errAgentCoreFileWorkspaceSymlink
		}
		if !info.IsDir() {
			return errors.New("agent workspace 根目录无效")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	segments := splitPathSegments(rel)
	for index, segment := range segments {
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errAgentCoreFileWorkspaceSymlink
		}
		if !info.IsDir() {
			if index == len(segments)-1 {
				return errors.New("agent workspace 必须是目录")
			}
			return errors.New("agent workspace 路径无效")
		}
	}
	return nil
}

func resolveAgentCoreWorkspace(cfg *config.Config, agentID string, create bool) (agentCoreWorkspaceLocation, error) {
	workspace := resolveAgentWorkspacePath(cfg, agentID)
	if workspace == "" {
		return agentCoreWorkspaceLocation{}, os.ErrNotExist
	}
	displayWorkspace, root, rel, err := resolveAgentCoreWorkspaceRoots(cfg, workspace)
	if err != nil {
		return agentCoreWorkspaceLocation{}, err
	}
	if err := ensureAgentWorkspaceComponentsSafe(root, rel); err != nil {
		return agentCoreWorkspaceLocation{}, err
	}
	realRoot, err := evalPathWithExistingPrefix(root)
	if err != nil {
		return agentCoreWorkspaceLocation{}, err
	}
	safeWorkspace := filepath.Clean(filepath.Join(realRoot, rel))
	if pathEscapesBase(realRoot, safeWorkspace) {
		return agentCoreWorkspaceLocation{}, errors.New("agent workspace 路径超出允许范围")
	}
	return agentCoreWorkspaceLocation{
		Display: displayWorkspace,
		Root:    realRoot,
		Rel:     rel,
		Safe:    safeWorkspace,
	}, nil
}

func resolveAllowedAgentCoreFilePath(workspace, name string) (string, error) {
	full := filepath.Join(workspace, name)
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absWorkspace, absFull)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("路径超出允许范围")
	}
	return absFull, nil
}

func statAgentCoreFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errAgentCoreFileSymlink
	}
	if info.IsDir() {
		return nil, errors.New("核心文件路径不能是目录")
	}
	return info, nil
}

func readAgentCoreFileContent(path string, size int64) (string, bool, error) {
	if size <= agentCoreFileMaxBytes {
		content, err := os.ReadFile(path)
		return string(content), false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, agentCoreFileMaxBytes))
	if err != nil {
		return "", false, err
	}
	content = append(content, []byte("\n\n... (文件过大，已截断)")...)
	return string(content), true, nil
}

func GetOpenClawAgentCoreFiles(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := strings.TrimSpace(c.Param("id"))
		if err := validateAgentID(agentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
			return
		}

		workspace, err := resolveAgentCoreWorkspace(cfg, agentID, false)
		if err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "agent workspace 未配置"})
				return
			}
			if errors.Is(err, errAgentCoreFileWorkspaceSymlink) || errors.Is(err, errAgentCoreFileWorkspaceOutsideRoots) {
				c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
			return
		}
		if workspace.Display == "" {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "agent workspace 未配置"})
			return
		}

		files := make([]agentCoreFileEntry, 0, len(agentCoreFileNames))
		for _, name := range agentCoreFileNames {
			entry := agentCoreFileEntry{
				Name:    name,
				Path:    filepath.Join(workspace.Display, name),
				Exists:  false,
				Size:    0,
				Content: "",
			}
			content, size, modified, exists, err := loadAgentCoreFile(workspace, name)
			switch {
			case err == nil && exists:
				entry.Exists = true
				entry.Size = size
				entry.Modified = modified.Format(time.RFC3339)
				entry.Content = content
			case errors.Is(err, errAgentCoreFileUnsupportedPlatform):
				c.JSON(http.StatusNotImplemented, gin.H{"ok": false, "error": err.Error()})
				return
			case errors.Is(err, errAgentCoreFileSymlink), os.IsNotExist(err), err == nil:
				// Keep symlinked or missing files absent in the list, but do not mask other errors.
			case err != nil:
				c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
				return
			}
			files = append(files, entry)
		}

		c.JSON(http.StatusOK, gin.H{
			"ok":        true,
			"agentId":   agentID,
			"workspace": workspace.Display,
			"files":     files,
		})
	}
}

func SaveOpenClawAgentCoreFile(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := strings.TrimSpace(c.Param("id"))
		if err := validateAgentID(agentID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
			return
		}

		var req struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, agentCoreFileMaxBytes+4096)
		if err := c.ShouldBindJSON(&req); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"ok": false, "error": "核心文件内容过大"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "参数错误"})
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if !isAllowedAgentCoreFile(req.Name) {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "不支持的核心文件"})
			return
		}
		if int64(len(req.Content)) > agentCoreFileMaxBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"ok": false, "error": "核心文件内容过大"})
			return
		}

		workspace, err := resolveAgentCoreWorkspace(cfg, agentID, true)
		if err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "agent workspace 未配置"})
				return
			}
			if errors.Is(err, errAgentCoreFileWorkspaceSymlink) || errors.Is(err, errAgentCoreFileWorkspaceOutsideRoots) {
				c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
			return
		}
		if workspace.Display == "" {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "agent workspace 未配置"})
			return
		}

		if err := saveAgentCoreFile(workspace, req.Name, []byte(req.Content)); err != nil {
			if errors.Is(err, errAgentCoreFileSymlink) {
				c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": err.Error()})
				return
			}
			if errors.Is(err, errAgentCoreFileUnsupportedPlatform) {
				c.JSON(http.StatusNotImplemented, gin.H{"ok": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"ok":        true,
			"agentId":   agentID,
			"workspace": workspace.Display,
			"path":      filepath.Join(workspace.Display, req.Name),
		})
	}
}
