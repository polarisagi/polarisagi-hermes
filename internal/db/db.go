package db

import (
	"database/sql"
	"embed"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func DB() *sql.DB {
	return db
}

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

//go:embed schema.sql
var schemaFS embed.FS

// InitDB 初始化数据库
func InitDB() {
	var err error
	dbPath := getDBPath()
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		slog.Error("❌ Failed to open database", "error", err)
		os.Exit(1)
	}

	slog.Info("📂 使用数据库文件", "path", dbPath)

type UsageLog struct {
	Platform    string // 新增：所属平台标识 (如 "vertex", "openai")
	NodeName   string
	ClientID   string
	MethodName string
	Prompt      int64
	Completion  int64
	Cost        float64
	Status      int
}

var logChan = make(chan UsageLog, 1024)

func InitDB() {
	var err error
	db, err = sql.Open("sqlite", DBFile)
	if err != nil {
		slog.Error("无法打开数据库", "error", err)
		os.Exit(1)
	}

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

// SaveUsage 签名更新，要求调用方显式传入 platform 来源
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
