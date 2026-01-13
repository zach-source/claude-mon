package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/ztaylor/claude-mon/internal/database"
)

// Config holds all daemon configuration
type Config struct {
	Directory   DirectoryConfig   `toml:"directory"`
	Database    DatabaseConfig    `toml:"database"`
	Sockets     SocketsConfig     `toml:"sockets"`
	Query       QueryConfig       `toml:"query"`
	Retention   RetentionConfig   `toml:"retention"`
	Backup      BackupConfig      `toml:"backup"`
	Workspaces  WorkspacesConfig  `toml:"workspaces"`
	Hooks       HooksConfig       `toml:"hooks"`
	Logging     LoggingConfig     `toml:"logging"`
	Performance PerformanceConfig `toml:"performance"`
}

// DirectoryConfig holds directory settings
type DirectoryConfig struct {
	DataDir string `toml:"data_dir"`
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	Path               string `toml:"path"`
	MaxDBSizeMB        int    `toml:"max_db_size_mb"`
	WALCheckpointPages int    `toml:"wal_checkpoint_pages"`
}

// SocketsConfig holds socket settings
type SocketsConfig struct {
	DaemonSocket string `toml:"daemon_socket"`
	QuerySocket  string `toml:"query_socket"`
	BufferSize   int    `toml:"buffer_size"`
}

// QueryConfig holds query settings
type QueryConfig struct {
	DefaultLimit int `toml:"default_limit"`
	MaxLimit     int `toml:"max_limit"`
	TimeoutSecs  int `toml:"timeout_seconds"`
}

// RetentionConfig holds data retention settings
type RetentionConfig struct {
	RetentionDays      int  `toml:"retention_days"`
	MaxEditsPerSession int  `toml:"max_edits_per_session"`
	CleanupIntervalHrs int  `toml:"cleanup_interval_hours"`
	AutoVacuum         bool `toml:"auto_vacuum"`
}

// BackupConfig holds backup settings
type BackupConfig struct {
	Enabled       bool   `toml:"enabled"`
	Path          string `toml:"path"`
	IntervalHrs   int    `toml:"interval_hours"`
	RetentionDays int    `toml:"retention_days"`
	Format        string `toml:"format"` // "sqlite" or "export"
}

// WorkspacesConfig holds workspace filtering settings
type WorkspacesConfig struct {
	Tracked []string `toml:"tracked"`
	Ignored []string `toml:"ignored"`
}

// HooksConfig holds hook integration settings
type HooksConfig struct {
	TimeoutSecs   int  `toml:"timeout_seconds"`
	RetryAttempts int  `toml:"retry_attempts"`
	AsyncMode     bool `toml:"async_mode"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Path       string `toml:"path"`
	Level      string `toml:"level"`
	MaxSizeMB  int    `toml:"max_size_mb"`
	MaxBackups int    `toml:"max_backups"`
	Compress   bool   `toml:"compress"`
}

// PerformanceConfig holds performance tuning settings
type PerformanceConfig struct {
	MaxConnections int  `toml:"max_connections"`
	PoolSize       int  `toml:"pool_size"`
	CacheEnabled   bool `toml:"cache_enabled"`
	CacheTTLSecs   int  `toml:"cache_ttl_seconds"`
}

// defaultConfig returns default configuration
func defaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	defaultDataDir := filepath.Join(homeDir, ".claude-mon")

	return &Config{
		Directory: DirectoryConfig{
			DataDir: defaultDataDir,
		},
		Database: DatabaseConfig{
			Path:               "claude-mon.db",
			MaxDBSizeMB:        500,
			WALCheckpointPages: 1000,
		},
		Sockets: SocketsConfig{
			DaemonSocket: "/tmp/claude-mon-daemon.sock",
			QuerySocket:  "/tmp/claude-mon-query.sock",
			BufferSize:   8192,
		},
		Query: QueryConfig{
			DefaultLimit: 50,
			MaxLimit:     1000,
			TimeoutSecs:  30,
		},
		Retention: RetentionConfig{
			RetentionDays:      90,
			MaxEditsPerSession: 10000,
			CleanupIntervalHrs: 24,
			AutoVacuum:         true,
		},
		Backup: BackupConfig{
			Enabled:       true,
			Path:          "backups",
			IntervalHrs:   24,
			RetentionDays: 30,
			Format:        "sqlite",
		},
		Workspaces: WorkspacesConfig{
			Tracked: []string{},
			Ignored: []string{"/tmp", "/var/tmp"},
		},
		Hooks: HooksConfig{
			TimeoutSecs:   30,
			RetryAttempts: 3,
			AsyncMode:     false,
		},
		Logging: LoggingConfig{
			Path:       "claude-mon.log",
			Level:      "info",
			MaxSizeMB:  100,
			MaxBackups: 3,
			Compress:   true,
		},
		Performance: PerformanceConfig{
			MaxConnections: 50,
			PoolSize:       10,
			CacheEnabled:   true,
			CacheTTLSecs:   300,
		},
	}
}

// LoadConfig loads configuration from file, environment variables, and defaults
// Priority: file > env vars > defaults
func LoadConfig(configPath string) (*Config, error) {
	cfg := defaultConfig()

	// Load from file if provided
	if configPath != "" {
		if err := loadConfigFile(cfg, configPath); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	} else {
		// Try default config path
		homeDir, _ := os.UserHomeDir()
		defaultConfigPath := filepath.Join(homeDir, ".config", "claude-mon", "daemon.toml")
		if _, err := os.Stat(defaultConfigPath); err == nil {
			if err := loadConfigFile(cfg, defaultConfigPath); err != nil {
				return nil, fmt.Errorf("failed to load default config file: %w", err)
			}
		}
	}

	// Override with environment variables
	applyEnvVars(cfg)

	// Expand paths
	if err := cfg.expandPaths(); err != nil {
		return nil, fmt.Errorf("failed to expand paths: %w", err)
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// loadConfigFile loads configuration from a TOML file
func loadConfigFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse TOML: %w", err)
	}

	return nil
}

// applyEnvVars applies environment variable overrides
func applyEnvVars(cfg *Config) {
	// Directory
	if v := os.Getenv("CLAUDE_MON_DATA_DIR"); v != "" {
		cfg.Directory.DataDir = v
	}

	// Sockets
	if v := os.Getenv("CLAUDE_MON_DAEMON_SOCKET"); v != "" {
		cfg.Sockets.DaemonSocket = v
	}
	if v := os.Getenv("CLAUDE_MON_QUERY_SOCKET"); v != "" {
		cfg.Sockets.QuerySocket = v
	}
}

// expandPaths expands ~ and relative paths
func (c *Config) expandPaths() error {
	// Expand data directory
	dataDir, err := expandPath(c.Directory.DataDir)
	if err != nil {
		return err
	}
	c.Directory.DataDir = dataDir

	return nil
}

// expandPath expands ~ to home directory and returns absolute path
func expandPath(path string) (string, error) {
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(homeDir, path[1:])
	}

	return filepath.Abs(path)
}

// validate validates the configuration
func (c *Config) validate() error {
	// Validate query limits
	if c.Query.DefaultLimit <= 0 {
		return fmt.Errorf("query.default_limit must be positive")
	}
	if c.Query.MaxLimit <= 0 {
		return fmt.Errorf("query.max_limit must be positive")
	}
	if c.Query.DefaultLimit > c.Query.MaxLimit {
		return fmt.Errorf("query.default_limit cannot exceed max_limit")
	}

	// Validate retention settings
	if c.Retention.RetentionDays < 0 {
		return fmt.Errorf("retention.retention_days cannot be negative")
	}
	if c.Retention.MaxEditsPerSession <= 0 {
		return fmt.Errorf("retention.max_edits_per_session must be positive")
	}

	// Validate backup format
	if c.Backup.Enabled {
		if c.Backup.Format != "sqlite" && c.Backup.Format != "export" {
			return fmt.Errorf("backup.format must be 'sqlite' or 'export'")
		}
	}

	// Validate logging level
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("logging.level must be one of: debug, info, warn, error")
	}

	return nil
}

// GetDBPath returns the absolute database path
func (c *Config) GetDBPath() string {
	return filepath.Join(c.Directory.DataDir, c.Database.Path)
}

// GetLogPath returns the absolute log path
func (c *Config) GetLogPath() string {
	return filepath.Join(c.Directory.DataDir, c.Logging.Path)
}

// GetBackupPath returns the absolute backup path
func (c *Config) GetBackupPath() string {
	return filepath.Join(c.Directory.DataDir, c.Backup.Path)
}

// ToDBConfig converts to database.Config for backwards compatibility
func (c *Config) ToDBConfig() (*database.Config, error) {
	return &database.Config{
		Path: c.GetDBPath(),
	}, nil
}

// ShouldTrackWorkspace checks if a workspace should be tracked
func (c *Config) ShouldTrackWorkspace(workspacePath string) bool {
	// If tracked list is non-empty, only track those
	if len(c.Workspaces.Tracked) > 0 {
		for _, prefix := range c.Workspaces.Tracked {
			if matchPrefix(workspacePath, prefix) {
				// Not in ignored list
				for _, ignored := range c.Workspaces.Ignored {
					if matchPrefix(workspacePath, ignored) {
						return false
					}
				}
				return true
			}
		}
		return false
	}

	// If tracked list is empty, track all except ignored
	for _, ignored := range c.Workspaces.Ignored {
		if matchPrefix(workspacePath, ignored) {
			return false
		}
	}

	return true
}

// matchPrefix checks if path matches prefix
func matchPrefix(path, prefix string) bool {
	return path == prefix || (len(path) > len(prefix) && path[:len(prefix)+1] == prefix+"/")
}

// WriteDefaultConfig writes the default configuration to a file
func WriteDefaultConfig(path string) error {
	cfg := defaultConfig()

	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config file
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// GetEnvInt reads an integer from environment variable with fallback
func GetEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

// GetEnvBool reads a boolean from environment variable with fallback
func GetEnvBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return defaultVal
}
