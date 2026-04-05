#!/usr/bin/env python3
"""
Tests for configuration module
"""

import os
import pytest
from unittest.mock import patch
from config import Config, setup_logging


class TestConfig:
    """Test configuration loading and validation"""
    
    def test_config_from_env_defaults(self):
        """Test configuration loading with default values"""
        with patch.dict(os.environ, {}, clear=True):
            config = Config.from_env()
            
            assert config.pg_conn_str == ""
            assert config.download_dir == "./downloads"
            assert config.web_host == "0.0.0.0"
            assert config.web_port == 8080
            assert config.chrome_headless is True
            assert config.max_retries == 3
    
    def test_config_from_env_custom(self):
        """Test configuration loading with custom environment variables"""
        test_env = {
            "PG_CONN_STR": "postgresql://test:pass@localhost:5432/test",
            "DOWNLOAD_DIR": "/custom/downloads",
            "WEB_HOST": "127.0.0.1",
            "WEB_PORT": "9000",
            "CHROME_HEADLESS": "false",
            "MAX_RETRIES": "5"
        }
        
        with patch.dict(os.environ, test_env, clear=True):
            config = Config.from_env()
            
            assert config.pg_conn_str == "postgresql://test:pass@localhost:5432/test"
            assert config.download_dir == "/custom/downloads"
            assert config.web_host == "127.0.0.1"
            assert config.web_port == 9000
            assert config.chrome_headless is False
            assert config.max_retries == 5
    
    def test_config_validation_success(self):
        """Test successful configuration validation"""
        config = Config(pg_conn_str="postgresql://test:pass@localhost:5432/test")
        assert config.validate() is True
    
    def test_config_validation_failure(self):
        """Test configuration validation failure"""
        config = Config(pg_conn_str="")
        
        with pytest.raises(ValueError, match="PG_CONN_STR environment variable is required"):
            config.validate()
    
    def test_config_dataclass_attributes(self):
        """Test that all expected attributes are present"""
        config = Config.from_env()
        
        # Check that all expected attributes exist
        expected_attrs = [
            'pg_conn_str', 'db_host', 'db_port', 'db_name', 'db_user', 'db_password',
            'download_dir', 'images_dir', 'videos_dir', 'chats_dir', 'backup_dir',
            'web_host', 'web_port', 'web_debug',
            'chrome_headless', 'chrome_window_size', 'chrome_disable_gpu',
            'max_retries', 'download_timeout', 'batch_size',
            'chat_scroll_pause', 'chat_max_messages',
            'log_level', 'log_file'
        ]
        
        for attr in expected_attrs:
            assert hasattr(config, attr), f"Missing attribute: {attr}"
    
    def test_setup_logging(self):
        """Test logging setup"""
        config = Config.from_env()
        
        # Should not raise an exception
        setup_logging(config)
        
        # Verify logging is configured
        import logging
        assert logging.getLogger().level == getattr(logging, config.log_level) 