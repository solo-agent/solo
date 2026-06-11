// Command migrate applies (or rolls back) SQL migrations from the migrations/
// directory, tracking applied versions in a schema_migrations table.
//
// Usage:
//
//	migrate up    # apply all pending *.up.sql migrations
//	migrate down  # roll back all applied *.down.sql migrations
//
// The DATABASE_URL environment variable controls the target database.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultDatabaseURL = "postgres://solo:solo-dev@localhost:5432/solo?sslmode=disable"
	maxSearchDepth     = 4
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: migrate <up|down>")
		os.Exit(1)
	}
	direction := os.Args[1]
	if direction != "up" && direction != "down" {
		fmt.Println("Usage: migrate <up|down>")
		os.Exit(1)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("unable to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("unable to ping database", "error", err)
		os.Exit(1)
	}

	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		slog.Error("failed to create migrations table", "error", err)
		os.Exit(1)
	}

	files, err := findMigrations(direction)
	if err != nil {
		slog.Error("failed to find migration files", "error", err)
		os.Exit(1)
	}

	for _, file := range files {
		version := extractVersion(file)

		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version,
		).Scan(&exists); err != nil {
			slog.Error("failed to check migration status", "version", version, "error", err)
			os.Exit(1)
		}
		if direction == "up" && exists {
			fmt.Printf("  skip  %s (already applied)\n", version)
			continue
		}
		if direction == "down" && !exists {
			fmt.Printf("  skip  %s (not applied)\n", version)
			continue
		}

		sql, err := os.ReadFile(file)
		if err != nil {
			slog.Error("failed to read migration file", "file", file, "error", err)
			os.Exit(1)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			slog.Error("failed to run migration", "file", file, "error", err)
			os.Exit(1)
		}

		if direction == "up" {
			_, err = pool.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
		} else {
			_, err = pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", version)
		}
		if err != nil {
			slog.Error("failed to record migration", "version", version, "error", err)
			os.Exit(1)
		}

		fmt.Printf("  %s    %s\n", direction, version)
	}

	fmt.Println("Done.")
}

// findMigrations returns sorted *.up.sql (forward) or *.down.sql (reverse) files
// from the nearest migrations/ directory.
func findMigrations(direction string) ([]string, error) {
	dir, err := resolveDir()
	if err != nil {
		return nil, err
	}
	suffix := "." + direction + ".sql"
	files, err := filepath.Glob(filepath.Join(dir, "*"+suffix))
	if err != nil {
		return nil, err
	}
	if direction == "down" {
		sort.Sort(sort.Reverse(sort.StringSlice(files)))
	} else {
		sort.Strings(files)
	}
	return files, nil
}

// resolveDir walks up from CWD and the executable directory to find a
// migrations/ directory, so the binary works both when run from the repo
// root (`go run ./cmd/migrate`) and from a deploy artifact directory.
func resolveDir() (string, error) {
	seen := map[string]bool{}
	for _, root := range searchRoots() {
		base := root
		for i := 0; i <= maxSearchDepth; i++ {
			dir := filepath.Clean(filepath.Join(base, "migrations"))
			if !seen[dir] {
				seen[dir] = true
				if info, err := os.Stat(dir); err == nil && info.IsDir() {
					return dir, nil
				}
			}
			base = filepath.Join(base, "..")
		}
	}
	return "", fmt.Errorf("migrations/ directory not found")
}

func searchRoots() []string {
	roots := []string{"."}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}
	return roots
}

func extractVersion(filename string) string {
	base := filepath.Base(filename)
	base = strings.TrimSuffix(base, ".up.sql")
	base = strings.TrimSuffix(base, ".down.sql")
	return base
}
