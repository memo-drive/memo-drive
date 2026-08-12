package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/memodrive/backend/internal/admin"
	"github.com/memodrive/backend/internal/config"
)

var appVersion = "dev"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout))
}

func run(ctx context.Context, args []string, stdout io.Writer) int {
	if len(args) == 0 {
		writeResult(stdout, map[string]any{"command": "", "success": false, "error": "command is required"})
		return 2
	}
	switch args[0] {
	case "backup":
		flags := flag.NewFlagSet("backup", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		output := flags.String("output", "", "backup output directory")
		archivePath := flags.String("archive", "", "optional ZIP64 archive path")
		envFile := flags.String("env-file", "", "MemoDrive env file")
		if err := flags.Parse(args[1:]); err != nil || *output == "" {
			writeResult(stdout, map[string]any{"command": "backup", "success": false, "error": "--output is required"})
			return 2
		}
		cfg, err := loadConfig(*envFile)
		if err != nil {
			writeResult(stdout, map[string]any{"command": "backup", "success": false, "error": err.Error()})
			return 1
		}
		summary, err := admin.New(cfg, appVersion).Backup(ctx, *output)
		if err != nil {
			writeResult(stdout, map[string]any{"command": "backup", "success": false, "error": err.Error()})
			return 1
		}
		if *archivePath != "" {
			if err := admin.ArchiveBackup(summary.BackupPath, *archivePath); err != nil {
				writeResult(stdout, map[string]any{"command": "backup", "success": false, "backup_path": summary.BackupPath, "error": err.Error()})
				return 1
			}
			summary.ArchivePath = *archivePath
		}
		writeResult(stdout, summary)
		return 0
	case "verify":
		flags := flag.NewFlagSet("verify", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		backupPath := flags.String("backup", "", "backup directory")
		sample := flags.Int("sample", 0, "number of files to sample; zero verifies all files")
		if err := flags.Parse(args[1:]); err != nil || *backupPath == "" || *sample < 0 {
			writeResult(stdout, map[string]any{"command": "verify", "success": false, "error": "--backup is required and --sample must not be negative"})
			return 2
		}
		summary, err := admin.New(nil, appVersion).Verify(ctx, *backupPath, *sample)
		if err != nil {
			writeResult(stdout, map[string]any{"command": "verify", "success": false, "error": err.Error()})
			return 1
		}
		writeResult(stdout, summary)
		return 0
	case "integrity":
		flags := flag.NewFlagSet("integrity", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		envFile := flags.String("env-file", "", "MemoDrive env file")
		if err := flags.Parse(args[1:]); err != nil {
			writeResult(stdout, map[string]any{"command": "integrity", "success": false, "error": "invalid arguments"})
			return 2
		}
		cfg, err := loadConfig(*envFile)
		if err != nil {
			writeResult(stdout, map[string]any{"command": "integrity", "success": false, "error": err.Error()})
			return 1
		}
		summary, err := admin.New(cfg, appVersion).Integrity(ctx)
		if err != nil {
			writeResult(stdout, map[string]any{"command": "integrity", "success": false, "error": err.Error()})
			return 1
		}
		writeResult(stdout, summary)
		return 0
	case "restore":
		flags := flag.NewFlagSet("restore", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		backupPath := flags.String("backup", "", "backup directory")
		targetRoot := flags.String("target-root", "", "restored storage root")
		targetDB := flags.String("target-db", "", "restored SQLite database path")
		force := flags.Bool("force", false, "replace non-empty targets")
		if err := flags.Parse(args[1:]); err != nil || *backupPath == "" || *targetRoot == "" || *targetDB == "" {
			writeResult(stdout, map[string]any{"command": "restore", "success": false, "error": "--backup, --target-root, and --target-db are required"})
			return 2
		}
		summary, err := admin.New(nil, appVersion).Restore(ctx, admin.RestoreOptions{
			BackupPath: *backupPath,
			TargetRoot: *targetRoot,
			TargetDB:   *targetDB,
			Force:      *force,
		})
		if err != nil {
			writeResult(stdout, map[string]any{"command": "restore", "success": false, "error": err.Error()})
			return 1
		}
		writeResult(stdout, summary)
		return 0
	case "reindex":
		flags := flag.NewFlagSet("reindex", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		all := flags.Bool("all", false, "rebuild all derived data")
		envFile := flags.String("env-file", "", "MemoDrive env file")
		if err := flags.Parse(args[1:]); err != nil || !*all {
			writeResult(stdout, map[string]any{"command": "reindex", "success": false, "error": "--all is required"})
			return 2
		}
		cfg, err := loadConfig(*envFile)
		if err != nil {
			writeResult(stdout, map[string]any{"command": "reindex", "success": false, "error": err.Error()})
			return 1
		}
		summary, err := admin.New(cfg, appVersion).ReindexAll(ctx)
		if err != nil {
			writeResult(stdout, map[string]any{
				"command":         "reindex",
				"success":         false,
				"error":           err.Error(),
				"total_files":     summary.TotalFiles,
				"reindexed_files": summary.ReindexedFiles,
				"failed_files":    summary.FailedFiles,
			})
			return 1
		}
		writeResult(stdout, summary)
		return 0
	default:
		writeResult(stdout, map[string]any{"command": args[0], "success": false, "error": fmt.Sprintf("unknown command %q", args[0])})
		return 2
	}
}

func loadConfig(envFile string) (*config.Config, error) {
	if envFile != "" {
		return config.LoadFromEnvFile(envFile)
	}
	return config.Load()
}

func writeResult(output io.Writer, value any) {
	_ = json.NewEncoder(output).Encode(value)
}
