package discovery

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database manages the SQLite database for rate limit discovery
type Database struct {
	db   *sql.DB
	path string
}

// NewDatabase creates a new discovery database
func NewDatabase(dbPath string) (*Database, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	database := &Database{
		db:   db,
		path: dbPath,
	}

	// Initialize schema
	if err := database.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	return database, nil
}

// initSchema creates the database tables
func (d *Database) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS rate_limits (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		resource_id TEXT NOT NULL,
		host TEXT NOT NULL,
		path TEXT NOT NULL,
		limit_value INTEGER NOT NULL,
		window_seconds INTEGER NOT NULL,
		remaining_value INTEGER DEFAULT 0,
		reset_time INTEGER NOT NULL,
		source TEXT NOT NULL,
		confidence REAL NOT NULL,
		last_seen INTEGER NOT NULL,
		observation_count INTEGER DEFAULT 1,
		successful_requests INTEGER DEFAULT 0,
		failed_requests INTEGER DEFAULT 0,
		last_429_response INTEGER,
		created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
		UNIQUE(resource_id, host, path)
	);

	CREATE TABLE IF NOT EXISTS learning_data (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		resource_id TEXT NOT NULL,
		request_time INTEGER NOT NULL,
		response_code INTEGER NOT NULL,
		success BOOLEAN NOT NULL,
		response_time_ms INTEGER NOT NULL,
		command TEXT NOT NULL,
		host TEXT NOT NULL,
		path TEXT NOT NULL,
		created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
	);

	CREATE INDEX IF NOT EXISTS idx_rate_limits_resource ON rate_limits(resource_id);
	CREATE INDEX IF NOT EXISTS idx_rate_limits_host_path ON rate_limits(host, path);
	CREATE INDEX IF NOT EXISTS idx_rate_limits_last_seen ON rate_limits(last_seen);
	CREATE INDEX IF NOT EXISTS idx_learning_data_resource ON learning_data(resource_id);
	CREATE INDEX IF NOT EXISTS idx_learning_data_time ON learning_data(request_time);
	`

	_, err := d.db.Exec(schema)
	return err
}

// SaveRateLimitInfo saves or updates rate limit information
func (d *Database) SaveRateLimitInfo(info *RateLimitInfo) error {
	// Check if we should update existing record
	existing, err := d.GetRateLimitInfo(info.ResourceID, info.Host, info.Path)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing rate limit info: %w", err)
	}

	now := time.Now().Unix()

	if existing != nil {
		// Update existing record if new info should replace it
		if !existing.ShouldUpdate(info) {
			// Just increment observation count and update last seen
			return d.updateObservation(existing.ResourceID, existing.Host, existing.Path)
		}

		// Update with new information
		query := `
		UPDATE rate_limits 
		SET limit_value = ?, window_seconds = ?, remaining_value = ?, reset_time = ?,
		    source = ?, confidence = ?, last_seen = ?, observation_count = observation_count + 1,
		    successful_requests = ?, failed_requests = ?, last_429_response = ?, updated_at = ?
		WHERE resource_id = ? AND host = ? AND path = ?`

		var last429 *int64
		if info.Last429Response != nil {
			ts := info.Last429Response.Unix()
			last429 = &ts
		}

		_, err = d.db.Exec(query,
			info.Limit, int64(info.Window.Seconds()), info.Remaining, info.ResetTime.Unix(),
			info.Source, info.Confidence, now, info.SuccessfulRequests, info.FailedRequests,
			last429, now, info.ResourceID, info.Host, info.Path)

		return err
	}

	// Insert new record
	query := `
	INSERT INTO rate_limits (
		resource_id, host, path, limit_value, window_seconds, remaining_value, reset_time,
		source, confidence, last_seen, observation_count, successful_requests, failed_requests,
		last_429_response, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var last429 *int64
	if info.Last429Response != nil {
		ts := info.Last429Response.Unix()
		last429 = &ts
	}

	_, err = d.db.Exec(query,
		info.ResourceID, info.Host, info.Path, info.Limit, int64(info.Window.Seconds()),
		info.Remaining, info.ResetTime.Unix(), info.Source, info.Confidence, now,
		info.ObservationCount, info.SuccessfulRequests, info.FailedRequests, last429, now, now)

	return err
}

// GetRateLimitInfo retrieves rate limit information for a resource
func (d *Database) GetRateLimitInfo(resourceID, host, path string) (*RateLimitInfo, error) {
	query := `
	SELECT resource_id, host, path, limit_value, window_seconds, remaining_value, reset_time,
	       source, confidence, last_seen, observation_count, successful_requests, failed_requests,
	       last_429_response
	FROM rate_limits 
	WHERE resource_id = ? AND host = ? AND path = ?`

	row := d.db.QueryRow(query, resourceID, host, path)

	info := &RateLimitInfo{}
	var windowSeconds int64
	var resetTime int64
	var lastSeen int64
	var last429 *int64

	err := row.Scan(
		&info.ResourceID, &info.Host, &info.Path, &info.Limit, &windowSeconds,
		&info.Remaining, &resetTime, &info.Source, &info.Confidence, &lastSeen,
		&info.ObservationCount, &info.SuccessfulRequests, &info.FailedRequests, &last429)

	if err != nil {
		return nil, err
	}

	// Convert timestamps
	info.Window = time.Duration(windowSeconds) * time.Second
	info.ResetTime = time.Unix(resetTime, 0)
	info.LastSeen = time.Unix(lastSeen, 0)

	if last429 != nil {
		t := time.Unix(*last429, 0)
		info.Last429Response = &t
	}

	return info, nil
}

// GetRateLimitInfoByHost retrieves rate limit information by host and path
func (d *Database) GetRateLimitInfoByHost(host, path string) (*RateLimitInfo, error) {
	query := `
	SELECT resource_id, host, path, limit_value, window_seconds, remaining_value, reset_time,
	       source, confidence, last_seen, observation_count, successful_requests, failed_requests,
	       last_429_response
	FROM rate_limits 
	WHERE host = ? AND path = ?
	ORDER BY confidence DESC, last_seen DESC
	LIMIT 1`

	row := d.db.QueryRow(query, host, path)

	info := &RateLimitInfo{}
	var windowSeconds int64
	var resetTime int64
	var lastSeen int64
	var last429 *int64

	err := row.Scan(
		&info.ResourceID, &info.Host, &info.Path, &info.Limit, &windowSeconds,
		&info.Remaining, &resetTime, &info.Source, &info.Confidence, &lastSeen,
		&info.ObservationCount, &info.SuccessfulRequests, &info.FailedRequests, &last429)

	if err != nil {
		return nil, err
	}

	// Convert timestamps
	info.Window = time.Duration(windowSeconds) * time.Second
	info.ResetTime = time.Unix(resetTime, 0)
	info.LastSeen = time.Unix(lastSeen, 0)

	if last429 != nil {
		t := time.Unix(*last429, 0)
		info.Last429Response = &t
	}

	return info, nil
}

// updateObservation increments the observation count for existing rate limit info
func (d *Database) updateObservation(resourceID, host, path string) error {
	query := `
	UPDATE rate_limits 
	SET observation_count = observation_count + 1, last_seen = ?, updated_at = ?
	WHERE resource_id = ? AND host = ? AND path = ?`

	now := time.Now().Unix()
	_, err := d.db.Exec(query, now, now, resourceID, host, path)
	return err
}

// SaveLearningData saves learning data for rate limit discovery
func (d *Database) SaveLearningData(data *LearningData) error {
	query := `
	INSERT INTO learning_data (
		resource_id, request_time, response_code, success, response_time_ms,
		command, host, path
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := d.db.Exec(query,
		data.ResourceID, data.RequestTime.Unix(), data.ResponseCode, data.Success,
		data.ResponseTime.Milliseconds(), data.Command, data.Host, data.Path)

	return err
}

// GetLearningData retrieves learning data for analysis
func (d *Database) GetLearningData(resourceID string, since time.Time) ([]*LearningData, error) {
	query := `
	SELECT resource_id, request_time, response_code, success, response_time_ms,
	       command, host, path
	FROM learning_data 
	WHERE resource_id = ? AND request_time >= ?
	ORDER BY request_time DESC`

	rows, err := d.db.Query(query, resourceID, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*LearningData
	for rows.Next() {
		data := &LearningData{}
		var requestTime int64
		var responseTimeMs int64

		err := rows.Scan(
			&data.ResourceID, &requestTime, &data.ResponseCode, &data.Success,
			&responseTimeMs, &data.Command, &data.Host, &data.Path)

		if err != nil {
			return nil, err
		}

		data.RequestTime = time.Unix(requestTime, 0)
		data.ResponseTime = time.Duration(responseTimeMs) * time.Millisecond

		results = append(results, data)
	}

	return results, rows.Err()
}

// CleanupExpiredData removes old data from the database
func (d *Database) CleanupExpiredData(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge).Unix()

	// Remove expired rate limit info
	_, err := d.db.Exec("DELETE FROM rate_limits WHERE last_seen < ?", cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired rate limits: %w", err)
	}

	// Remove old learning data (keep more learning data than rate limits)
	learningCutoff := time.Now().Add(-maxAge * 2).Unix()
	_, err = d.db.Exec("DELETE FROM learning_data WHERE request_time < ?", learningCutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup old learning data: %w", err)
	}

	return nil
}

// GetStats returns database statistics
func (d *Database) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count rate limits
	var rateLimitCount int
	err := d.db.QueryRow("SELECT COUNT(*) FROM rate_limits").Scan(&rateLimitCount)
	if err != nil {
		return nil, err
	}
	stats["rate_limit_count"] = rateLimitCount

	// Count learning data
	var learningDataCount int
	err = d.db.QueryRow("SELECT COUNT(*) FROM learning_data").Scan(&learningDataCount)
	if err != nil {
		return nil, err
	}
	stats["learning_data_count"] = learningDataCount

	// Get most recent rate limit
	var lastSeen *int64
	err = d.db.QueryRow("SELECT MAX(last_seen) FROM rate_limits").Scan(&lastSeen)
	if err != nil {
		return nil, err
	}
	if lastSeen != nil {
		stats["last_rate_limit_seen"] = time.Unix(*lastSeen, 0)
	}

	// Get database file size
	if info, err := os.Stat(d.path); err == nil {
		stats["database_size_bytes"] = info.Size()
	}

	return stats, nil
}

// ListRateLimits returns all stored rate limit information
func (d *Database) ListRateLimits() ([]*RateLimitInfo, error) {
	query := `
	SELECT resource_id, host, path, limit_value, window_seconds, remaining_value, reset_time,
	       source, confidence, last_seen, observation_count, successful_requests, failed_requests,
	       last_429_response
	FROM rate_limits 
	ORDER BY confidence DESC, last_seen DESC`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*RateLimitInfo
	for rows.Next() {
		info := &RateLimitInfo{}
		var windowSeconds int64
		var resetTime int64
		var lastSeen int64
		var last429 *int64

		err := rows.Scan(
			&info.ResourceID, &info.Host, &info.Path, &info.Limit, &windowSeconds,
			&info.Remaining, &resetTime, &info.Source, &info.Confidence, &lastSeen,
			&info.ObservationCount, &info.SuccessfulRequests, &info.FailedRequests, &last429)

		if err != nil {
			return nil, err
		}

		// Convert timestamps
		info.Window = time.Duration(windowSeconds) * time.Second
		info.ResetTime = time.Unix(resetTime, 0)
		info.LastSeen = time.Unix(lastSeen, 0)

		if last429 != nil {
			t := time.Unix(*last429, 0)
			info.Last429Response = &t
		}

		results = append(results, info)
	}

	return results, rows.Err()
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// GetDefaultDatabasePath returns the default path for the discovery database
func GetDefaultDatabasePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/patience-discovery.db"
	}
	return filepath.Join(homeDir, ".patience", "discovery.db")
}
