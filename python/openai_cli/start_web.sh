#!/bin/bash

# OAI CLI Web Interface Startup Script

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Starting OAI CLI Web Interface...${NC}"

# Activate virtual environment
source venv/bin/activate

# Start web interface
python web/app.py
