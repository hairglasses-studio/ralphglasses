#!/bin/bash

# OAI CLI Startup Script

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Starting OAI CLI...${NC}"

# Activate virtual environment
source venv/bin/activate

# Check if database is available
if ! python -c "from db.database import DatabaseManager; from config import Config; db = DatabaseManager(Config.from_env())" 2>/dev/null; then
    echo "⚠️  Database not available. Starting with database disabled."
fi

# Start the application
python oai_cli.py "$@"
