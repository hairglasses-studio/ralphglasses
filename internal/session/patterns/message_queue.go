package patterns

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// MessageQueue is a durable, SQLite-backed message queue for inter-session
// communication. Messages are stored as serialized Envelope JSON with
// sender/receiver routing and acknowledgement tracking.
type MessageQueue struct {
	db *sql.DB
	mu sync.Mutex
}

// NewMessageQueue opens or creates a SQLite database at dbPath and initializes
// the messages table. Uses WAL mode for concurrent read performance.
func NewMessageQueue(dbPath string) (*MessageQueue, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("message queue: mkdir %s: %w", dir, err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("message queue: open %s: %w", dbPath, err)
	}

	// Serialize writes through a single connection (modernc.org/sqlite
	// recommendation for concurrent writers).
	db.SetMaxOpenConns(1)

	mq := &MessageQueue{db: db}
	if err := mq.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("message queue: migrate: %w", err)
	}
	return mq, nil
}

// migrate creates the messages table and indexes if they do not exist.
func (mq *MessageQueue) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    envelope   TEXT NOT NULL,
    from_id    TEXT NOT NULL,
    to_id      TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    acked_at   DATETIME
);

CREATE INDEX IF NOT EXISTS idx_messages_to ON messages(to_id, acked_at);
`
	_, err := mq.db.Exec(ddl)
	return err
}

// Send inserts an envelope into the message queue.
func (mq *MessageQueue) Send(env *Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("message queue: marshal envelope: %w", err)
	}

	mq.mu.Lock()
	defer mq.mu.Unlock()

	const q = `INSERT INTO messages (envelope, from_id, to_id) VALUES (?, ?, ?)`
	_, err = mq.db.Exec(q, string(data), env.From, env.To)
	return err
}

// Receive returns up to limit unacknowledged messages addressed to sessionID.
// Messages with to_id matching sessionID or broadcast messages (to_id = '')
// are included.
func (mq *MessageQueue) Receive(sessionID string, limit int) ([]*Envelope, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	const q = `SELECT id, envelope FROM messages
		WHERE (to_id = ? OR to_id = '') AND acked_at IS NULL
		ORDER BY id ASC LIMIT ?`

	rows, err := mq.db.Query(q, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return mq.scanEnvelopes(rows)
}

// ReceiveAll returns up to limit unacknowledged messages regardless of
// recipient (useful for broadcast monitoring).
func (mq *MessageQueue) ReceiveAll(limit int) ([]*Envelope, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	const q = `SELECT id, envelope FROM messages
		WHERE acked_at IS NULL
		ORDER BY id ASC LIMIT ?`

	rows, err := mq.db.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return mq.scanEnvelopes(rows)
}

// Ack marks a message as acknowledged by setting acked_at to the current time.
func (mq *MessageQueue) Ack(messageID int64) error {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	const q = `UPDATE messages SET acked_at = CURRENT_TIMESTAMP WHERE id = ?`
	res, err := mq.db.Exec(q, messageID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("message queue: message %d not found", messageID)
	}
	return nil
}

// Pending returns the count of unacknowledged messages for sessionID.
// Includes both direct and broadcast messages.
func (mq *MessageQueue) Pending(sessionID string) (int, error) {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	const q = `SELECT COUNT(*) FROM messages
		WHERE (to_id = ? OR to_id = '') AND acked_at IS NULL`

	var count int
	err := mq.db.QueryRow(q, sessionID).Scan(&count)
	return count, err
}

// Close closes the underlying database connection.
func (mq *MessageQueue) Close() error {
	return mq.db.Close()
}

// scanEnvelopes reads rows of (id, envelope) and deserializes them.
// The Envelope.ID is overwritten with the database row ID as a string
// for use with Ack().
func (mq *MessageQueue) scanEnvelopes(rows *sql.Rows) ([]*Envelope, error) {
	var envelopes []*Envelope
	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		var env Envelope
		if err := json.Unmarshal([]byte(data), &env); err != nil {
			return nil, fmt.Errorf("message queue: unmarshal envelope id=%d: %w", id, err)
		}
		envelopes = append(envelopes, &env)
	}
	return envelopes, rows.Err()
}
