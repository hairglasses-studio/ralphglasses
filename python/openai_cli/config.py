import os
from dataclasses import dataclass
from typing import Optional
import logging

@dataclass
class Config:
    # Database
    pg_conn_str: str
    db_host: str = "localhost"
    db_port: int = 5432
    db_name: str = "oai_archive"
    db_user: str = "postgres"
    db_password: str = ""
    
    # Storage
    download_dir: str = "./downloads"
    images_dir: str = "./downloads/images"
    videos_dir: str = "./downloads/videos"
    chats_dir: str = "./downloads/chats"
    backup_dir: str = "/mnt/unraid/backups/oai_archive"
    
    # Web Interface
    web_host: str = "0.0.0.0"
    web_port: int = 8080
    web_debug: bool = False
    
    # Selenium/Chrome
    chrome_headless: bool = True
    chrome_window_size: str = "1920,1080"
    chrome_disable_gpu: bool = True
    
    # Archive Settings
    max_retries: int = 3
    download_timeout: int = 30
    batch_size: int = 50
    
    # Chat Settings
    chat_scroll_pause: float = 2.0
    chat_max_messages: int = 1000
    
    # Logging
    log_level: str = "INFO"
    log_file: str = "/app/oai_cli.log"
    
    @classmethod
    def from_env(cls) -> 'Config':
        """Load configuration from environment variables"""
        return cls(
            pg_conn_str=os.getenv("PG_CONN_STR", ""),
            db_host=os.getenv("DB_HOST", "localhost"),
            db_port=int(os.getenv("DB_PORT", "5432")),
            db_name=os.getenv("DB_NAME", "oai_archive"),
            db_user=os.getenv("DB_USER", "postgres"),
            db_password=os.getenv("DB_PASSWORD", ""),
            download_dir=os.getenv("DOWNLOAD_DIR", "./downloads"),
            images_dir=os.getenv("IMAGES_DIR", "./downloads/images"),
            videos_dir=os.getenv("VIDEOS_DIR", "./downloads/videos"),
            chats_dir=os.getenv("CHATS_DIR", "./downloads/chats"),
            backup_dir=os.getenv("BACKUP_DIR", "/mnt/unraid/backups/oai_archive"),
            web_host=os.getenv("WEB_HOST", "0.0.0.0"),
            web_port=int(os.getenv("WEB_PORT", "8080")),
            web_debug=os.getenv("WEB_DEBUG", "false").lower() == "true",
            chrome_headless=os.getenv("CHROME_HEADLESS", "true").lower() == "true",
            chrome_window_size=os.getenv("CHROME_WINDOW_SIZE", "1920,1080"),
            chrome_disable_gpu=os.getenv("CHROME_DISABLE_GPU", "true").lower() == "true",
            max_retries=int(os.getenv("MAX_RETRIES", "3")),
            download_timeout=int(os.getenv("DOWNLOAD_TIMEOUT", "30")),
            batch_size=int(os.getenv("BATCH_SIZE", "50")),
            chat_scroll_pause=float(os.getenv("CHAT_SCROLL_PAUSE", "2.0")),
            chat_max_messages=int(os.getenv("CHAT_MAX_MESSAGES", "1000")),
            log_level=os.getenv("LOG_LEVEL", "INFO"),
            log_file=os.getenv("LOG_FILE", "/app/oai_cli.log")
        )
    
    def validate(self) -> bool:
        """Validate required configuration"""
        if not self.pg_conn_str:
            raise ValueError("PG_CONN_STR environment variable is required")
        return True

def setup_logging(config: Config):
    """Configure logging with file and console output"""
    logging.basicConfig(
        level=getattr(logging, config.log_level),
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
        handlers=[
            logging.FileHandler(config.log_file),
            logging.StreamHandler()
        ]
    ) 