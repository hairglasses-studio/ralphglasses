#!/usr/bin/env python3
"""
Tests for database module
"""

import pytest
from unittest.mock import Mock, patch, MagicMock
from config import Config
from db.database import DatabaseManager


class TestDatabaseManager:
    """Test database manager functionality"""
    
    @pytest.fixture
    def mock_config(self):
        """Create a mock configuration"""
        config = Mock(spec=Config)
        config.pg_conn_str = "postgresql://test:pass@localhost:5432/test"
        return config
    
    @pytest.fixture
    def mock_pool(self):
        """Create a mock connection pool"""
        pool = Mock()
        conn = Mock()
        cur = Mock()
        
        # Setup mock chain
        pool.getconn.return_value = conn
        conn.cursor.return_value.__enter__.return_value = cur
        conn.cursor.return_value.__exit__.return_value = None
        
        return pool, conn, cur
    
    def test_database_manager_init(self, mock_config):
        """Test database manager initialization"""
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool = Mock()
            mock_pool_class.return_value = mock_pool
            
            db = DatabaseManager(mock_config)
            
            assert db.config == mock_config
            assert db.pool == mock_pool
            mock_pool_class.assert_called_once()
    
    def test_get_connection_context_manager(self, mock_config, mock_pool):
        """Test connection context manager"""
        pool, conn, cur = mock_pool
        
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool_class.return_value = pool
            
            db = DatabaseManager(mock_config)
            
            with db.get_connection() as connection:
                assert connection == conn
            
            pool.getconn.assert_called_once()
            pool.putconn.assert_called_once_with(conn)
    
    def test_execute_query_success(self, mock_config, mock_pool):
        """Test successful query execution"""
        pool, conn, cur = mock_pool
        
        # Mock query results
        cur.fetchall.return_value = [{'id': 1, 'name': 'test'}]
        cur.description = [('id',), ('name',)]
        
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool_class.return_value = pool
            
            db = DatabaseManager(mock_config)
            
            result = db.execute_query("SELECT * FROM test", (1,))
            
            assert result == [{'id': 1, 'name': 'test'}]
            cur.execute.assert_called_once_with("SELECT * FROM test", (1,))
    
    def test_execute_query_no_results(self, mock_config, mock_pool):
        """Test query execution with no results"""
        pool, conn, cur = mock_pool
        
        # Mock no results
        cur.fetchall.return_value = []
        cur.description = None
        
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool_class.return_value = pool
            
            db = DatabaseManager(mock_config)
            
            result = db.execute_query("INSERT INTO test VALUES (%s)", (1,))
            
            assert result == []
    
    def test_execute_command_success(self, mock_config, mock_pool):
        """Test successful command execution"""
        pool, conn, cur = mock_pool
        
        # Mock command result
        cur.rowcount = 5
        
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool_class.return_value = pool
            
            db = DatabaseManager(mock_config)
            
            result = db.execute_command("INSERT INTO test VALUES (%s)", (1,))
            
            assert result == 5
            cur.execute.assert_called_once_with("INSERT INTO test VALUES (%s)", (1,))
            conn.commit.assert_called_once()
    
    def test_insert_image_metadata_success(self, mock_config, mock_pool):
        """Test successful image metadata insertion"""
        pool, conn, cur = mock_pool
        
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool_class.return_value = pool
            
            db = DatabaseManager(mock_config)
            
            result = db.insert_image_metadata(
                image_id="test_id",
                filename="test.jpg",
                prompt_hash="hash123",
                created_at="2023-01-01 12:00:00",
                tags=["test", "image"]
            )
            
            assert result is True
            cur.execute.assert_called_once()
    
    def test_insert_image_metadata_failure(self, mock_config, mock_pool):
        """Test failed image metadata insertion"""
        pool, conn, cur = mock_pool
        
        # Mock database error
        cur.execute.side_effect = Exception("Database error")
        
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool_class.return_value = pool
            
            db = DatabaseManager(mock_config)
            
            result = db.insert_image_metadata(
                image_id="test_id",
                filename="test.jpg",
                prompt_hash="hash123",
                created_at="2023-01-01 12:00:00",
                tags=["test", "image"]
            )
            
            assert result is False
    
    def test_get_archive_stats_success(self, mock_config, mock_pool):
        """Test successful archive stats retrieval"""
        pool, conn, cur = mock_pool
        
        # Mock stats results
        cur.fetchall.return_value = [{
            'total_images': 10,
            'total_videos': 5,
            'total_chats': 3,
            'recent_images': 2,
            'recent_videos': 1,
            'recent_chats': 1,
            'total_file_size': 1024000,
            'last_archive_session': '2023-01-01 12:00:00'
        }]
        cur.description = [('total_images',), ('total_videos',), ('total_chats',),
                          ('recent_images',), ('recent_videos',), ('recent_chats',),
                          ('total_file_size',), ('last_archive_session',)]
        
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool_class.return_value = pool
            
            db = DatabaseManager(mock_config)
            
            stats = db.get_archive_stats()
            
            assert stats['total_images'] == 10
            assert stats['total_videos'] == 5
            assert stats['total_chats'] == 3
            assert stats['total_file_size'] == 1024000
    
    def test_search_content(self, mock_config, mock_pool):
        """Test content search functionality"""
        pool, conn, cur = mock_pool
        
        # Mock search results
        cur.fetchall.return_value = [{'id': 1, 'title': 'test image'}]
        cur.description = [('id',), ('title',)]
        
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool_class.return_value = pool
            
            db = DatabaseManager(mock_config)
            
            results = db.search_content("test", "images", 10)
            
            assert 'images' in results
            assert len(results['images']) == 1
            assert results['images'][0]['title'] == 'test image'
    
    def test_close_pool(self, mock_config, mock_pool):
        """Test connection pool cleanup"""
        pool, conn, cur = mock_pool
        
        with patch('db.database.SimpleConnectionPool') as mock_pool_class:
            mock_pool_class.return_value = pool
            
            db = DatabaseManager(mock_config)
            db.close()
            
            pool.closeall.assert_called_once() 