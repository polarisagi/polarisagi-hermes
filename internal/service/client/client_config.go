package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/polarisagi/polarisagi-hermes/internal/domain"
	"github.com/polarisagi/polarisagi-hermes/internal/repository/sqlite"
)

// ─────────────────────────────────────────────
// 注入类型常量
// ─────────────────────────────────────────────

const (
	injectTypeEnv  = "env"  // KEY=VALUE 行格式的 .env 文件
	injectTypeJSON = "json" // JSON 配置文件
)

// ─────────────────────────────────────────────
// 客户端定义
// ─────────────────────────────────────────────

// clientDef 描述一种 AI 客户端的探测路径和注入逻辑
type clientDef struct {
	Name        string
	DisplayName string
	Description string
	Icon        string
	InjectType  string

	// 配置文件路径（相对于 HOME）
	ConfigRelPath string

	// 对于 env 类型：需要注入的 KEY 列表
	EnvKeys []envKeyDef

	// 对于 JSON 类型的补丁生成器
	BuildJSONPatch func(listenAddr string) map[string]interface{}

	// 检测"已配置"的特征 key（env 类型）
	SignatureEnvKey string
	// 检测"已配置"的 JSON dot.path（json 类型）
	SignatureJSONPath string
}

type envKeyDef struct {
	Key   string
	Value func(listenAddr string) string
}

// polarisAPIKey 是注入给客户端的占位 API Key（网关内部不校验 key，直接路由）
const polarisAPIKey = "sk-polarisagi-hermes"

// polarisMarker 用于检测配置中是否已被 Polaris 注入（通过端口号或标识字符串）
const polarisMarker = "27777"

// allClients 是全部支持的客户端定义列表
var allClients = []clientDef{
	// ── 1. Claude Code ──────────────────────────────────────────────
	{
		Name:          "claude_code",
		DisplayName:   "Claude Code",
		Description:   "Anthropic 官方 AI 编程助手",
		Icon:          "🤖",
		InjectType:    injectTypeEnv,
		ConfigRelPath: ".claude/.env",
		EnvKeys: []envKeyDef{
			{Key: "ANTHROPIC_BASE_URL", Value: func(addr string) string { return "http://" + addr }},
			{Key: "ANTHROPIC_API_KEY", Value: func(_ string) string { return polarisAPIKey }},
		},
		SignatureEnvKey: "ANTHROPIC_BASE_URL",
	},

	// ── 2. OpenAI Codex CLI ─────────────────────────────────────────
	{
		Name:          "codex",
		DisplayName:   "OpenAI Codex",
		Description:   "OpenAI 官方命令行 AI 编程工具",
		Icon:          "⚡",
		InjectType:    injectTypeJSON,
		ConfigRelPath: ".codex/config.json",
		BuildJSONPatch: func(addr string) map[string]interface{} {
			return map[string]interface{}{
				"provider": "openai",
				"providers": map[string]interface{}{
					"openai": map[string]interface{}{
						"name":    "Polaris-Hermes (via OpenAI)",
						"baseURL": "http://" + addr + "/v1",
					},
				},
			}
		},
		SignatureJSONPath: "providers.openai.baseURL",
	},

	// ── 3. OpenCode ─────────────────────────────────────────────────
	{
		Name:          "opencode",
		DisplayName:   "OpenCode",
		Description:   "开源 AI 终端编程助手 (opencode.ai)",
		Icon:          "🔧",
		InjectType:    injectTypeEnv,
		ConfigRelPath: ".config/opencode/.env",
		EnvKeys: []envKeyDef{
			{Key: "OPENAI_API_KEY", Value: func(_ string) string { return polarisAPIKey }},
			{Key: "OPENAI_BASE_URL", Value: func(addr string) string { return "http://" + addr + "/v1" }},
		},
		SignatureEnvKey: "OPENAI_BASE_URL",
	},

	// ── 4. Gemini CLI ────────────────────────────────────────────────
	{
		Name:          "gemini_cli",
		DisplayName:   "Gemini CLI",
		Description:   "Google 官方 Gemini 命令行工具",
		Icon:          "✨",
		InjectType:    injectTypeJSON,
		ConfigRelPath: ".gemini/settings.json",
		BuildJSONPatch: func(addr string) map[string]interface{} {
			return map[string]interface{}{
				"apiEndpoint": "http://" + addr + "/v1",
				"apiKey":      polarisAPIKey,
			}
		},
		SignatureJSONPath: "apiEndpoint",
	},
}

// ─────────────────────────────────────────────
// Manager
// ─────────────────────────────────────────────

// Manager 管理所有客户端的自动配置注入和恢复
type Manager struct {
	settingsRepo *sqlite.SettingsRepo
	backupRepo   *sqlite.ClientBackupRepo
}

// NewManager 创建一个新的 Manager
func NewManager(settingsRepo *sqlite.SettingsRepo, backupRepo *sqlite.ClientBackupRepo) *Manager {
	return &Manager{
		settingsRepo: settingsRepo,
		backupRepo:   backupRepo,
	}
}

// ─────────────────────────────────────────────
// GetAllStatuses — 探测全部客户端状态
// ─────────────────────────────────────────────

// GetAllStatuses 返回所有支持客户端的当前状态列表
func (m *Manager) GetAllStatuses(ctx context.Context) ([]domain.ClientStatus, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("无法获取用户主目录: %w", err)
	}

	// 批量加载所有备份记录，避免 N+1 查询
	backups, err := m.backupRepo.GetAll(ctx)
	if err != nil {
		slog.Warn("加载客户端备份记录失败", "error", err)
		backups = make(map[string]*sqlite.ClientBackupRecord)
	}

	statuses := make([]domain.ClientStatus, 0, len(allClients))
	for _, def := range allClients {
		st := m.detectStatus(home, def, backups)
		statuses = append(statuses, st)
	}
	return statuses, nil
}

// detectStatus 探测单个客户端的状态
func (m *Manager) detectStatus(home string, def clientDef, backups map[string]*sqlite.ClientBackupRecord) domain.ClientStatus {
	st := domain.ClientStatus{
		Name:        def.Name,
		DisplayName: def.DisplayName,
		Description: def.Description,
		Icon:        def.Icon,
	}

	configPath := filepath.Join(home, def.ConfigRelPath)

	// 检测是否已安装（父目录存在即视为安装）
	if _, err := os.Stat(filepath.Dir(configPath)); err == nil {
		st.IsInstalled = true
	}

	// 检测是否已注入代理配置
	switch def.InjectType {
	case injectTypeEnv:
		st.IsConfigured = isEnvConfigured(configPath, def.SignatureEnvKey)
	case injectTypeJSON:
		st.IsConfigured = isJSONConfigured(configPath, def.SignatureJSONPath)
	}

	// 检测是否有备份（来自批量预加载的 map）
	_, st.HasBackup = backups[def.Name]

	return st
}

// ─────────────────────────────────────────────
// ApplyConfig — 备份并注入代理配置
// ─────────────────────────────────────────────

// ApplyConfig 为指定客户端注入 Polaris-Hermes 代理配置
func (m *Manager) ApplyConfig(ctx context.Context, clientName string) error {
	def, ok := findClient(clientName)
	if !ok {
		return fmt.Errorf("不支持的客户端: %s", clientName)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("无法获取用户主目录: %w", err)
	}

	// 读取监听地址
	listenAddr, _ := m.settingsRepo.GetSetting(ctx, "listen_addr")
	if listenAddr == "" {
		listenAddr = "127.0.0.1:27777"
	}

	configPath := filepath.Join(home, def.ConfigRelPath)

	// 确保父目录存在
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("无法创建配置目录: %w", err)
	}

	// 备份现有配置到专用表
	if err := m.backup(ctx, def.Name, configPath); err != nil {
		slog.Warn("备份配置文件失败，继续写入", "client", def.Name, "error", err)
	}

	// 执行注入
	switch def.InjectType {
	case injectTypeEnv:
		err = applyEnvConfig(configPath, def.EnvKeys, listenAddr)
	case injectTypeJSON:
		err = applyJSONConfig(configPath, def.BuildJSONPatch(listenAddr))
	}

	if err != nil {
		return fmt.Errorf("注入配置失败: %w", err)
	}

	slog.Info("客户端代理配置注入成功", "client", def.DisplayName, "config", configPath)
	return nil
}

// ─────────────────────────────────────────────
// RestoreConfig — 从专用表恢复原始配置
// ─────────────────────────────────────────────

// RestoreConfig 从 client_config_backups 表恢复指定客户端的原始配置
func (m *Manager) RestoreConfig(ctx context.Context, clientName string) error {
	def, ok := findClient(clientName)
	if !ok {
		return fmt.Errorf("不支持的客户端: %s", clientName)
	}

	rec, err := m.backupRepo.Get(ctx, clientName)
	if err != nil {
		return fmt.Errorf("查询备份记录失败: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("未找到客户端 %s 的备份数据", def.DisplayName)
	}

	if rec.OriginalContent == "" {
		// 备份时文件不存在，恢复时删除注入的配置文件
		if err := os.Remove(rec.ConfigPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除注入的配置文件失败: %w", err)
		}
	} else {
		if err := os.WriteFile(rec.ConfigPath, []byte(rec.OriginalContent), 0644); err != nil {
			return fmt.Errorf("恢复配置文件失败: %w", err)
		}
	}

	// 删除备份记录
	if err := m.backupRepo.Delete(ctx, clientName); err != nil {
		slog.Warn("删除备份记录失败", "client", clientName, "error", err)
	}

	slog.Info("客户端配置已恢复原始状态", "client", def.DisplayName)
	return nil
}

// ─────────────────────────────────────────────
// 内部：备份到专用表
// ─────────────────────────────────────────────

func (m *Manager) backup(ctx context.Context, clientName, configPath string) error {
	content := ""
	if data, err := os.ReadFile(configPath); err == nil {
		content = string(data)
	}
	// 使用 Upsert：重复执行 Apply 时更新备份而不是报错
	return m.backupRepo.Upsert(ctx, clientName, configPath, content)
}

// ─────────────────────────────────────────────
// 内部：env 注入 & 检测
// ─────────────────────────────────────────────

func applyEnvConfig(path string, keys []envKeyDef, listenAddr string) error {
	existingLines := []string{}
	if data, err := os.ReadFile(path); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			existingLines = append(existingLines, scanner.Text())
		}
	}

	injectSet := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		injectSet[k.Key] = struct{}{}
	}

	// 过滤掉已存在的同名 key 行及旧的 Polaris 注释块
	filtered := make([]string, 0, len(existingLines))
	skipBlock := false
	for _, line := range existingLines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "Polaris-Hermes Proxy Config") {
			skipBlock = true
		}
		if strings.Contains(trimmed, "End Polaris-Hermes") {
			skipBlock = false
			continue
		}
		if skipBlock {
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			filtered = append(filtered, line)
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			if _, exists := injectSet[parts[0]]; exists {
				continue // 跳过旧值行
			}
		}
		filtered = append(filtered, line)
	}

	// 追加新注入块
	filtered = append(filtered, "")
	filtered = append(filtered, "# ── Polaris-Hermes Proxy Config (auto-injected) ──")
	for _, k := range keys {
		filtered = append(filtered, fmt.Sprintf("%s=%s", k.Key, k.Value(listenAddr)))
	}
	filtered = append(filtered, "# ── End Polaris-Hermes ──")

	return os.WriteFile(path, []byte(strings.Join(filtered, "\n")+"\n"), 0644)
}

func isEnvConfigured(path, signatureKey string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, signatureKey+"=") {
			val := strings.TrimPrefix(line, signatureKey+"=")
			return strings.Contains(val, polarisMarker) || strings.Contains(strings.ToLower(val), "polaris")
		}
	}
	return false
}

// ─────────────────────────────────────────────
// 内部：JSON 注入 & 检测
// ─────────────────────────────────────────────

func applyJSONConfig(path string, patch map[string]interface{}) error {
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	merged := deepMerge(existing, patch)
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

func isJSONConfigured(path, jsonPath string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return false
	}
	val, _ := getJSONPath(obj, jsonPath).(string)
	return strings.Contains(val, polarisMarker) || strings.Contains(strings.ToLower(val), "polaris")
}

func deepMerge(base, patch map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range patch {
		if pMap, ok := v.(map[string]interface{}); ok {
			if bMap, ok := result[k].(map[string]interface{}); ok {
				result[k] = deepMerge(bMap, pMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}

func getJSONPath(obj map[string]interface{}, path string) interface{} {
	parts := strings.SplitN(path, ".", 2)
	val, ok := obj[parts[0]]
	if !ok {
		return nil
	}
	if len(parts) == 1 {
		return val
	}
	sub, ok := val.(map[string]interface{})
	if !ok {
		return nil
	}
	return getJSONPath(sub, parts[1])
}

// ─────────────────────────────────────────────
// 工具
// ─────────────────────────────────────────────

func findClient(name string) (clientDef, bool) {
	for _, c := range allClients {
		if c.Name == name {
			return c, true
		}
	}
	return clientDef{}, false
}
