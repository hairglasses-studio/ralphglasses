-- Enhanced OAI Archive Schema
-- Supports images, videos, and chat logs with comprehensive metadata

-- Images table
CREATE TABLE IF NOT EXISTS images (
  id SERIAL PRIMARY KEY,
  image_id TEXT UNIQUE,
  filename TEXT,
  prompt_hash TEXT,
  created_at TIMESTAMP,
  tags TEXT[],
  archived BOOLEAN DEFAULT FALSE,
  archived_at TIMESTAMP,
  file_size BIGINT,
  resolution TEXT,
  model_used TEXT,
  prompt_text TEXT
);

-- Videos table
CREATE TABLE IF NOT EXISTS sora_videos (
  id SERIAL PRIMARY KEY,
  title TEXT,
  filename TEXT UNIQUE,
  url TEXT,
  duration_seconds INT,
  resolution TEXT,
  tags TEXT[],
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  file_size BIGINT,
  model_used TEXT,
  prompt_text TEXT,
  archived BOOLEAN DEFAULT FALSE,
  archived_at TIMESTAMP
);

-- Chat logs table
CREATE TABLE IF NOT EXISTS chat_logs (
  id SERIAL PRIMARY KEY,
  conversation_id TEXT UNIQUE,
  title TEXT,
  filename TEXT UNIQUE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  message_count INT DEFAULT 0,
  total_tokens INT DEFAULT 0,
  model_used TEXT,
  tags TEXT[],
  archived BOOLEAN DEFAULT FALSE,
  archived_at TIMESTAMP,
  file_size BIGINT,
  participants TEXT[],
  summary TEXT
);

-- Chat messages table for detailed conversation tracking
CREATE TABLE IF NOT EXISTS chat_messages (
  id SERIAL PRIMARY KEY,
  conversation_id TEXT REFERENCES chat_logs(conversation_id) ON DELETE CASCADE,
  message_id TEXT UNIQUE,
  role TEXT NOT NULL, -- 'user', 'assistant', 'system'
  content TEXT,
  timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  tokens_used INT DEFAULT 0,
  model_used TEXT,
  metadata JSONB
);

-- Archive sessions table for tracking archiving operations
CREATE TABLE IF NOT EXISTS archive_sessions (
  id SERIAL PRIMARY KEY,
  session_id TEXT UNIQUE,
  session_type TEXT NOT NULL, -- 'images', 'videos', 'chats', 'all'
  started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMP,
  status TEXT DEFAULT 'running', -- 'running', 'completed', 'failed'
  items_found INT DEFAULT 0,
  items_processed INT DEFAULT 0,
  items_successful INT DEFAULT 0,
  items_failed INT DEFAULT 0,
  error_message TEXT,
  metadata JSONB
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_images_created_at ON images(created_at);
CREATE INDEX IF NOT EXISTS idx_images_archived ON images(archived);
CREATE INDEX IF NOT EXISTS idx_videos_created_at ON sora_videos(created_at);
CREATE INDEX IF NOT EXISTS idx_videos_archived ON sora_videos(archived);
CREATE INDEX IF NOT EXISTS idx_chat_logs_created_at ON chat_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_chat_logs_archived ON chat_logs(archived);
CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation_id ON chat_messages(conversation_id);
CREATE INDEX IF NOT EXISTS idx_chat_messages_timestamp ON chat_messages(timestamp);
CREATE INDEX IF NOT EXISTS idx_archive_sessions_started_at ON archive_sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_archive_sessions_status ON archive_sessions(status);

-- Create full-text search indexes
CREATE INDEX IF NOT EXISTS idx_images_prompt_text_gin ON images USING gin(to_tsvector('english', prompt_text));
CREATE INDEX IF NOT EXISTS idx_videos_title_gin ON sora_videos USING gin(to_tsvector('english', title));
CREATE INDEX IF NOT EXISTS idx_chat_logs_title_gin ON chat_logs USING gin(to_tsvector('english', title));

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to automatically update updated_at
CREATE TRIGGER update_chat_logs_updated_at 
    BEFORE UPDATE ON chat_logs 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to get archive statistics
CREATE OR REPLACE FUNCTION get_archive_stats()
RETURNS TABLE(
    total_images BIGINT,
    total_videos BIGINT,
    total_chats BIGINT,
    recent_images BIGINT,
    recent_videos BIGINT,
    recent_chats BIGINT,
    total_file_size BIGINT,
    last_archive_session TIMESTAMP
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        (SELECT COUNT(*) FROM images WHERE archived = true) as total_images,
        (SELECT COUNT(*) FROM sora_videos WHERE archived = true) as total_videos,
        (SELECT COUNT(*) FROM chat_logs WHERE archived = true) as total_chats,
        (SELECT COUNT(*) FROM images WHERE archived = true AND created_at > NOW() - INTERVAL '24 hours') as recent_images,
        (SELECT COUNT(*) FROM sora_videos WHERE archived = true AND created_at > NOW() - INTERVAL '24 hours') as recent_videos,
        (SELECT COUNT(*) FROM chat_logs WHERE archived = true AND created_at > NOW() - INTERVAL '24 hours') as recent_chats,
        COALESCE((SELECT SUM(file_size) FROM images WHERE archived = true), 0) + 
        COALESCE((SELECT SUM(file_size) FROM sora_videos WHERE archived = true), 0) + 
        COALESCE((SELECT SUM(file_size) FROM chat_logs WHERE archived = true), 0) as total_file_size,
        (SELECT MAX(started_at) FROM archive_sessions WHERE status = 'completed') as last_archive_session;
END;
$$ LANGUAGE plpgsql;
