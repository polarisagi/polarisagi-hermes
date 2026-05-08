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
	runMigrations()

	go dbWriter()
}

func runMigrations() {
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
		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			slog.Error("无法读取迁移文件", "file", name, "error", err)
			os.Exit(1)
		}
		if _, err := db.Exec(string(content)); err != nil {
			slog.Error("执行迁移文件失败", "file", name, "error", err)
			os.Exit(1)
		}
		slog.Info("✅ 成功加载数据库表结构", "file", name)
	}
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
