#!/usr/bin/env python3
"""
Advanced logging configuration for OAI CLI
Provides structured logging with multiple outputs and rotation
"""

import os
import sys
import json
import logging
import logging.handlers
from datetime import datetime
from typing import Dict, Any, Optional
from dataclasses import dataclass, asdict
from config import Config

@dataclass
class LogRecord:
    """Structured log record"""
    timestamp: str
    level: str
    logger: str
    message: str
    module: str
    function: str
    line: int
    extra: Optional[Dict[str, Any]] = None

class StructuredFormatter(logging.Formatter):
    """Structured JSON formatter for logs"""
    
    def format(self, record: logging.LogRecord) -> str:
        """Format log record as structured JSON"""
        log_data = LogRecord(
            timestamp=datetime.fromtimestamp(record.created).isoformat(),
            level=record.levelname,
            logger=record.name,
            message=record.getMessage(),
            module=record.module,
            function=record.funcName,
            line=record.lineno,
            extra=getattr(record, 'extra', None)
        )
        
        return json.dumps(asdict(log_data), default=str)
    
    def formatException(self, record: logging.LogRecord) -> str:
        """Format exception information"""
        if record.exc_info:
            import traceback
            return json.dumps({
                "timestamp": datetime.fromtimestamp(record.created).isoformat(),
                "level": record.levelname,
                "logger": record.name,
                "message": record.getMessage(),
                "module": record.module,
                "function": record.funcName,
                "line": record.lineno,
                "exception": {
                    "type": record.exc_info[0].__name__,
                    "message": str(record.exc_info[1]),
                    "traceback": traceback.format_exception(*record.exc_info)
                }
            }, default=str)
        return self.format(record)

class ColoredFormatter(logging.Formatter):
    """Colored console formatter"""
    
    COLORS = {
        'DEBUG': '\033[36m',    # Cyan
        'INFO': '\033[32m',     # Green
        'WARNING': '\033[33m',  # Yellow
        'ERROR': '\033[31m',    # Red
        'CRITICAL': '\033[35m', # Magenta
        'RESET': '\033[0m'      # Reset
    }
    
    def format(self, record: logging.LogRecord) -> str:
        """Format log record with colors"""
        color = self.COLORS.get(record.levelname, self.COLORS['RESET'])
        reset = self.COLORS['RESET']
        
        # Format the message
        formatted = super().format(record)
        
        # Add colors
        return f"{color}{formatted}{reset}"

class LogManager:
    """Comprehensive logging manager"""
    
    def __init__(self, config: Config):
        self.config = config
        self.loggers = {}
        self._setup_logging()
    
    def _setup_logging(self):
        """Setup comprehensive logging configuration"""
        # Create logs directory
        log_dir = os.path.dirname(self.config.log_file)
        os.makedirs(log_dir, exist_ok=True)
        
        # Configure root logger
        root_logger = logging.getLogger()
        root_logger.setLevel(getattr(logging, self.config.log_level))
        
        # Clear existing handlers
        root_logger.handlers.clear()
        
        # Add console handler with colors
        console_handler = logging.StreamHandler(sys.stdout)
        console_handler.setLevel(logging.INFO)
        console_formatter = ColoredFormatter(
            '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
        )
        console_handler.setFormatter(console_formatter)
        root_logger.addHandler(console_handler)
        
        # Add file handler with rotation
        file_handler = logging.handlers.RotatingFileHandler(
            self.config.log_file,
            maxBytes=10*1024*1024,  # 10MB
            backupCount=5
        )
        file_handler.setLevel(logging.DEBUG)
        file_formatter = logging.Formatter(
            '%(asctime)s - %(name)s - %(levelname)s - %(module)s:%(funcName)s:%(lineno)d - %(message)s'
        )
        file_handler.setFormatter(file_formatter)
        root_logger.addHandler(file_handler)
        
        # Add structured JSON handler
        json_handler = logging.handlers.RotatingFileHandler(
            self.config.log_file.replace('.log', '_structured.json'),
            maxBytes=10*1024*1024,  # 10MB
            backupCount=5
        )
        json_handler.setLevel(logging.DEBUG)
        json_formatter = StructuredFormatter()
        json_handler.setFormatter(json_formatter)
        root_logger.addHandler(json_handler)
        
        # Add error handler for critical errors
        error_handler = logging.handlers.RotatingFileHandler(
            self.config.log_file.replace('.log', '_errors.log'),
            maxBytes=5*1024*1024,  # 5MB
            backupCount=3
        )
        error_handler.setLevel(logging.ERROR)
        error_formatter = logging.Formatter(
            '%(asctime)s - %(name)s - %(levelname)s - %(module)s:%(funcName)s:%(lineno)d - %(message)s\n'
            'Exception: %(exc_info)s\n'
        )
        error_handler.setFormatter(error_formatter)
        root_logger.addHandler(error_handler)
    
    def get_logger(self, name: str) -> logging.Logger:
        """Get a logger with the specified name"""
        if name not in self.loggers:
            self.loggers[name] = logging.getLogger(name)
        return self.loggers[name]
    
    def log_archive_session(self, session_type: str, results: Dict[str, Any]):
        """Log archive session results"""
        logger = self.get_logger('archive.session')
        
        log_data = {
            'session_type': session_type,
            'timestamp': datetime.now().isoformat(),
            'results': results,
            'success': results.get('success', False),
            'items_processed': results.get('downloaded', 0),
            'items_found': results.get('total_found', 0)
        }
        
        if results.get('success'):
            logger.info(f"Archive session completed successfully", extra={'archive_data': log_data})
        else:
            logger.error(f"Archive session failed: {results.get('message', 'Unknown error')}", 
                        extra={'archive_data': log_data})
    
    def log_database_operation(self, operation: str, table: str, success: bool, 
                              details: Optional[Dict[str, Any]] = None):
        """Log database operations"""
        logger = self.get_logger('database.operations')
        
        log_data = {
            'operation': operation,
            'table': table,
            'success': success,
            'timestamp': datetime.now().isoformat(),
            'details': details or {}
        }
        
        if success:
            logger.info(f"Database operation successful: {operation} on {table}", 
                       extra={'db_data': log_data})
        else:
            logger.error(f"Database operation failed: {operation} on {table}", 
                        extra={'db_data': log_data})
    
    def log_web_request(self, endpoint: str, method: str, status_code: int, 
                       response_time: float, user_agent: str = None):
        """Log web interface requests"""
        logger = self.get_logger('web.requests')
        
        log_data = {
            'endpoint': endpoint,
            'method': method,
            'status_code': status_code,
            'response_time': response_time,
            'user_agent': user_agent,
            'timestamp': datetime.now().isoformat()
        }
        
        if status_code < 400:
            logger.info(f"Web request: {method} {endpoint} - {status_code} ({response_time:.3f}s)", 
                       extra={'web_data': log_data})
        else:
            logger.warning(f"Web request failed: {method} {endpoint} - {status_code} ({response_time:.3f}s)", 
                          extra={'web_data': log_data})
    
    def log_system_metrics(self, metrics: Dict[str, Any]):
        """Log system metrics"""
        logger = self.get_logger('system.metrics')
        
        log_data = {
            'timestamp': datetime.now().isoformat(),
            'metrics': metrics
        }
        
        logger.info("System metrics recorded", extra={'metrics_data': log_data})
    
    def log_security_event(self, event_type: str, details: Dict[str, Any]):
        """Log security events"""
        logger = self.get_logger('security.events')
        
        log_data = {
            'event_type': event_type,
            'timestamp': datetime.now().isoformat(),
            'details': details
        }
        
        logger.warning(f"Security event: {event_type}", extra={'security_data': log_data})
    
    def cleanup_old_logs(self, days_to_keep: int = 30):
        """Clean up old log files"""
        import glob
        from datetime import datetime, timedelta
        
        log_dir = os.path.dirname(self.config.log_file)
        cutoff_date = datetime.now() - timedelta(days=days_to_keep)
        
        # Find old log files
        log_patterns = [
            os.path.join(log_dir, "*.log"),
            os.path.join(log_dir, "*.log.*"),
            os.path.join(log_dir, "*_structured.json"),
            os.path.join(log_dir, "*_structured.json.*"),
            os.path.join(log_dir, "*_errors.log"),
            os.path.join(log_dir, "*_errors.log.*")
        ]
        
        files_removed = 0
        for pattern in log_patterns:
            for log_file in glob.glob(pattern):
                try:
                    file_time = datetime.fromtimestamp(os.path.getmtime(log_file))
                    if file_time < cutoff_date:
                        os.remove(log_file)
                        files_removed += 1
                        self.get_logger('system.cleanup').info(f"Removed old log file: {log_file}")
                except Exception as e:
                    self.get_logger('system.cleanup').error(f"Failed to remove old log file {log_file}: {e}")
        
        return files_removed

def setup_logging(config: Config) -> LogManager:
    """Setup and return logging manager"""
    return LogManager(config)

def main():
    """Test logging configuration"""
    import argparse
    
    parser = argparse.ArgumentParser(description="OAI CLI Logging Test")
    parser.add_argument("--level", choices=["DEBUG", "INFO", "WARNING", "ERROR"], 
                       default="INFO", help="Log level to test")
    
    args = parser.parse_args()
    
    # Load configuration
    config = Config.from_env()
    config.log_level = args.level
    config.validate()
    
    # Setup logging
    log_manager = setup_logging(config)
    
    # Test different loggers
    logger = log_manager.get_logger('test')
    
    logger.debug("This is a debug message")
    logger.info("This is an info message")
    logger.warning("This is a warning message")
    logger.error("This is an error message")
    
    # Test structured logging
    log_manager.log_archive_session("images", {
        'success': True,
        'downloaded': 10,
        'total_found': 15
    })
    
    log_manager.log_database_operation("INSERT", "images", True, {
        'rows_affected': 1,
        'table': 'images'
    })
    
    print("Logging test completed. Check log files for output.")

if __name__ == "__main__":
    main() 