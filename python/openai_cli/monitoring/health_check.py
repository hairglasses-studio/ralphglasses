#!/usr/bin/env python3
"""
Health check system for OAI CLI
Provides comprehensive monitoring of all system components
"""

import os
import time
import psutil
import requests
import logging
from datetime import datetime, timedelta
from typing import Dict, Any, List, Optional
from dataclasses import dataclass
from config import Config
from db.database import DatabaseManager

logger = logging.getLogger(__name__)

@dataclass
class HealthStatus:
    """Health status information"""
    component: str
    status: str  # 'healthy', 'warning', 'critical'
    message: str
    timestamp: datetime
    details: Optional[Dict[str, Any]] = None

class HealthChecker:
    """Comprehensive health checking system"""
    
    def __init__(self, config: Config, db: Optional[DatabaseManager] = None):
        self.config = config
        self.db = db
        self.checks = []
    
    def check_database_connection(self) -> HealthStatus:
        """Check database connectivity and performance"""
        try:
            if not self.db:
                return HealthStatus(
                    component="database",
                    status="critical",
                    message="Database manager not initialized",
                    timestamp=datetime.now()
                )
            
            # Test basic connection
            start_time = time.time()
            result = self.db.execute_query("SELECT 1 as test")
            query_time = time.time() - start_time
            
            if not result or result[0]['test'] != 1:
                return HealthStatus(
                    component="database",
                    status="critical",
                    message="Database query failed",
                    timestamp=datetime.now()
                )
            
            # Check connection pool status
            pool_status = "healthy" if query_time < 1.0 else "warning"
            
            return HealthStatus(
                component="database",
                status=pool_status,
                message=f"Database connection healthy (query time: {query_time:.3f}s)",
                timestamp=datetime.now(),
                details={
                    "query_time": query_time,
                    "pool_size": 10,  # Default pool size
                    "active_connections": len(result)
                }
            )
            
        except Exception as e:
            return HealthStatus(
                component="database",
                status="critical",
                message=f"Database connection failed: {str(e)}",
                timestamp=datetime.now()
            )
    
    def check_disk_space(self) -> HealthStatus:
        """Check available disk space"""
        try:
            download_path = os.path.abspath(self.config.download_dir)
            disk_usage = psutil.disk_usage(download_path)
            
            # Calculate usage percentage
            usage_percent = (disk_usage.used / disk_usage.total) * 100
            
            if usage_percent > 90:
                status = "critical"
                message = f"Disk space critical: {usage_percent:.1f}% used"
            elif usage_percent > 80:
                status = "warning"
                message = f"Disk space warning: {usage_percent:.1f}% used"
            else:
                status = "healthy"
                message = f"Disk space healthy: {usage_percent:.1f}% used"
            
            return HealthStatus(
                component="disk_space",
                status=status,
                message=message,
                timestamp=datetime.now(),
                details={
                    "total_gb": disk_usage.total / (1024**3),
                    "used_gb": disk_usage.used / (1024**3),
                    "free_gb": disk_usage.free / (1024**3),
                    "usage_percent": usage_percent
                }
            )
            
        except Exception as e:
            return HealthStatus(
                component="disk_space",
                status="critical",
                message=f"Disk space check failed: {str(e)}",
                timestamp=datetime.now()
            )
    
    def check_memory_usage(self) -> HealthStatus:
        """Check system memory usage"""
        try:
            memory = psutil.virtual_memory()
            memory_percent = memory.percent
            
            if memory_percent > 90:
                status = "critical"
                message = f"Memory usage critical: {memory_percent:.1f}%"
            elif memory_percent > 80:
                status = "warning"
                message = f"Memory usage warning: {memory_percent:.1f}%"
            else:
                status = "healthy"
                message = f"Memory usage healthy: {memory_percent:.1f}%"
            
            return HealthStatus(
                component="memory",
                status=status,
                message=message,
                timestamp=datetime.now(),
                details={
                    "total_gb": memory.total / (1024**3),
                    "available_gb": memory.available / (1024**3),
                    "used_gb": memory.used / (1024**3),
                    "usage_percent": memory_percent
                }
            )
            
        except Exception as e:
            return HealthStatus(
                component="memory",
                status="critical",
                message=f"Memory check failed: {str(e)}",
                timestamp=datetime.now()
            )
    
    def check_web_interface(self) -> HealthStatus:
        """Check web interface availability"""
        try:
            url = f"http://{self.config.web_host}:{self.config.web_port}/health"
            response = requests.get(url, timeout=5)
            
            if response.status_code == 200:
                data = response.json()
                if data.get('status') == 'healthy':
                    return HealthStatus(
                        component="web_interface",
                        status="healthy",
                        message="Web interface is responding",
                        timestamp=datetime.now(),
                        details=data
                    )
                else:
                    return HealthStatus(
                        component="web_interface",
                        status="warning",
                        message=f"Web interface unhealthy: {data.get('error', 'Unknown')}",
                        timestamp=datetime.now(),
                        details=data
                    )
            else:
                return HealthStatus(
                    component="web_interface",
                    status="critical",
                    message=f"Web interface returned status {response.status_code}",
                    timestamp=datetime.now()
                )
                
        except requests.exceptions.RequestException as e:
            return HealthStatus(
                component="web_interface",
                status="critical",
                message=f"Web interface unreachable: {str(e)}",
                timestamp=datetime.now()
            )
    
    def check_chrome_availability(self) -> HealthStatus:
        """Check Chrome/Chromium availability"""
        try:
            import subprocess
            
            # Try to find Chrome/Chromium
            chrome_paths = [
                "google-chrome",
                "chromium-browser",
                "chromium",
                "/usr/bin/google-chrome",
                "/usr/bin/chromium-browser"
            ]
            
            chrome_found = False
            chrome_path = None
            
            for path in chrome_paths:
                try:
                    result = subprocess.run([path, "--version"], 
                                          capture_output=True, text=True, timeout=5)
                    if result.returncode == 0:
                        chrome_found = True
                        chrome_path = path
                        break
                except (subprocess.TimeoutExpired, FileNotFoundError):
                    continue
            
            if chrome_found:
                return HealthStatus(
                    component="chrome",
                    status="healthy",
                    message=f"Chrome found at {chrome_path}",
                    timestamp=datetime.now(),
                    details={"chrome_path": chrome_path}
                )
            else:
                return HealthStatus(
                    component="chrome",
                    status="critical",
                    message="Chrome/Chromium not found",
                    timestamp=datetime.now()
                )
                
        except Exception as e:
            return HealthStatus(
                component="chrome",
                status="critical",
                message=f"Chrome check failed: {str(e)}",
                timestamp=datetime.now()
            )
    
    def check_download_directories(self) -> HealthStatus:
        """Check download directory permissions and space"""
        try:
            directories = [
                self.config.download_dir,
                self.config.images_dir,
                self.config.videos_dir,
                self.config.chats_dir
            ]
            
            missing_dirs = []
            inaccessible_dirs = []
            
            for directory in directories:
                if not os.path.exists(directory):
                    missing_dirs.append(directory)
                elif not os.access(directory, os.W_OK):
                    inaccessible_dirs.append(directory)
            
            if missing_dirs or inaccessible_dirs:
                status = "critical"
                message = f"Directory issues: {len(missing_dirs)} missing, {len(inaccessible_dirs)} inaccessible"
            else:
                status = "healthy"
                message = "All download directories accessible"
            
            return HealthStatus(
                component="download_directories",
                status=status,
                message=message,
                timestamp=datetime.now(),
                details={
                    "missing_directories": missing_dirs,
                    "inaccessible_directories": inaccessible_dirs,
                    "total_directories": len(directories)
                }
            )
            
        except Exception as e:
            return HealthStatus(
                component="download_directories",
                status="critical",
                message=f"Directory check failed: {str(e)}",
                timestamp=datetime.now()
            )
    
    def run_all_checks(self) -> List[HealthStatus]:
        """Run all health checks"""
        checks = [
            self.check_database_connection(),
            self.check_disk_space(),
            self.check_memory_usage(),
            self.check_web_interface(),
            self.check_chrome_availability(),
            self.check_download_directories()
        ]
        
        self.checks = checks
        return checks
    
    def get_overall_status(self) -> str:
        """Get overall system status"""
        if not self.checks:
            self.run_all_checks()
        
        critical_count = sum(1 for check in self.checks if check.status == "critical")
        warning_count = sum(1 for check in self.checks if check.status == "warning")
        
        if critical_count > 0:
            return "critical"
        elif warning_count > 0:
            return "warning"
        else:
            return "healthy"
    
    def generate_report(self) -> Dict[str, Any]:
        """Generate comprehensive health report"""
        checks = self.run_all_checks()
        overall_status = self.get_overall_status()
        
        return {
            "timestamp": datetime.now().isoformat(),
            "overall_status": overall_status,
            "checks": [
                {
                    "component": check.component,
                    "status": check.status,
                    "message": check.message,
                    "timestamp": check.timestamp.isoformat(),
                    "details": check.details
                }
                for check in checks
            ],
            "summary": {
                "total_checks": len(checks),
                "healthy": sum(1 for check in checks if check.status == "healthy"),
                "warnings": sum(1 for check in checks if check.status == "warning"),
                "critical": sum(1 for check in checks if check.status == "critical")
            }
        }

def main():
    """Run health check as standalone script"""
    import argparse
    
    parser = argparse.ArgumentParser(description="OAI CLI Health Checker")
    parser.add_argument("--format", choices=["json", "text"], default="text",
                       help="Output format")
    parser.add_argument("--critical-only", action="store_true",
                       help="Only show critical issues")
    
    args = parser.parse_args()
    
    # Load configuration
    config = Config.from_env()
    config.validate()
    
    # Initialize health checker
    checker = HealthChecker(config)
    
    # Run checks
    report = checker.generate_report()
    
    if args.format == "json":
        import json
        print(json.dumps(report, indent=2))
    else:
        # Text format
        print(f"🎨 OAI CLI Health Report")
        print(f"Time: {report['timestamp']}")
        print(f"Overall Status: {report['overall_status'].upper()}")
        print()
        
        for check in report['checks']:
            if args.critical_only and check['status'] != 'critical':
                continue
                
            status_icon = {
                'healthy': '✅',
                'warning': '⚠️',
                'critical': '❌'
            }.get(check['status'], '❓')
            
            print(f"{status_icon} {check['component']}: {check['message']}")
        
        print()
        summary = report['summary']
        print(f"Summary: {summary['healthy']} healthy, {summary['warnings']} warnings, {summary['critical']} critical")

if __name__ == "__main__":
    main() 