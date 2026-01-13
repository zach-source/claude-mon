-- Schema for claude-mon daemon SQLite database
-- Stores all Claude Code activity across sessions

CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_path TEXT NOT NULL,
    workspace_name TEXT NOT NULL,
    branch TEXT,
    commit_sha TEXT,
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_activity DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(workspace_path, branch)
);

CREATE TABLE IF NOT EXISTS edits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    tool_name TEXT NOT NULL,
    file_path TEXT NOT NULL,
    old_string TEXT,
    new_string TEXT,
    line_num INTEGER,
    line_count INTEGER,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS prompts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER,
    name TEXT NOT NULL,
    description TEXT,
    content TEXT NOT NULL,
    tags TEXT, -- JSON array
    version INTEGER DEFAULT 1,
    is_global BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL,
    UNIQUE(name, session_id)
);

CREATE TABLE IF NOT EXISTS prompt_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    prompt_id INTEGER NOT NULL,
    version INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (prompt_id) REFERENCES prompts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS hooks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    hook_type TEXT NOT NULL, -- 'pre', 'post', etc.
    tool_name TEXT,
    tool_input TEXT, -- JSON
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_edits_session ON edits(session_id);
CREATE INDEX IF NOT EXISTS idx_edits_file ON edits(file_path);
CREATE INDEX IF NOT EXISTS idx_edits_timestamp ON edits(timestamp);
CREATE INDEX IF NOT EXISTS idx_prompts_session ON prompts(session_id);
CREATE INDEX IF NOT EXISTS idx_prompts_name ON prompts(name);
CREATE INDEX IF NOT EXISTS idx_hooks_session ON hooks(session_id);
CREATE INDEX IF NOT EXISTS idx_sessions_workspace ON sessions(workspace_path);

-- View for recent activity
CREATE VIEW IF NOT EXISTS recent_activity AS
SELECT
    e.id,
    e.tool_name,
    e.file_path,
    s.workspace_name,
    s.branch,
    e.timestamp
FROM edits e
JOIN sessions s ON e.session_id = s.id
ORDER BY e.timestamp DESC
LIMIT 100;
