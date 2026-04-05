package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/db"
	"github.com/hairglasses-studio/runmylife/internal/tui"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := flag.String("db", "", "Path to SQLite database (default: from config)")
	ralphDBPath := flag.String("ralph-db", "", "Path to ralphglasses SQLite database for fleet monitoring")
	refresh := flag.Duration("refresh", 60*time.Second, "Auto-refresh interval")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	path := *dbPath
	if path == "" {
		path = cfg.DBPath
	}

	database, err := db.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer database.ForceClose()

	// Optional ralphglasses DB for fleet monitoring
	var ralphDB *sql.DB
	rpath := *ralphDBPath
	if rpath == "" {
		rpath = cfg.RalphDBPath
	}
	if rpath == "" {
		// Default location
		home, _ := os.UserHomeDir()
		if home != "" {
			candidate := filepath.Join(home, ".config", "ralphglasses", "ralph.db")
			if _, err := os.Stat(candidate); err == nil {
				rpath = candidate
			}
		}
	}
	if rpath != "" {
		ralphDB, err = sql.Open("sqlite3", rpath+"?mode=ro")
		if err == nil {
			defer ralphDB.Close()
			if ralphDB.Ping() != nil {
				ralphDB = nil
			}
		} else {
			ralphDB = nil
		}
	}

	sqlDB := database.SqlDB()
	model := tui.NewModel(sqlDB, ralphDB, *refresh)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}
