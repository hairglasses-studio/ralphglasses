
# OAI CLI - OpenAI API Integration & Content Scraping

A comprehensive OpenAI API integration and content scraping system designed for high-performance content processing, automated workflows, and intelligent content management with support for multiple AI models and advanced scraping capabilities.

## 🎯 Development Principles

This project follows the [AFTRS Development Principles](../PRINCIPLES.md) which emphasize:
- **Senior/Staff DevOps Engineer Approach**: Independent decision making and complete product focus
- **Performance Optimization**: Speed and efficiency in all operations
- **Comprehensive Documentation**: Self-documenting code and clear guides
- **AI Integration**: Persistent context and decision logging
- **UNRAID Server Integration**: Leveraging the Tailscale-connected infrastructure

## 🚀 Quick Start

```bash
# Install and setup
./install.sh

# Start content scraping
./oai_cli.py scrape --url https://example.com

# Process with AI
./oai_cli.py process --input /path/to/content

# Start web interface
./start_web.sh
```

### Prerequisites

- **Python 3.11+** with pip
- **PostgreSQL 12+** database
- **Chrome/Chromium** browser (for web scraping)
- **Docker** (optional, for containerized deployment)

### 🛠️ Installation

#### Option 1: Automated Installation (Recommended)

```bash
# Clone the repository
git clone https://github.com/hairglasses/oai_cli.git
cd oai_cli

# Run the automated installation script
chmod +x install.sh
./install.sh
```

The installation script will:
- ✅ Check and install system dependencies
- ✅ Install Python 3.11+ if needed
- ✅ Install Chrome/Chromium and ChromeDriver
- ✅ Create virtual environment and install Python packages
- ✅ Setup configuration files and directories
- ✅ Create startup scripts
- ✅ Run basic tests

#### Option 2: Manual Installation

```bash
# Clone the repository
git clone https://github.com/hairglasses/oai_cli.git
cd oai_cli

# Create virtual environment
python3 -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate

# Install dependencies
pip install -r requirements.txt

# Setup environment
cp .env.example .env
# Edit .env with your database credentials

# Create directories
mkdir -p downloads/{images,videos,chats} logs backups
```

### 🗄️ Database Setup

#### Using Docker Compose (Recommended)

```bash
# Start PostgreSQL with Docker Compose
docker-compose up -d db

# Wait for database to be ready, then run schema
docker-compose exec db psql -U oai_user -d oai_archive -f /docker-entrypoint-initdb.d/01-init.sql
```

#### Manual PostgreSQL Setup

```sql
-- Create database and user
CREATE DATABASE oai_archive;
CREATE USER oai_user WITH PASSWORD 'oai_password';
GRANT ALL PRIVILEGES ON DATABASE oai_archive TO oai_user;

-- Run the schema file
psql -d oai_archive -f sora_archive_schema.sql
```

### ⚙️ Configuration

Edit the `.env` file with your settings:

```bash
# Database Configuration
PG_CONN_STR=postgresql://oai_user:oai_password@localhost:5432/oai_archive

# Storage Configuration
DOWNLOAD_DIR=./downloads
IMAGES_DIR=./downloads/images
VIDEOS_DIR=./downloads/videos
CHATS_DIR=./downloads/chats

# Web Interface
WEB_HOST=0.0.0.0
WEB_PORT=8080
WEB_DEBUG=false

# Chrome Configuration
CHROME_HEADLESS=true
CHROME_WINDOW_SIZE=1920,1080
CHROME_DISABLE_GPU=true

# Archive Settings
MAX_RETRIES=3
DOWNLOAD_TIMEOUT=30
BATCH_SIZE=50

# Chat Settings
CHAT_SCROLL_PAUSE=2.0
CHAT_MAX_MESSAGES=1000

# Logging
LOG_LEVEL=INFO
LOG_FILE=./logs/oai_cli.log
```

## 📖 Usage

### 🖥️ CLI Commands

```bash
# Archive specific content types
./start.sh archive images     # Archive images only
./start.sh archive videos     # Archive Sora videos only
./start.sh archive chats      # Archive chat conversations
./start.sh archive all        # Archive everything

# View statistics
./start.sh stats              # Show archive statistics

# Health check
python monitoring/health_check.py

# Logging test
python logging_config.py --level INFO
```

### 📚 **Usage Examples**

#### 🖼️ **Image Archiving Example**

```bash
# Start image archiving
python oai_cli.py archive images

# Example workflow:
# 1. Script opens Chrome browser
# 2. Navigates to https://chat.openai.com/media-library
# 3. Prompts: "🔐 Please log into ChatGPT manually and press Enter to continue..."
# 4. You log in to your ChatGPT account
# 5. Press Enter to continue
# 6. Script automatically:
#    - Scrapes all your generated images
#    - Downloads them to ./downloads/images/
#    - Extracts metadata (prompts, timestamps, etc.)
#    - Stores metadata in PostgreSQL database
#    - Generates unique filenames
#    - Handles duplicates automatically

# Example output:
# [INFO] Found 47 images to process
# [INFO] Downloading image: sunset_over_ocean_2024_01_15.png
# [INFO] Successfully downloaded: sunset_over_ocean_2024_01_15.png
# [INFO] Inserted metadata for image: sunset_over_ocean_2024_01_15.png
# [INFO] Image archive completed: 47 downloaded, 47 metadata inserted
```

#### 🎬 **Sora Video Archiving Example**

```bash
# Start Sora video archiving
python oai_cli.py archive videos

# Example workflow:
# 1. Script opens Chrome browser
# 2. Navigates to https://openai.com/sora
# 3. Prompts: "🔐 Please log into OpenAI Sora manually and press Enter to continue..."
# 4. You log in to your OpenAI account with Sora access
# 5. Press Enter to continue
# 6. Script automatically:
#    - Scrapes all your Sora-generated videos
#    - Downloads them to ./downloads/videos/
#    - Extracts video metadata (duration, resolution, FPS)
#    - Associates prompt text with videos
#    - Stores metadata in PostgreSQL database
#    - Handles different video formats (MP4, WebM, etc.)

# Example output:
# [INFO] Found 12 videos to process
# [INFO] Downloading video: cosmic_journey_through_nebula.mp4
# [INFO] Video metadata: 1920x1080, 30fps, 15.2s duration
# [INFO] Successfully downloaded: cosmic_journey_through_nebula.mp4
# [INFO] Inserted metadata for video: cosmic_journey_through_nebula.mp4
# [INFO] Sora video archive completed: 12 downloaded, 12 metadata inserted
```

#### 💬 **Chat Conversation Archiving Example**

```bash
# Start chat conversation archiving
python oai_cli.py archive chats

# Example workflow:
# 1. Script opens Chrome browser
# 2. Navigates to https://chat.openai.com/
# 3. Prompts: "🔐 Please log into ChatGPT manually and press Enter to continue..."
# 4. You log in to your ChatGPT account
# 5. Press Enter to continue
# 6. Script automatically:
#    - Scrapes all your conversation metadata
#    - Downloads full conversation content
#    - Extracts messages, timestamps, and model info
#    - Generates conversation summaries
#    - Saves JSON exports to ./downloads/chats/
#    - Stores metadata in PostgreSQL database
#    - Handles pagination and scrolling

# Example output:
# [INFO] Found 156 conversations to process
# [INFO] Processing conversation: "Python API Development Discussion"
# [INFO] Extracted 47 messages from conversation
# [INFO] Generated summary: "Discussion about Flask API development..."
# [INFO] Saved JSON export: chat_2024_01_15_python_api.json
# [INFO] Inserted metadata for conversation: Python API Development Discussion
# [INFO] Chat archive completed: 156 conversations processed, 156 metadata inserted
```

#### 🔄 **Complete Archive Example**

```bash
# Archive everything (images, videos, and chats)
python oai_cli.py archive all

# This runs all three archiving processes in sequence:
# 1. Image archiving
# 2. Video archiving  
# 3. Chat archiving

# Example output:
# [INFO] Starting complete archive process...
# [INFO] === IMAGE ARCHIVING ===
# [INFO] Found 47 images to process
# [INFO] Image archive completed: 47 downloaded, 47 metadata inserted
# [INFO] === VIDEO ARCHIVING ===
# [INFO] Found 12 videos to process
# [INFO] Video archive completed: 12 downloaded, 12 metadata inserted
# [INFO] === CHAT ARCHIVING ===
# [INFO] Found 156 conversations to process
# [INFO] Chat archive completed: 156 conversations processed, 156 metadata inserted
# [INFO] Complete archive finished successfully!
```

#### 📊 **Statistics and Monitoring**

```bash
# View archive statistics
python oai_cli.py stats

# Example output:
# 📊 OAI Archive Statistics
# ========================
# 
# 🖼️ Images:
#   Total: 47
#   Total size: 156.7 MB
#   Average size: 3.3 MB
#   Date range: 2024-01-01 to 2024-01-15
# 
# 🎬 Videos:
#   Total: 12
#   Total size: 2.1 GB
#   Average duration: 18.5 seconds
#   Average resolution: 1920x1080
# 
# 💬 Chats:
#   Total conversations: 156
#   Total messages: 3,847
#   Average messages per chat: 24.7
#   Date range: 2024-01-01 to 2024-01-15
# 
# 💾 Storage:
#   Total archive size: 2.3 GB
#   Database size: 15.2 MB
#   Backup status: Last backup 2024-01-15 14:30:00
```

#### 🌐 **Web Dashboard Example**

```bash
# Start web dashboard
python web/app.py

# Access dashboard at http://localhost:8080
# Features:
# - Real-time statistics
# - Search across all content types
# - Image/video previews
# - Chat conversation browser
# - Download management
# - API endpoints for integration
```

#### 🔧 **Advanced Usage Examples**

```bash
# Archive with custom configuration
CHROME_HEADLESS=false python oai_cli.py archive images  # Show browser window

# Archive with specific batch size
BATCH_SIZE=10 python oai_cli.py archive chats  # Process 10 chats at a time

# Archive with custom download directory
DOWNLOAD_DIR=/mnt/storage/oai_archive python oai_cli.py archive all

# Run health check before archiving
python monitoring/health_check.py && python oai_cli.py archive all

# Archive with detailed logging
LOG_LEVEL=DEBUG python oai_cli.py archive images

# Backup to Unraid after archiving
python oai_cli.py archive all && ./backup_to_unraid.sh
```

### 🌐 Web Interface

```bash
# Start web dashboard
./start_web.sh

# Access dashboard
open http://localhost:8080
```

### 🔐 **Account Setup Examples**

#### **ChatGPT Account (Images & Chats)**
```bash
# 1. Ensure you have a ChatGPT Plus account for image generation
# 2. Run the archiving command
python oai_cli.py archive images

# 3. When prompted, log into your ChatGPT account:
#    - Email: your-email@example.com
#    - Password: your-password
#    - Complete any 2FA if enabled
#    - Navigate to your media library
#    - Press Enter to continue

# 4. For chat archiving:
python oai_cli.py archive chats
#    - Same login process
#    - Script will access your conversation history
```

#### **OpenAI Sora Account (Videos)**
```bash
# 1. Ensure you have Sora access (currently limited access)
# 2. Run the video archiving command
python oai_cli.py archive videos

# 3. When prompted, log into your OpenAI account:
#    - Email: your-openai-email@example.com
#    - Password: your-openai-password
#    - Complete any 2FA if enabled
#    - Navigate to Sora interface
#    - Press Enter to continue

# Note: Sora access is currently limited to select users
# If you don't have access, the script will handle gracefully
```

#### **Multiple Account Support**
```bash
# You can archive from multiple accounts by running the commands multiple times
# Each run will prompt for login, allowing you to switch accounts

# Example: Archive from two different ChatGPT accounts
python oai_cli.py archive images  # Login to account 1
python oai_cli.py archive images  # Login to account 2

# The database will store all content with unique identifiers
# No conflicts between different accounts
```

## 🔧 **Troubleshooting**

### **Common Issues & Solutions**

#### **ChromeDriver Issues**
```bash
# Problem: ChromeDriver not found or version mismatch
# Solution: Reinstall ChromeDriver
sudo rm -f /usr/local/bin/chromedriver
./install.sh  # This will reinstall ChromeDriver

# Alternative: Manual ChromeDriver installation
CHROME_VERSION=$(google-chrome --version | grep -oE "[0-9]+\.[0-9]+\.[0-9]+" | head -1)
CHROME_MAJOR=$(echo "$CHROME_VERSION" | cut -d. -f1)
wget -O /tmp/chromedriver.zip "https://edgedl.me.gvt1.com/edgedl/chrome/chrome-for-testing/120.0.6099.109/linux64/chromedriver-linux64.zip"
unzip /tmp/chromedriver.zip -d /tmp/
sudo mv /tmp/chromedriver-linux64/chromedriver /usr/local/bin/
sudo chmod +x /usr/local/bin/chromedriver
```

#### **Login Issues**
```bash
# Problem: Script can't find login elements
# Solution: Use non-headless mode for debugging
CHROME_HEADLESS=false python oai_cli.py archive images

# Problem: 2FA blocking automation
# Solution: Complete 2FA manually, then press Enter
# The script waits for manual intervention
```

#### **Database Connection Issues**
```bash
# Problem: PostgreSQL connection failed
# Solution: Check database status
sudo systemctl status postgresql

# Start PostgreSQL if needed
sudo systemctl start postgresql

# Check connection string
echo $PG_CONN_STR

# Test connection
psql $PG_CONN_STR -c "SELECT version();"
```

#### **Download Issues**
```bash
# Problem: Downloads failing or incomplete
# Solution: Check disk space
df -h

# Check download directory permissions
ls -la downloads/

# Increase timeout for slow connections
DOWNLOAD_TIMEOUT=60 python oai_cli.py archive images
```

#### **Memory Issues**
```bash
# Problem: Out of memory during large archives
# Solution: Reduce batch size
BATCH_SIZE=10 python oai_cli.py archive chats

# Monitor memory usage
python monitoring/health_check.py
```

### **Debug Mode**
```bash
# Enable detailed logging
LOG_LEVEL=DEBUG python oai_cli.py archive images

# Show browser window for debugging
CHROME_HEADLESS=false LOG_LEVEL=DEBUG python oai_cli.py archive images

# Run health check first
python monitoring/health_check.py && python oai_cli.py archive images
```

### **Performance Optimization**
```bash
# For large archives, use these settings:
BATCH_SIZE=25          # Process 25 items at a time
CHAT_MAX_MESSAGES=500  # Limit messages per chat
MAX_RETRIES=5          # More retries for reliability
DOWNLOAD_TIMEOUT=45    # Longer timeout for large files
```

### 🐳 Docker Deployment

#### Quick Start with Docker Compose

```bash
# Start the complete stack
docker-compose up -d

# View logs
docker-compose logs -f

# Access web interface
open http://localhost:8080
```

#### Manual Docker Build

```bash
# Build the image
docker build -t oai_cli .

# Run with environment variables
docker run -d \
  --name oai_archive \
  -e PG_CONN_STR="postgresql://user:password@host:5432/oai_archive" \
  -e DOWNLOAD_DIR="/app/downloads" \
  -e CHATS_DIR="/app/downloads/chats" \
  -v $(pwd)/downloads:/app/downloads \
  -p 8080:8080 \
  oai_cli
```

## 🔧 Advanced Usage

### 📊 Monitoring & Health Checks

```bash
# Run comprehensive health check
python monitoring/health_check.py --format json

# Check only critical issues
python monitoring/health_check.py --critical-only

# Monitor system metrics
python -c "
from monitoring.health_check import HealthChecker
from config import Config
checker = HealthChecker(Config.from_env())
report = checker.generate_report()
print(f'Overall Status: {report[\"overall_status\"]}')
"
```

### 🔍 Search & Discovery

```bash
# Search via API
curl "http://localhost:8080/api/search?q=landscape&type=images"

# Get recent content
curl "http://localhost:8080/api/images?limit=20"
curl "http://localhost:8080/api/videos?limit=20"
curl "http://localhost:8080/api/chats?limit=20"

# Get chat messages
curl "http://localhost:8080/api/chats/{conversation_id}/messages"
```

### 📈 Analytics & Reporting

```bash
# Get archive statistics
curl "http://localhost:8080/api/stats"

# Health check endpoint
curl "http://localhost:8080/health"
```

### 🔄 Automated Scheduling

Add to your crontab for automated archiving:

```bash
# Archive everything every 6 hours
0 */6 * * * cd /path/to/oai_cli && ./start.sh archive all

# Archive chats every 2 hours
0 */2 * * * cd /path/to/oai_cli && ./start.sh archive chats

# Health check every hour
0 * * * * cd /path/to/oai_cli && python monitoring/health_check.py --critical-only
```

### 🗂️ Backup & Restore

```bash
# Run backup script
./backup_to_unraid.sh

# Manual backup
rsync -av downloads/ /mnt/backup/oai_archive/
pg_dump oai_archive > backup_$(date +%Y%m%d_%H%M%S).sql
```

## 🏗️ Architecture

```
oai_cli/
├── 📁 Core Application
│   ├── oai_cli.py              # Main CLI application
│   ├── config.py               # Configuration management
│   └── requirements.txt        # Python dependencies
│
├── 📁 Scrapers & Downloaders
│   ├── media_scraper.py       # Image scraping and downloading
│   ├── chat_scraper.py        # Chat conversation scraping
│   └── sora_downloader.py     # Video downloading (placeholder)
│
├── 📁 Database Layer
│   ├── db/
│   │   ├── database.py        # Database connection and operations
│   │   └── insert_image_metadata.py
│   └── sora_archive_schema.sql # Database schema
│
├── 📁 Web Interface
│   ├── web/
│   │   ├── app.py            # Flask web application
│   │   └── templates/        # HTML templates
│   └── static/               # Static assets
│
├── 📁 Monitoring & Logging
│   ├── monitoring/
│   │   └── health_check.py   # System health monitoring
│   └── logging_config.py     # Advanced logging configuration
│
├── 📁 Testing
│   ├── tests/                # Test suite
│   ├── pytest.ini           # Test configuration
│   └── .github/workflows/   # CI/CD pipelines
│
├── 📁 Deployment
│   ├── Dockerfile            # Multi-stage Docker build
│   ├── docker-compose.yml    # Complete stack deployment
│   └── install.sh           # Automated installation script
│
├── 📁 Documentation
│   ├── README.md            # This file
│   └── docs/               # Additional documentation
│
└── 📁 Scripts
    ├── backup_to_unraid.sh  # Unraid backup script
    ├── apply_patch.sh       # Patch application script
    └── crontab.txt         # Cron job examples
```

## 🧪 Testing

### Running Tests

```bash
# Install test dependencies
pip install pytest pytest-cov pytest-mock

# Run all tests
pytest tests/ -v

# Run with coverage
pytest tests/ --cov=. --cov-report=html

# Run specific test categories
pytest tests/ -m unit
pytest tests/ -m integration
pytest tests/ -m database
```

### Test Categories

- **Unit Tests**: Individual component testing
- **Integration Tests**: Database and web interface testing
- **Database Tests**: Database operations and schema testing
- **Web Tests**: API endpoint and web interface testing

## 🔧 Development

### Setting Up Development Environment

```bash
# Clone and setup
git clone https://github.com/hairglasses/oai_cli.git
cd oai_cli

# Install development dependencies
pip install -r requirements.txt
pip install pytest pytest-cov pytest-mock black flake8 mypy

# Setup pre-commit hooks
pre-commit install

# Run code formatting
black .
flake8 .
mypy .
```

### Adding New Features

1. **New Content Type**: Extend scraper classes and database schema
2. **Database Schema**: Update `sora_archive_schema.sql`
3. **Web Interface**: Add routes to `web/app.py` and templates
4. **CLI Commands**: Extend `oai_cli.py` argument parser
5. **Tests**: Add corresponding test files

### Code Style

- **Python**: PEP 8 with Black formatting
- **Type Hints**: Comprehensive type annotations
- **Documentation**: Docstrings for all functions
- **Testing**: Unit tests for all new features

## 🐛 Troubleshooting

### Common Issues

#### Database Connection Issues

```bash
# Check PostgreSQL status
sudo systemctl status postgresql

# Test connection
psql -h localhost -U oai_user -d oai_archive -c "SELECT 1"

# Check connection string
echo $PG_CONN_STR
```

#### Chrome/ChromeDriver Issues

```bash
# Check Chrome installation
google-chrome --version

# Check ChromeDriver
chromedriver --version

# Reinstall ChromeDriver
./install.sh  # This will reinstall ChromeDriver
```

#### Permission Issues

```bash
# Fix directory permissions
sudo chown -R $USER:$USER downloads/ logs/ backups/

# Fix file permissions
chmod +x *.sh
chmod +x web/app.py
```

#### Memory Issues

```bash
# Check system resources
python monitoring/health_check.py

# Reduce batch size in .env
BATCH_SIZE=25

# Enable Chrome headless mode
CHROME_HEADLESS=true
```

### Debug Mode

```bash
# Enable debug logging
export LOG_LEVEL=DEBUG

# Run with verbose output
python oai_cli.py archive images --verbose

# Check logs
tail -f logs/oai_cli.log
```

### Performance Optimization

```bash
# Increase connection pool size
# Edit config.py: maxconn=20

# Optimize batch processing
# Edit .env: BATCH_SIZE=100

# Enable database connection pooling
# Already enabled by default
```

## 📊 API Reference

### REST API Endpoints

#### Archive Statistics
```http
GET /api/stats
```

#### Content Retrieval
```http
GET /api/images?limit=20
GET /api/videos?limit=20
GET /api/chats?limit=20
```

#### Search
```http
GET /api/search?q=query&type=all&limit=50
```

#### Chat Messages
```http
GET /api/chats/{conversation_id}/messages?limit=100
```

#### Health Check
```http
GET /health
```

### Response Formats

All API endpoints return JSON responses with consistent structure:

```json
{
  "success": true,
  "data": [...],
  "timestamp": "2023-01-01T12:00:00Z",
  "count": 10
}
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes
4. Add tests for new functionality
5. Run the test suite: `pytest tests/`
6. Commit your changes: `git commit -m 'Add amazing feature'`
7. Push to the branch: `git push origin feature/amazing-feature`
8. Open a Pull Request

### Code of Conduct

This project adheres to the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Built with ❤️ and Python
- Inspired by the need for reliable AI content archiving
- Powered by Selenium, Flask, and PostgreSQL
- Enhanced with comprehensive chat archiving capabilities
- Community contributions and feedback

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/hairglasses/oai_cli/issues)
- **Discussions**: [GitHub Discussions](https://github.com/hairglasses/oai_cli/discussions)
- **Documentation**: [Wiki](https://github.com/hairglasses/oai_cli/wiki)

---

**Ready to archive your AI creations and conversations? Let's get started! 🚀**

*Built for the AI content creators and archivists of the future.*
