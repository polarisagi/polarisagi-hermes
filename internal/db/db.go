package db

import (
	"database/sql"
	"embed"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

func getDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("❌ 无法获取用户主目录，回退到当前目录", "error", err)
		return "./polaris_gateway.db"
	}
	
	dir := filepath.Join(home, ".polaris-gateway")
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("❌ 无法创建配置目录，回退到当前目录", "error", err)
		return "./polaris_gateway.db"
	}
	
	return filepath.Join(dir, "polaris_gateway.db")
}

var db *sql.DB

//go:embed migrations/*.sql
var migrationsFS embed.FS

func DB() *sql.DB {
	return db
}

// UsageLog 请求用量记录，通过 channel 异步写入数据库
// 避免同步 I/O 阻塞 API 请求路径，以 1024 缓冲区缓解写入峰值
type UsageLog struct {
	Platform    string  // 所属平台标识: "openai"/"vertex"/"anthropic"
	NodeName   string   // 节点名称
	ClientID   string   // 客户端识别标识（从 User-Agent 推导）
	MethodName string   // 调用方法名，如 "chat/completions"
	Prompt      int64   // 提示词 token 数
	Completion  int64   // 生成 token 数
	Cost        float64 // 本次调用费用（美元）
	Status      int     // HTTP 状态码
}

var logChan = make(chan UsageLog, 1024) // 异步写入通道，缓冲区 1024 条

// InitDB 初始化 SQLite 数据库连接，启用 WAL 模式提高并发读写性能
// 自动执行嵌入的 schema 迁移脚本，并启动后台异步写入协程
func InitDB() {
	var err error
	dbPath := getDBPath()
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		slog.Error("无法打开数据库", "error", err)
		os.Exit(1)
	}
	slog.Info("📂 使用数据库文件", "path", dbPath)

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		slog.Error("设置 WAL 模式失败", "error", err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL;"); err != nil {
		slog.Error("设置 synchronous 失败", "error", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		slog.Error("设置 busy_timeout 失败", "error", err)
	}

	// Schema 演进：通过 migrations 执行
	migrateOldRoutesIfNeeded()
	runMigrations()

	go dbWriter()
}

// runMigrations 按文件名排序执行 SQL 迁移脚本，通过 _migrations 表追踪已执行的迁移
// 每个迁移脚本只会被执行一次，避免重复执行导致数据丢失（如 DROP TABLE）
func runMigrations() {
	// 创建迁移追踪表
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS _migrations (filename TEXT PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		slog.Error("无法创建迁移追踪表", "error", err)
		os.Exit(1)
	}

	files, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		slog.Error("无法读取迁移文件夹", "error", err)
		os.Exit(1)
	}

	var fileNames []string
	for _, f := range files {
		fileNames = append(fileNames, f.Name())
	}
	sort.Strings(fileNames)

	for _, name := range fileNames {
		// 跳过已执行过的迁移
		var exists int
		if err := db.QueryRow("SELECT COUNT(*) FROM _migrations WHERE filename = ?", name).Scan(&exists); err == nil && exists > 0 {
			slog.Debug("⏭️ 迁移已执行，跳过", "file", name)
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			slog.Error("无法读取迁移文件", "file", name, "error", err)
			os.Exit(1)
		}
		if _, err := db.Exec(string(content)); err != nil {
			slog.Error("执行迁移文件失败", "file", name, "error", err)
			os.Exit(1)
		}

		// 记录已执行的迁移
		if _, err := db.Exec("INSERT INTO _migrations (filename) VALUES (?)", name); err != nil {
			slog.Error("记录迁移状态失败", "file", name, "error", err)
		}

		slog.Info("✅ 成功加载数据库表结构", "file", name)
	}
}

// migrateOldRoutesIfNeeded 检测 sys_routes 是否为旧版 schema (match_model/node_id/target_model)
// 若是，则将旧数据迁移至新版 schema (source_protocol/target_protocol/model_mappings)
// 此函数仅在首次升级时执行一次，之后 _migrations 追踪表会阻止重复执行
func migrateOldRoutesIfNeeded() {
	// 检查 sys_routes 表是否存在
	var tableCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sys_routes'").Scan(&tableCount); err != nil || tableCount == 0 {
		return
	}

	// 检查是否存在新版列（source_protocol），若已存在则跳过
	var newColCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sys_routes') WHERE name='source_protocol'").Scan(&newColCount); err == nil && newColCount > 0 {
		return
	}

	// 检查是否存在旧版列（match_model），若无则说明是其他未知状态，跳过
	var oldColCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sys_routes') WHERE name='match_model'").Scan(&oldColCount); err != nil || oldColCount == 0 {
		return
	}

	slog.Info("🔄 检测到旧版 sys_routes 表结构，开始迁移数据...")

	// 在事务中执行迁移
	tx, err := db.Begin()
	if err != nil {
		slog.Error("迁移事务启动失败", "error", err)
		return
	}
	defer tx.Rollback()

	// 创建新版表
	if _, err := tx.Exec(`
		CREATE TABLE sys_routes_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_protocol TEXT NOT NULL DEFAULT 'openai',
			target_protocol TEXT NOT NULL DEFAULT 'openai',
			model_mappings TEXT DEFAULT '[]',
			status INTEGER NOT NULL DEFAULT 1
		)
	`); err != nil {
		slog.Error("创建新版 sys_routes 表失败", "error", err)
		return
	}

	// 迁移旧数据：match_model → model_mappings[0].match, target_model → model_mappings[0].target
	if _, err := tx.Exec(`
		INSERT INTO sys_routes_new (id, source_protocol, target_protocol, model_mappings, status)
		SELECT id, 'openai', 'openai',
			json_array(json_object('match', match_model, 'target', target_model)),
			status
		FROM sys_routes
	`); err != nil {
		slog.Error("迁移旧路由数据失败", "error", err)
		return
	}

	// 替换旧表
	if _, err := tx.Exec("DROP TABLE sys_routes"); err != nil {
		slog.Error("删除旧版 sys_routes 表失败", "error", err)
		return
	}
	if _, err := tx.Exec("ALTER TABLE sys_routes_new RENAME TO sys_routes"); err != nil {
		slog.Error("重命名新版 sys_routes 表失败", "error", err)
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("迁移事务提交失败", "error", err)
		return
	}

	var migratedCount int
	db.QueryRow("SELECT COUNT(*) FROM sys_routes").Scan(&migratedCount)
	slog.Info("✅ 旧版路由数据迁移完成", "migrated_routes", migratedCount)
}

// dbWriter 后台异步写入协程，从 channel 读取用量记录并写入 SQLite
// 使用 channel 解耦保证 API 响应路径不会被数据库 I/O 阻塞
func dbWriter() {
	for logEntry := range logChan {
		_, err := db.Exec(
			"INSERT INTO account_logs (platform, node_name, client_id, method_name, prompt_tokens, completion_tokens, cost_usd, status_code) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			logEntry.Platform, logEntry.NodeName, logEntry.ClientID, logEntry.MethodName, logEntry.Prompt, logEntry.Completion, logEntry.Cost, logEntry.Status,
		)
		if err != nil {
			slog.Error("写入数据库失败", "node", logEntry.NodeName, "error", err)
		}
	}
}

func CloseDB() {
	if db != nil {
		close(logChan)
		db.Close()
	}
}

// SaveUsage 异步保存用量记录，要求调用方显式传入 platform 来源
// 不阻塞调用方，通过 channel 发送给后台 dbWriter 协程异步写入
func SaveUsage(platform, name, client, method string, prompt, completion int64, cost float64, status int) {
	logChan <- UsageLog{
		Platform:   platform,
		NodeName:   name,
		ClientID:   client,
		MethodName:  method,
		Prompt:      prompt,
		Completion:  completion,
		Cost:        cost,
		Status:      status,
	}
}

func GetTotalCost(name string) float64 {
	if db == nil {
		return 0
	}
	var total float64
	query := "SELECT COALESCE(SUM(cost_usd), 0) FROM account_logs WHERE node_name = ?"
	err := db.QueryRow(query, name).Scan(&total)
	if err != nil && err != sql.ErrNoRows {
		return 0
	}
	return total
}

func GetConsumedSince(name string, startDate string) float64 {
	if db == nil {
		return 0
	}
	var total float64
	query := `
		SELECT COALESCE(SUM(cost_usd), 0) 
		FROM account_logs 
		WHERE node_name = ? 
		AND datetime(created_at, 'localtime') >= datetime(?)`
	err := db.QueryRow(query, name, startDate).Scan(&total)
	if err != nil && err != sql.ErrNoRows {
		slog.Error("查询账期消耗失败", "error", err)
		return 0
	}
	return total
}
