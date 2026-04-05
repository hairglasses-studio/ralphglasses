#!/bin/bash

# Backup script for syncing OAI archive to Unraid storage
# This script syncs downloaded content and database backups to your Unraid server

set -e

# Configuration
SOURCE_DIR="/app/downloads"
BACKUP_DIR="/mnt/unraid/backups/oai_archive"
DB_BACKUP_DIR="/mnt/unraid/backups/oai_archive/database"
LOG_FILE="/app/logs/backup.log"
RETENTION_DAYS=30

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" | tee -a "$LOG_FILE"
}

# Error handling
error_exit() {
    log "${RED}ERROR: $1${NC}" >&2
    exit 1
}

# Check if backup directory exists
check_backup_dir() {
    if [ ! -d "$BACKUP_DIR" ]; then
        log "${YELLOW}Creating backup directory: $BACKUP_DIR${NC}"
        mkdir -p "$BACKUP_DIR" || error_exit "Failed to create backup directory"
    fi
    
    if [ ! -d "$DB_BACKUP_DIR" ]; then
        log "${YELLOW}Creating database backup directory: $DB_BACKUP_DIR${NC}"
        mkdir -p "$DB_BACKUP_DIR" || error_exit "Failed to create database backup directory"
    fi
}

# Database backup function
backup_database() {
    log "${GREEN}Starting database backup...${NC}"
    
    # Get database connection string from environment
    if [ -z "$PG_CONN_STR" ]; then
        error_exit "PG_CONN_STR environment variable not set"
    fi
    
    # Extract connection details
    DB_HOST=$(echo "$PG_CONN_STR" | sed -n 's/.*@\([^:]*\):.*/\1/p')
    DB_PORT=$(echo "$PG_CONN_STR" | sed -n 's/.*:\([0-9]*\)\/.*/\1/p')
    DB_NAME=$(echo "$PG_CONN_STR" | sed -n 's/.*\/\([^?]*\).*/\1/p')
    DB_USER=$(echo "$PG_CONN_STR" | sed -n 's/.*:\/\/\([^:]*\):.*/\1/p')
    DB_PASS=$(echo "$PG_CONN_STR" | sed -n 's/.*:\/\/[^:]*:\([^@]*\)@.*/\1/p')
    
    # Create backup filename with timestamp
    BACKUP_FILE="oai_archive_$(date +%Y%m%d_%H%M%S).sql"
    BACKUP_PATH="$DB_BACKUP_DIR/$BACKUP_FILE"
    
    # Set password for pg_dump
    export PGPASSWORD="$DB_PASS"
    
    # Create database backup
    if pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" > "$BACKUP_PATH"; then
        log "${GREEN}Database backup created: $BACKUP_FILE${NC}"
        
        # Compress the backup
        gzip "$BACKUP_PATH"
        log "${GREEN}Database backup compressed: $BACKUP_FILE.gz${NC}"
    else
        error_exit "Database backup failed"
    fi
    
    # Clean up old backups
    find "$DB_BACKUP_DIR" -name "*.sql.gz" -mtime +$RETENTION_DAYS -delete
    log "${YELLOW}Cleaned up database backups older than $RETENTION_DAYS days${NC}"
}

# Content backup function
backup_content() {
    log "${GREEN}Starting content backup...${NC}"
    
    # Check if source directory exists
    if [ ! -d "$SOURCE_DIR" ]; then
        error_exit "Source directory does not exist: $SOURCE_DIR"
    fi
    
    # Create content backup directory with timestamp
    CONTENT_BACKUP_DIR="$BACKUP_DIR/content_$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$CONTENT_BACKUP_DIR"
    
    # Sync content with rsync
    if rsync -av --delete "$SOURCE_DIR/" "$CONTENT_BACKUP_DIR/"; then
        log "${GREEN}Content backup completed: $(basename "$CONTENT_BACKUP_DIR")${NC}"
        
        # Create a symlink to the latest backup
        ln -sfn "$CONTENT_BACKUP_DIR" "$BACKUP_DIR/latest"
        log "${GREEN}Updated latest symlink${NC}"
    else
        error_exit "Content backup failed"
    fi
    
    # Clean up old content backups
    find "$BACKUP_DIR" -maxdepth 1 -type d -name "content_*" -mtime +$RETENTION_DAYS -exec rm -rf {} \;
    log "${YELLOW}Cleaned up content backups older than $RETENTION_DAYS days${NC}"
}

# Create backup report
create_report() {
    log "${GREEN}Creating backup report...${NC}"
    
    REPORT_FILE="$BACKUP_DIR/backup_report_$(date +%Y%m%d_%H%M%S).txt"
    
    {
        echo "OAI Archive Backup Report"
        echo "========================="
        echo "Date: $(date)"
        echo "Source: $SOURCE_DIR"
        echo "Destination: $BACKUP_DIR"
        echo ""
        echo "Database Backups:"
        ls -lh "$DB_BACKUP_DIR"/*.sql.gz 2>/dev/null || echo "No database backups found"
        echo ""
        echo "Content Backups:"
        ls -lh "$BACKUP_DIR"/content_* 2>/dev/null || echo "No content backups found"
        echo ""
        echo "Latest Backup:"
        if [ -L "$BACKUP_DIR/latest" ]; then
            ls -la "$BACKUP_DIR/latest"
        else
            echo "No latest backup found"
        fi
        echo ""
        echo "Disk Usage:"
        df -h "$BACKUP_DIR"
    } > "$REPORT_FILE"
    
    log "${GREEN}Backup report created: $(basename "$REPORT_FILE")${NC}"
}

# Main execution
main() {
    log "${GREEN}Starting OAI Archive backup process...${NC}"
    
    # Check backup directory
    check_backup_dir
    
    # Create log directory if it doesn't exist
    mkdir -p "$(dirname "$LOG_FILE")"
    
    # Perform backups
    backup_database
    backup_content
    
    # Create report
    create_report
    
    log "${GREEN}Backup process completed successfully!${NC}"
}

# Run main function
main "$@" 