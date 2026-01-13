package daemon

import (
	"time"

	"github.com/ztaylor/claude-mon/internal/database"
	"github.com/ztaylor/claude-mon/internal/logger"
)

// CleanupManager handles automated data retention and cleanup
type CleanupManager struct {
	cfg      *Config
	db       CleanupDatabase
	stopCh   chan struct{}
	interval time.Duration
}

// CleanupDatabase defines the database cleanup interface
type CleanupDatabase interface {
	DeleteOldEdits(beforeDate time.Time) (int64, error)
	CapEditsPerSession(sessionID int64, maxEdits int) (int64, error)
	GetDatabaseSize() (int64, error)
	Vacuum() error
}

// NewCleanupManager creates a new cleanup manager
func NewCleanupManager(cfg *Config, db *database.DB) *CleanupManager {
	interval := time.Duration(cfg.Retention.CleanupIntervalHrs) * time.Hour
	if interval == 0 {
		interval = 24 * time.Hour // Default to 24 hours
	}

	return &CleanupManager{
		cfg:      cfg,
		db:       db,
		stopCh:   make(chan struct{}),
		interval: interval,
	}
}

// Start begins the background cleanup goroutine
func (cm *CleanupManager) Start() {
	logger.Log("Starting cleanup manager (interval: %v)", cm.interval)

	go func() {
		ticker := time.NewTicker(cm.interval)
		defer ticker.Stop()

		// Run initial cleanup after a short delay
		time.Sleep(30 * time.Second)
		cm.runCleanup()

		for {
			select {
			case <-ticker.C:
				cm.runCleanup()
			case <-cm.stopCh:
				logger.Log("Cleanup manager stopped")
				return
			}
		}
	}()
}

// Stop stops the cleanup manager
func (cm *CleanupManager) Stop() {
	close(cm.stopCh)
}

// runCleanup executes the cleanup process
func (cm *CleanupManager) runCleanup() {
	logger.Log("Running cleanup...")

	// 1. Delete old records based on retention policy
	if cm.cfg.Retention.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -cm.cfg.Retention.RetentionDays)
		deleted, err := cm.db.DeleteOldEdits(cutoff)
		if err != nil {
			logger.Log("Failed to delete old edits: %v", err)
		} else {
			logger.Log("Deleted %d old edits (older than %v)", deleted, cutoff.Format("2006-01-02"))
		}
	}

	// 2. Cap edits per session
	if cm.cfg.Retention.MaxEditsPerSession > 0 {
		// This would require getting all sessions first, which is a bit complex
		// For now, we'll skip this or implement it in the database layer
		logger.Log("Session edit capping not yet implemented")
	}

	// 3. Check database size and trigger cleanup if needed
	if cm.cfg.Database.MaxDBSizeMB > 0 {
		sizeBytes, err := cm.db.GetDatabaseSize()
		if err != nil {
			logger.Log("Failed to get database size: %v", err)
		} else {
			sizeMB := sizeBytes / (1024 * 1024)
			logger.Log("Database size: %d MB (max: %d MB)", sizeMB, cm.cfg.Database.MaxDBSizeMB)

			if sizeMB > int64(cm.cfg.Database.MaxDBSizeMB) {
				logger.Log("Database size exceeded, triggering cleanup")
				cm.aggressiveCleanup()
			}
		}
	}

	// 4. Run VACUUM if enabled
	if cm.cfg.Retention.AutoVacuum {
		if err := cm.db.Vacuum(); err != nil {
			logger.Log("Vacuum failed: %v", err)
		} else {
			logger.Log("Vacuum completed successfully")
		}
	}
}

// aggressiveCleanup performs more aggressive cleanup when database is too large
func (cm *CleanupManager) aggressiveCleanup() {
	// Delete even older records
	aggressiveCutoff := time.Now().AddDate(0, 0, -cm.cfg.Retention.RetentionDays*2)
	deleted, err := cm.db.DeleteOldEdits(aggressiveCutoff)
	if err != nil {
		logger.Log("Aggressive cleanup failed: %v", err)
	} else {
		logger.Log("Aggressive cleanup deleted %d records", deleted)
	}

	// Run vacuum to reclaim space
	if err := cm.db.Vacuum(); err != nil {
		logger.Log("Post-cleanup vacuum failed: %v", err)
	}
}

// GetDatabaseSize returns the size of the database file in bytes
func (cm *CleanupManager) GetDatabaseSize() (int64, error) {
	return cm.db.GetDatabaseSize()
}
