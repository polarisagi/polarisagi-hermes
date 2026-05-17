package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".polaris-gateway", "polaris_gateway.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT name, provider, base_url, credentials, project_id, location FROM sys_nodes WHERE provider='google'")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, provider, baseURL, credentials string
		var projectID, location sql.NullString
		if err := rows.Scan(&name, &provider, &baseURL, &credentials, &projectID, &location); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Provider: %s\nBaseURL: %s\nCreds: %s\nProjectID: %s\nLocation: %s\n", provider, baseURL, credentials, projectID.String, location.String)
	}
}
