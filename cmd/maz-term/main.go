// Command maz-term is a terminal dashboard for local system, HTTP endpoint and
// Git repository metrics.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Teomazivila/maz-term/internal/storage"
	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/ui"
)

// Version is set at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	os.Exit(run())
}

// run holds the real entry point so that deferred cleanup still executes on a
// non-zero exit; main only translates the result into a process status.
func run() int {
	configPath := flag.String("config", "", "path to the configuration file")
	dataPath := flag.String("data-path", "", "path to the metrics database (default <user-dir>/data.db)")
	logPath := flag.String("log-file", "", "path to the log file (default <user-dir>/maz-term.log)")
	versionFlag := flag.Bool("version", false, "print version information and exit")
	noStorage := flag.Bool("no-storage", false, "run without persisting metrics history")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	if *versionFlag {
		fmt.Println("maz-term " + Version)
		return 0
	}

	// The dashboard owns the terminal for its whole lifetime, so logs must
	// never reach stdout or stderr while it runs: any write scrambles the
	// rendered frame. Everything goes to a file instead.
	logFile, err := openLogFile(*logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "maz-term: cannot open log file: %v\n", err)
		return 1
	}
	defer logFile.Close()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	logger.Info("starting maz-term", "version", Version, "log_file", logFile.Name())

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		// A configuration file the user explicitly asked for must never be
		// silently replaced by defaults; a missing file is not an error and is
		// handled inside LoadConfig.
		logger.Error("cannot load configuration", "error", err)
		fmt.Fprintf(os.Stderr, "maz-term: %v\n", err)
		return 1
	}

	app := ui.NewApp(cfg)

	if !*noStorage {
		dbPath := *dataPath
		if dbPath == "" {
			dir, err := config.UserDir()
			if err != nil {
				logger.Error("cannot resolve data directory", "error", err)
				fmt.Fprintf(os.Stderr, "maz-term: %v\n", err)
				return 1
			}
			dbPath = filepath.Join(dir, "data.db")
		}

		db, err := storage.New(&storage.Config{
			DataPath:        dbPath,
			RetentionPeriod: cfg.General.HistoryRetention,
			Logger:          logger,
		})
		if err != nil {
			logger.Error("cannot initialise database", "error", err)
			fmt.Fprintf(os.Stderr, "maz-term: %v\n", err)
			return 1
		}
		defer func() {
			if err := db.Close(); err != nil {
				logger.Error("error closing database", "error", err)
			}
		}()

		adapter := storage.NewAdapter(db)
		// Both calls are order-independent with respect to collector creation:
		// the app records the provider and applies it to every collector as it
		// is built inside Run.
		app.SetStorageProvider(adapter)
		app.SetStorage(adapter)
	} else {
		logger.Info("storage disabled, history will not be recorded")
	}

	// A single context drives shutdown for both paths: an operator pressing q
	// inside the dashboard, and SIGINT/SIGTERM from the outside.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("dashboard exited with an error", "error", err)
		fmt.Fprintf(os.Stderr, "maz-term: %v\n", err)
		return 1
	}

	logger.Info("maz-term stopped")
	return 0
}

// openLogFile opens the log file at path, or at the default location under the
// per-user maz-term directory when path is empty.
func openLogFile(path string) (*os.File, error) {
	if path == "" {
		dir, err := config.UserDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(dir, "maz-term.log")
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating log directory %s: %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", path, err)
	}
	return f, nil
}
