package daemon

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ztaylor/claude-mon/internal/logger"
)

// BackupManager handles automated backups
type BackupManager struct {
	cfg      *Config
	stopCh   chan struct{}
	interval time.Duration
}

// NewBackupManager creates a new backup manager
func NewBackupManager(cfg *Config) *BackupManager {
	interval := time.Duration(cfg.Backup.IntervalHrs) * time.Hour
	if interval == 0 {
		interval = 24 * time.Hour // Default to 24 hours
	}

	return &BackupManager{
		cfg:      cfg,
		stopCh:   make(chan struct{}),
		interval: interval,
	}
}

// Start begins the background backup goroutine
func (bm *BackupManager) Start() {
	if !bm.cfg.Backup.Enabled {
		logger.Log("Backup manager disabled")
		return
	}

	logger.Log("Starting backup manager (interval: %v)", bm.interval)

	go func() {
		ticker := time.NewTicker(bm.interval)
		defer ticker.Stop()

		// Run initial backup after a short delay
		time.Sleep(60 * time.Second)
		bm.runBackup()

		for {
			select {
			case <-ticker.C:
				bm.runBackup()
			case <-bm.stopCh:
				logger.Log("Backup manager stopped")
				return
			}
		}
	}()
}

// Stop stops the backup manager
func (bm *BackupManager) Stop() {
	close(bm.stopCh)
}

// runBackup executes the backup process
func (bm *BackupManager) runBackup() {
	logger.Log("Running backup...")

	// Create backup directory
	backupPath := bm.cfg.GetBackupPath()
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		logger.Log("Failed to create backup directory: %v", err)
		return
	}

	// Generate backup filename
	timestamp := time.Now().Format("20060102-150405")
	var backupFile string

	if bm.cfg.Backup.Format == "sqlite" {
		backupFile = filepath.Join(backupPath, fmt.Sprintf("claude-mon-%s.db", timestamp))

		// Copy database file
		dbPath := bm.cfg.GetDBPath()
		if err := copyFile(dbPath, backupFile); err != nil {
			logger.Log("Failed to copy database: %v", err)
			return
		}

		logger.Log("SQLite backup created: %s", backupFile)

		// Compress if enabled (create .gz version)
		if err := compressFile(backupFile, backupFile+".gz"); err != nil {
			logger.Log("Failed to compress backup: %v", err)
		} else {
			logger.Log("Backup compressed: %s.gz", backupFile)
			// Remove uncompressed version
			os.Remove(backupFile)
		}

	} else {
		// JSON export format
		backupFile = filepath.Join(backupPath, fmt.Sprintf("claude-mon-%s.json.gz", timestamp))

		if err := bm.exportToJSON(backupFile); err != nil {
			logger.Log("Failed to export to JSON: %v", err)
			return
		}

		logger.Log("JSON backup created: %s", backupFile)
	}

	// Clean up old backups
	bm.cleanupOldBackups()
}

// exportToJSON exports database to JSON format
func (bm *BackupManager) exportToJSON(backupFile string) error {
	// This is a placeholder for actual JSON export
	// In a real implementation, you'd query the database and export to JSON
	// For now, we'll create a simple placeholder

	data := map[string]interface{}{
		"version":   "1.0",
		"timestamp": time.Now().Format(time.RFC3339),
		"format":    "json",
		"note":      "Full JSON export not yet implemented",
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Create gzipped file
	f, err := os.Create(backupFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	if _, err := gw.Write(jsonData); err != nil {
		return err
	}

	return nil
}

// cleanupOldBackups removes backups older than retention period
func (bm *BackupManager) cleanupOldBackups() {
	if bm.cfg.Backup.RetentionDays <= 0 {
		return // No cleanup
	}

	backupPath := bm.cfg.GetBackupPath()
	cutoff := time.Now().AddDate(0, 0, -bm.cfg.Backup.RetentionDays)

	err := filepath.Walk(backupPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file is older than cutoff
		if info.ModTime().Before(cutoff) {
			logger.Log("Deleting old backup: %s (age: %d days)",
				filepath.Base(path), int(time.Since(info.ModTime()).Hours()/24))

			if err := os.Remove(path); err != nil {
				logger.Log("Failed to delete old backup: %v", err)
			}
		}

		return nil
	})

	if err != nil {
		logger.Log("Error cleaning up old backups: %v", err)
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

// compressFile compresses a file with gzip
func compressFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	gw := gzip.NewWriter(destination)
	defer gw.Close()

	_, err = io.Copy(gw, source)
	return err
}
