package data

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var db *sql.DB
var logger *log.Logger

func init() {
	logger = log.New(log.Default().Writer(), "[DB] ", log.Default().Flags())
	home, err := os.UserHomeDir()
	if err != nil {
		logger.Fatal(err)
	}
	dbFolder := filepath.Join(home, ".mh-gobot")
	if err := os.MkdirAll(dbFolder, 0700); err != nil {
		logger.Fatal(err)
	}
	logger.Printf("loading db from folder %v", dbFolder)
	dbPath := filepath.Join(dbFolder, "data.db")
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		logger.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		logger.Fatal(err)
	}
	logger.Print("Db online")
	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(1)
	db.Exec("PRAGMA busy_timeout=10000")
	// this will pretty much make writes happen to a log file
	db.Exec("PRAGMA journal_mode=WAL")
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS players (
		player_id TEXT PRIMARY KEY,
		username  TEXT NOT NULL,
		raw_score INTEGER NOT NULL DEFAULT 0,
		score     INTEGER NOT NULL DEFAULT 0,
		kills     INTEGER NOT NULL DEFAULT 0,
		deaths    INTEGER NOT NULL DEFAULT 0,
		assists   INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		logger.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`)
	if err != nil {
		logger.Fatal(err)
	}

}
