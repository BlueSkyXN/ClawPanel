package process

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zhaoxinyi02/ClawPanel/internal/config"
	"github.com/zhaoxinyi02/ClawPanel/internal/websocket"
)

// Status 进程状态
type Status struct {
	Running   bool      `json:"running"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	Uptime    int64     `json:"uptime"` // 秒
	ExitCode  int       `json:"exitCode,omitempty"`
}

// Manager 进程管理器
type Manager struct {
	cfg       *config.Config
	cmd       *exec.Cmd
	status    Status
	mu        sync.RWMutex
	logLines  []string
	logMu     sync.RWMutex
	maxLog    int
	stopCh    chan struct{}
	logReader io.ReadCloser
}

// NewManager 创建进程管理器
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:    cfg,
		maxLog: 5000,
		stopCh: make(chan struct{}),
	}
}

// Start 启动 OpenClaw 进程
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status.Running {
		return fmt.Errorf("OpenClaw 已在运行中 (PID: %d)", m.status.PID)
	}

	// 启动前确保 openclaw.json 配置正确
	m.ensureOpenClawConfig()

	// 查找 openclaw 可执行文件
	openclawBin := m.findOpenClawBin()
	if openclawBin == "" {
		return fmt.Errorf("未找到 openclaw 可执行文件，请确保已安装 OpenClaw")
	}

	// 构建启动命令
	m.cmd = exec.Command(openclawBin, "gateway")
	m.cmd.Dir = m.cfg.OpenClawDir
	m.cmd.Env = append(os.Environ(),
		fmt.Sprintf("OPENCLAW_DIR=%s", m.cfg.OpenClawDir),
		fmt.Sprintf("OPENCLAW_STATE_DIR=%s", m.cfg.OpenClawDir),
		fmt.Sprintf("OPENCLAW_CONFIG_PATH=%s/openclaw.json", m.cfg.OpenClawDir),
	)

	// 捕获 stdout 和 stderr
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 stdout 管道失败: %w", err)
	}
	stderr, err := m.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("启动 OpenClaw 失败: %w", err)
	}

	m.status = Status{
		Running:   true,
		PID:       m.cmd.Process.Pid,
		StartedAt: time.Now(),
	}

	// 合并 stdout 和 stderr
	m.logReader = io.NopCloser(io.MultiReader(stdout, stderr))

	// 后台监控进程退出
	go m.waitForExit()

	log.Printf("[ProcessMgr] OpenClaw 已启动 (PID: %d)", m.status.PID)
	return nil
}

// Stop 停止 OpenClaw 进程
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.status.Running || m.cmd == nil || m.cmd.Process == nil {
		return fmt.Errorf("OpenClaw 未在运行")
	}

	log.Printf("[ProcessMgr] 正在停止 OpenClaw (PID: %d)...", m.status.PID)

	// 先尝试优雅关闭
	if runtime.GOOS == "windows" {
		m.cmd.Process.Kill()
	} else {
		m.cmd.Process.Signal(os.Interrupt)
		// 等待 5 秒，如果还没退出则强制杀死
		done := make(chan struct{})
		go func() {
			m.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			m.cmd.Process.Kill()
		}
	}

	m.status.Running = false
	m.status.PID = 0
	log.Println("[ProcessMgr] OpenClaw 已停止")
	return nil
}

// Restart 重启 OpenClaw 进程
func (m *Manager) Restart() error {
	if m.GetStatus().Running {
		if err := m.Stop(); err != nil {
			log.Printf("[ProcessMgr] 停止失败: %v", err)
		}
		time.Sleep(time.Second)
	}
	return m.Start()
}

// StopAll 停止所有进程
func (m *Manager) StopAll() {
	if m.GetStatus().Running {
		m.Stop()
	}
}

// GetStatus 获取进程状态
func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s := m.status
	if s.Running {
		s.Uptime = int64(time.Since(s.StartedAt).Seconds())
	}
	return s
}

// GetLogs 获取日志
func (m *Manager) GetLogs(n int) []string {
	m.logMu.RLock()
	defer m.logMu.RUnlock()

	if n <= 0 || n > len(m.logLines) {
		n = len(m.logLines)
	}
	start := len(m.logLines) - n
	if start < 0 {
		start = 0
	}
	result := make([]string, n)
	copy(result, m.logLines[start:])
	return result
}

// StreamLogs 将进程日志流式推送到 WebSocket Hub
func (m *Manager) StreamLogs(hub *websocket.Hub) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	lastIdx := 0
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.logMu.RLock()
			newLines := m.logLines[lastIdx:]
			lastIdx = len(m.logLines)
			m.logMu.RUnlock()

			for _, line := range newLines {
				hub.Broadcast([]byte(line))
			}
		}
	}
}

// addLogLine 添加日志行
func (m *Manager) addLogLine(line string) {
	m.logMu.Lock()
	defer m.logMu.Unlock()

	m.logLines = append(m.logLines, line)
	if len(m.logLines) > m.maxLog {
		m.logLines = m.logLines[len(m.logLines)-m.maxLog:]
	}
}

// waitForExit 等待进程退出，异常退出时自动重启
func (m *Manager) waitForExit() {
	if m.logReader != nil {
		scanner := bufio.NewScanner(m.logReader)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)
		for scanner.Scan() {
			m.addLogLine(scanner.Text())
		}
	}

	if m.cmd != nil {
		err := m.cmd.Wait()
		m.mu.Lock()
		wasRunning := m.status.Running
		m.status.Running = false
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
				m.status.ExitCode = exitCode
			}
		}
		m.mu.Unlock()
		log.Printf("[ProcessMgr] OpenClaw 进程已退出 (code: %d)", exitCode)

		// OpenClaw gateway uses a daemon fork pattern: it spawns a child
		// process "openclaw-gateway" that holds the port, then the parent
		// exits (often with code 1). If the gateway port is listening after
		// the parent exits, the daemon started successfully.
		if wasRunning && exitCode != 0 {
			time.Sleep(1 * time.Second) // give the daemon child time to bind
			gatewayPort := m.getGatewayPort()
			if gatewayPort != "" && m.isPortListening(gatewayPort) {
				log.Printf("[ProcessMgr] OpenClaw 父进程已退出但网关守护进程正在端口 %s 运行（daemon fork 模式），视为正常", gatewayPort)
				m.mu.Lock()
				m.status.Running = true
				m.status.ExitCode = 0
				m.mu.Unlock()
				// Monitor the daemon process; when port goes down, restart
				go m.monitorDaemon(gatewayPort)
				return
			}
			log.Println("[ProcessMgr] 检测到 OpenClaw 异常退出，3秒后自动重启...")
			time.Sleep(2 * time.Second)
			if err := m.Start(); err != nil {
				log.Printf("[ProcessMgr] 自动重启失败: %v", err)
			} else {
				log.Println("[ProcessMgr] OpenClaw 已自动重启")
			}
		}
	}
}

// getGatewayPort reads the gateway port from openclaw.json config
func (m *Manager) getGatewayPort() string {
	ocDir := m.cfg.OpenClawDir
	if ocDir == "" {
		home, _ := os.UserHomeDir()
		ocDir = filepath.Join(home, ".openclaw")
	}
	cfgPath := filepath.Join(ocDir, "openclaw.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return "18789" // default gateway port
	}
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		return "18789"
	}
	if gw, ok := cfg["gateway"].(map[string]interface{}); ok {
		if port, ok := gw["port"].(float64); ok && port > 0 {
			return fmt.Sprintf("%d", int(port))
		}
	}
	return "18789"
}

// isPortListening checks if a TCP port is currently listening
func (m *Manager) isPortListening(port string) bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// monitorDaemon monitors the OpenClaw daemon process (fork pattern).
// When the gateway port stops listening, mark process as stopped and restart.
func (m *Manager) monitorDaemon(port string) {
	failCount := 0
	for {
		time.Sleep(5 * time.Second)
		m.mu.RLock()
		running := m.status.Running
		m.mu.RUnlock()
		if !running {
			return // manually stopped
		}
		if m.isPortListening(port) {
			failCount = 0
			continue
		}
		failCount++
		if failCount >= 2 { // 2 consecutive failures (10s)
			log.Printf("[ProcessMgr] OpenClaw 守护进程端口 %s 不再监听，尝试重启...", port)
			m.mu.Lock()
			m.status.Running = false
			m.mu.Unlock()
			time.Sleep(2 * time.Second)
			if err := m.Start(); err != nil {
				log.Printf("[ProcessMgr] 自动重启失败: %v", err)
			} else {
				log.Println("[ProcessMgr] OpenClaw 已自动重启")
			}
			return
		}
	}
}

// ensureOpenClawConfig 启动前检查并修复 openclaw.json 关键配置
// 始终确保 gateway.mode=local；仅当 QQ 插件已安装且 NapCat 正在运行时
// 才写入 channels.qq / plugins.entries.qq / plugins.installs.qq，
// 避免用户不使用 QQ 插件时被强制写入导致 OpenClaw 网关启动失败。
func (m *Manager) ensureOpenClawConfig() {
	ocDir := m.cfg.OpenClawDir
	if ocDir == "" {
		home, _ := os.UserHomeDir()
		ocDir = filepath.Join(home, ".openclaw")
	}
	cfgPath := filepath.Join(ocDir, "openclaw.json")

	var cfg map[string]interface{}
	created := false

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		// 配置文件不存在，创建目录并初始化空配置
		os.MkdirAll(filepath.Dir(cfgPath), 0755)
		cfg = map[string]interface{}{}
		created = true
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			cfg = map[string]interface{}{}
			created = true
		}
	}

	changed := created

	// Always ensure gateway.mode = "local" — safe regardless of plugins
	gw, _ := cfg["gateway"].(map[string]interface{})
	if gw == nil {
		gw = map[string]interface{}{}
		cfg["gateway"] = gw
	}
	if gw["mode"] != "local" {
		gw["mode"] = "local"
		changed = true
	}

	// Only write QQ plugin config if:
	//   1. The QQ extension directory is installed (extensions/qq exists), AND
	//   2. NapCat is actually running (Docker container or Windows process)
	// Without both conditions, injecting channels.qq causes OpenClaw gateway to
	// fail on startup with "unknown channel id: qq" or similar errors.
	qqExtDir := filepath.Join(ocDir, "extensions", "qq")
	qqInstalled := false
	if _, err := os.Stat(qqExtDir); err == nil {
		qqInstalled = true
	}
	napcatRunning := m.isNapCatRunning()

	if qqInstalled && napcatRunning {
		// Ensure channels.qq with wsUrl
		ch, _ := cfg["channels"].(map[string]interface{})
		if ch == nil {
			ch = map[string]interface{}{}
			cfg["channels"] = ch
		}
		qq, _ := ch["qq"].(map[string]interface{})
		if qq == nil {
			qq = map[string]interface{}{}
			ch["qq"] = qq
		}
		if qq["wsUrl"] == nil || qq["wsUrl"] == "" {
			qq["wsUrl"] = "ws://127.0.0.1:3001"
			changed = true
		}
		if qq["enabled"] == nil {
			qq["enabled"] = true
			changed = true
		}

		// Ensure plugins.entries.qq
		pl, _ := cfg["plugins"].(map[string]interface{})
		if pl == nil {
			pl = map[string]interface{}{}
			cfg["plugins"] = pl
		}
		ent, _ := pl["entries"].(map[string]interface{})
		if ent == nil {
			ent = map[string]interface{}{}
			pl["entries"] = ent
		}
		if ent["qq"] == nil {
			ent["qq"] = map[string]interface{}{"enabled": true}
			changed = true
		}

		// Ensure plugins.installs.qq
		ins, _ := pl["installs"].(map[string]interface{})
		if ins == nil {
			ins = map[string]interface{}{}
			pl["installs"] = ins
		}
		if ins["qq"] == nil {
			ins["qq"] = map[string]interface{}{
				"installPath": qqExtDir,
				"source":      "archive",
				"version":     "1.0.0",
			}
			changed = true
		}
	} else if qqInstalled && !napcatRunning {
		log.Println("[ProcessMgr] QQ 插件已安装但 NapCat 未运行，跳过 channels.qq 配置注入")
	}

	if changed {
		out, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			log.Printf("[ProcessMgr] openclaw.json 序列化失败: %v", err)
		} else if err := os.WriteFile(cfgPath, out, 0644); err != nil {
			log.Printf("[ProcessMgr] openclaw.json 写入失败: %v", err)
		} else {
			log.Println("[ProcessMgr] openclaw.json 配置已自动修复 (gateway.mode/channels.qq/plugins)")
		}
	}

	// Patch QQ plugin channel.ts: startAccount must return a long-lived Promise
	m.patchQQPluginChannelTS(ocDir)
}

// patchQQPluginChannelTS fixes the critical bug where the QQ plugin's startAccount
// returns a cleanup function instead of a long-lived Promise. OpenClaw gateway
// wraps startAccount's return value with Promise.resolve(task); if it resolves
// immediately (non-Promise return), the framework treats the account as exited
// and triggers auto-restart attempts (up to 10), after which the channel handler
// dies and incoming messages are never processed.
func (m *Manager) patchQQPluginChannelTS(ocDir string) {
	channelTS := filepath.Join(ocDir, "extensions", "qq", "src", "channel.ts")
	data, err := os.ReadFile(channelTS)
	if err != nil {
		return // plugin not installed
	}
	content := string(data)

	// Already patched?
	if strings.Contains(content, "new Promise") {
		return
	}
	// Check for the broken pattern
	if !strings.Contains(content, "return () => {") || !strings.Contains(content, "client.disconnect") {
		return
	}

	oldCode := `      client.connect();
      
      return () => {
        client.disconnect();
        clients.delete(account.accountId);
        stopFileServer();
      };`

	newCode := `      client.connect();

      // Return a Promise that stays pending until abortSignal fires.
      // OpenClaw gateway expects startAccount to return a long-lived Promise;
      // if it resolves immediately, the framework treats the account as exited
      // and triggers auto-restart attempts.
      const abortSignal = (ctx as any).abortSignal as AbortSignal | undefined;
      return new Promise<void>((resolve) => {
        const cleanup = () => {
          client.disconnect();
          clients.delete(account.accountId);
          stopFileServer();
          resolve();
        };
        if (abortSignal) {
          if (abortSignal.aborted) { cleanup(); return; }
          abortSignal.addEventListener("abort", cleanup, { once: true });
        }
        // Also clean up if the WebSocket closes unexpectedly
        client.on("close", () => {
          cleanup();
        });
      });`

	if !strings.Contains(content, oldCode) {
		log.Println("[ProcessMgr] channel.ts 需要修复但模式不匹配，跳过自动补丁")
		return
	}

	patched := strings.Replace(content, oldCode, newCode, 1)
	if err := os.WriteFile(channelTS, []byte(patched), 0644); err != nil {
		log.Printf("[ProcessMgr] channel.ts 补丁写入失败: %v", err)
		return
	}
	log.Println("[ProcessMgr] ✅ channel.ts startAccount 已自动修复 (返回 long-lived Promise)")
}

// findOpenClawBin 查找 openclaw 可执行文件
func (m *Manager) findOpenClawBin() string {
	candidates := []string{
		"openclaw",
	}

	// 添加常见路径
	home, _ := os.UserHomeDir()
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", "openclaw"),
			filepath.Join(home, "openclaw", "app", "openclaw"),
		)
	}

	switch runtime.GOOS {
	case "linux":
		candidates = append(candidates,
			"/usr/local/bin/openclaw",
			"/usr/bin/openclaw",
			"/snap/bin/openclaw",
		)
	case "darwin":
		candidates = append(candidates,
			"/usr/local/bin/openclaw",
			"/opt/homebrew/bin/openclaw",
		)
	case "windows":
		candidates = append(candidates,
			`C:\Program Files\openclaw\openclaw.exe`,
			filepath.Join(home, "AppData", "Roaming", "npm", "openclaw.cmd"),
		)
	}

	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

// isNapCatRunning returns true if NapCat is currently running.
// On Linux it checks for the "openclaw-qq" Docker container;
// on Windows it checks for NapCat shell processes.
func (m *Manager) isNapCatRunning() bool {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq NapCatWinBootMain.exe", "/NH").Output()
		if err == nil && strings.Contains(string(out), "NapCatWinBootMain") {
			return true
		}
		out2, err2 := exec.Command("tasklist", "/FI", "IMAGENAME eq napcat.exe", "/NH").Output()
		return err2 == nil && strings.Contains(string(out2), "napcat.exe")
	}
	// Linux: check Docker container state
	out, err := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", "openclaw-qq").Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}
