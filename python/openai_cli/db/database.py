import psycopg2
from psycopg2.pool import SimpleConnectionPool
from psycopg2.extras import RealDictCursor
import logging
from typing import List, Dict, Any, Optional, Tuple
from contextlib import contextmanager
import os
from config import Config

logger = logging.getLogger(__name__)

class DatabaseManager:
    def __init__(self, config: Config):
        self.config = config
        self.pool: Optional[SimpleConnectionPool] = None
        self._init_pool()
    
    def _init_pool(self):
        """Initialize connection pool"""
        try:
            self.pool = SimpleConnectionPool(
                minconn=1,
                maxconn=10,
                dsn=self.config.pg_conn_str,
                cursor_factory=RealDictCursor
            )
            logger.info("Database connection pool initialized")
        except Exception as e:
            logger.error(f"Failed to initialize database pool: {e}")
            raise
    
    @contextmanager
    def get_connection(self):
        """Context manager for database connections"""
        conn = None
        try:
            if self.pool is None:
                raise RuntimeError("Database pool not initialized")
            conn = self.pool.getconn()
            yield conn
        except Exception as e:
            if conn:
                conn.rollback()
            logger.error(f"Database operation failed: {e}")
            raise
        finally:
            if conn and self.pool:
                self.pool.putconn(conn)
    
    def execute_query(self, query: str, params: Optional[Tuple[Any, ...]] = None) -> List[Dict[str, Any]]:
        """Execute a query and return results as list of dicts"""
        with self.get_connection() as conn:
            with conn.cursor() as cur:
                cur.execute(query, params)
                if cur.description:
                    return [dict(row) for row in cur.fetchall()]
                return []
    
    def execute_command(self, command: str, params: Optional[Tuple[Any, ...]] = None) -> int:
        """Execute a command and return number of affected rows"""
        with self.get_connection() as conn:
            with conn.cursor() as cur:
                cur.execute(command, params)
                conn.commit()
                return cur.rowcount
    
    def insert_image_metadata(self, image_id: str, filename: str, prompt_hash: str, 
                            created_at: str, tags: List[str]) -> bool:
        """Insert image metadata with conflict handling"""
        query = """
            INSERT INTO images (image_id, filename, prompt_hash, created_at, tags, archived, archived_at)
            VALUES (%s, %s, %s, %s, %s, TRUE, NOW())
            ON CONFLICT (image_id) DO UPDATE SET
                filename = EXCLUDED.filename,
                prompt_hash = EXCLUDED.prompt_hash,
                tags = EXCLUDED.tags,
                archived = TRUE,
                archived_at = NOW()
        """
        try:
            self.execute_command(query, (image_id, filename, prompt_hash, created_at, tags))
            logger.info(f"Inserted/updated image metadata for {image_id}")
            return True
        except Exception as e:
            logger.error(f"Failed to insert image metadata for {image_id}: {e}")
            return False
    
    def insert_video_metadata(self, title: str, filename: str, url: str, 
                            duration_seconds: int, resolution: str, tags: List[str]) -> bool:
        """Insert video metadata with conflict handling"""
        query = """
            INSERT INTO sora_videos (title, filename, url, duration_seconds, resolution, tags)
            VALUES (%s, %s, %s, %s, %s, %s)
            ON CONFLICT (filename) DO UPDATE SET
                title = EXCLUDED.title,
                url = EXCLUDED.url,
                duration_seconds = EXCLUDED.duration_seconds,
                resolution = EXCLUDED.resolution,
                tags = EXCLUDED.tags
        """
        try:
            self.execute_command(query, (title, filename, url, duration_seconds, resolution, tags))
            logger.info(f"Inserted/updated video metadata for {filename}")
            return True
        except Exception as e:
            logger.error(f"Failed to insert video metadata for {filename}: {e}")
            return False
    
    def insert_chat_metadata(self, conversation_id: str, title: str, filename: str,
                           message_count: int, total_tokens: int, model_used: str,
                           participants: List[str], summary: str, file_size: int) -> bool:
        """Insert chat metadata with conflict handling"""
        query = """
            INSERT INTO chat_logs (conversation_id, title, filename, message_count, total_tokens, 
                                 model_used, participants, summary, file_size, archived, archived_at)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, TRUE, NOW())
            ON CONFLICT (conversation_id) DO UPDATE SET
                title = EXCLUDED.title,
                filename = EXCLUDED.filename,
                message_count = EXCLUDED.message_count,
                total_tokens = EXCLUDED.total_tokens,
                model_used = EXCLUDED.model_used,
                participants = EXCLUDED.participants,
                summary = EXCLUDED.summary,
                file_size = EXCLUDED.file_size,
                archived = TRUE,
                archived_at = NOW()
        """
        try:
            self.execute_command(query, (conversation_id, title, filename, message_count, 
                                       total_tokens, model_used, participants, summary, file_size))
            logger.info(f"Inserted/updated chat metadata for {conversation_id}")
            return True
        except Exception as e:
            logger.error(f"Failed to insert chat metadata for {conversation_id}: {e}")
            return False
    
    def insert_chat_message(self, conversation_id: str, message_id: str, role: str,
                          content: str, tokens_used: int, model_used: str, metadata: Dict) -> bool:
        """Insert individual chat message"""
        query = """
            INSERT INTO chat_messages (conversation_id, message_id, role, content, 
                                     tokens_used, model_used, metadata)
            VALUES (%s, %s, %s, %s, %s, %s, %s)
            ON CONFLICT (message_id) DO UPDATE SET
                content = EXCLUDED.content,
                tokens_used = EXCLUDED.tokens_used,
                model_used = EXCLUDED.model_used,
                metadata = EXCLUDED.metadata
        """
        try:
            import json
            metadata_json = json.dumps(metadata) if metadata else None
            self.execute_command(query, (conversation_id, message_id, role, content,
                                       tokens_used, model_used, metadata_json))
            return True
        except Exception as e:
            logger.error(f"Failed to insert chat message {message_id}: {e}")
            return False
    
    def get_recent_images(self, limit: int = 20) -> List[Dict[str, Any]]:
        """Get recent images ordered by creation date"""
        query = """
            SELECT image_id, filename, prompt_hash, created_at, tags, archived_at
            FROM images 
            ORDER BY created_at DESC 
            LIMIT %s
        """
        return self.execute_query(query, (limit,))
    
    def get_recent_videos(self, limit: int = 20) -> List[Dict[str, Any]]:
        """Get recent videos ordered by creation date"""
        query = """
            SELECT title, filename, url, duration_seconds, resolution, tags, created_at
            FROM sora_videos 
            ORDER BY created_at DESC 
            LIMIT %s
        """
        return self.execute_query(query, (limit,))
    
    def get_recent_chats(self, limit: int = 20) -> List[Dict[str, Any]]:
        """Get recent chat conversations ordered by creation date"""
        query = """
            SELECT conversation_id, title, filename, message_count, total_tokens, 
                   model_used, participants, summary, created_at
            FROM chat_logs 
            ORDER BY created_at DESC 
            LIMIT %s
        """
        return self.execute_query(query, (limit,))
    
    def get_chat_messages(self, conversation_id: str, limit: int = 100) -> List[Dict[str, Any]]:
        """Get messages for a specific conversation"""
        query = """
            SELECT message_id, role, content, timestamp, tokens_used, model_used, metadata
            FROM chat_messages 
            WHERE conversation_id = %s
            ORDER BY timestamp ASC 
            LIMIT %s
        """
        return self.execute_query(query, (conversation_id, limit))
    
    def get_archive_stats(self) -> Dict[str, Any]:
        """Get archive statistics using the database function"""
        try:
            # Try to use the database function first
            result = self.execute_query("SELECT * FROM get_archive_stats()")
            if result:
                stats = result[0]
                return {
                    'total_images': stats['total_images'],
                    'total_videos': stats['total_videos'],
                    'total_chats': stats['total_chats'],
                    'recent_images': stats['recent_images'],
                    'recent_videos': stats['recent_videos'],
                    'recent_chats': stats['recent_chats'],
                    'total_file_size': stats['total_file_size'],
                    'last_archive_session': stats['last_archive_session']
                }
        except Exception as e:
            logger.warning(f"Database function not available, using fallback queries: {e}")
        
        # Fallback to individual queries
        queries = {
            'total_images': "SELECT COUNT(*) as count FROM images WHERE archived = true",
            'total_videos': "SELECT COUNT(*) as count FROM sora_videos WHERE archived = true",
            'total_chats': "SELECT COUNT(*) as count FROM chat_logs WHERE archived = true",
            'recent_images': "SELECT COUNT(*) as count FROM images WHERE archived = true AND created_at > NOW() - INTERVAL '24 hours'",
            'recent_videos': "SELECT COUNT(*) as count FROM sora_videos WHERE archived = true AND created_at > NOW() - INTERVAL '24 hours'",
            'recent_chats': "SELECT COUNT(*) as count FROM chat_logs WHERE archived = true AND created_at > NOW() - INTERVAL '24 hours'"
        }
        
        stats = {}
        for key, query in queries.items():
            result = self.execute_query(query)
            stats[key] = result[0]['count'] if result else 0
        
        # Add file size calculation
        size_queries = {
            'images_size': "SELECT COALESCE(SUM(file_size), 0) as size FROM images WHERE archived = true",
            'videos_size': "SELECT COALESCE(SUM(file_size), 0) as size FROM sora_videos WHERE archived = true",
            'chats_size': "SELECT COALESCE(SUM(file_size), 0) as size FROM chat_logs WHERE archived = true"
        }
        
        total_size = 0
        for key, query in size_queries.items():
            result = self.execute_query(query)
            total_size += result[0]['size'] if result else 0
        
        stats['total_file_size'] = total_size
        stats['last_archive_session'] = None
        
        return stats
    
    def search_content(self, query: str, content_type: str = 'all', limit: int = 50) -> Dict[str, List[Dict[str, Any]]]:
        """Search content across all types"""
        results = {
            'images': [],
            'videos': [],
            'chats': []
        }
        
        if content_type in ['images', 'all']:
            images = self.execute_query("""
                SELECT * FROM images 
                WHERE (filename ILIKE %s OR prompt_text ILIKE %s OR tags::text ILIKE %s)
                AND archived = true
                ORDER BY created_at DESC LIMIT %s
            """, (f'%{query}%', f'%{query}%', f'%{query}%', limit))
            results['images'] = images
        
        if content_type in ['videos', 'all']:
            videos = self.execute_query("""
                SELECT * FROM sora_videos 
                WHERE (title ILIKE %s OR prompt_text ILIKE %s OR tags::text ILIKE %s)
                AND archived = true
                ORDER BY created_at DESC LIMIT %s
            """, (f'%{query}%', f'%{query}%', f'%{query}%', limit))
            results['videos'] = videos
        
        if content_type in ['chats', 'all']:
            chats = self.execute_query("""
                SELECT * FROM chat_logs 
                WHERE (title ILIKE %s OR summary ILIKE %s OR participants::text ILIKE %s)
                AND archived = true
                ORDER BY created_at DESC LIMIT %s
            """, (f'%{query}%', f'%{query}%', f'%{query}%', limit))
            results['chats'] = chats
        
        return results
    
    def close(self):
        """Close the connection pool"""
        if self.pool:
            self.pool.closeall()
            logger.info("Database connection pool closed") 