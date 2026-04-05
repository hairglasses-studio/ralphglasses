#!/bin/bash

# OAI CLI Installation Script
# This script installs all dependencies and sets up the environment

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REQUIRED_PYTHON_VERSION="3.11"
REQUIRED_PACKAGES=("python3" "pip3" "git" "curl" "wget")
OPTIONAL_PACKAGES=("docker" "docker-compose" "postgresql-client")

# Logging function
log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}✅ $1${NC}"
}

warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

error() {
    echo -e "${RED}❌ $1${NC}"
}

# Ensure gum is installed
gum_installed() {
  command -v gum &>/dev/null
}

install_gum() {
  if ! gum_installed; then
    log "Installing charmbracelet/gum for pretty terminal output..."
    case $PACKAGE_MANAGER in
      apt)
        sudo apt update && sudo apt install -y gum || \
        (log "gum not in apt, installing via Homebrew..." && \
         (command -v brew &>/dev/null || /bin/bash -c \"$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\") && \
         brew install gum)
        ;;
      brew)
        brew install gum
        ;;
      yum|dnf)
        sudo $PACKAGE_MANAGER install -y gum || log "Please install gum manually from https://github.com/charmbracelet/gum"
        ;;
      *)
        log "Unsupported package manager for gum. Please install manually."
        ;;
    esac
  else
    success "gum is already installed"
  fi
}

# Ensure ghorg is installed
ghorg_installed() {
  command -v ghorg &>/dev/null
}

install_ghorg() {
  if ! ghorg_installed; then
    log "Installing ghorg (bulk GitHub org/user repo manager)..."
    curl -Lo ghorg.tar.gz https://github.com/gabrie30/ghorg/releases/latest/download/ghorg_Linux_x86_64.tar.gz
    tar -xzf ghorg.tar.gz
    sudo mv ghorg /usr/local/bin/
    rm -f ghorg.tar.gz
    success "ghorg installed"
  else
    success "ghorg is already installed"
  fi
}

# Check if running as root
check_root() {
    if [[ $EUID -eq 0 ]]; then
        warning "Running as root. Consider running as a regular user for better security."
    fi
}

# Detect OS
detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if command -v apt-get &> /dev/null; then
            OS="ubuntu"
            PACKAGE_MANAGER="apt"
        elif command -v yum &> /dev/null; then
            OS="centos"
            PACKAGE_MANAGER="yum"
        elif command -v dnf &> /dev/null; then
            OS="fedora"
            PACKAGE_MANAGER="dnf"
        else
            OS="linux"
            PACKAGE_MANAGER="unknown"
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        OS="macos"
        PACKAGE_MANAGER="brew"
    else
        OS="unknown"
        PACKAGE_MANAGER="unknown"
    fi
    
    log "Detected OS: $OS with package manager: $PACKAGE_MANAGER"
}

# Check Python version
check_python() {
    log "Checking Python installation..."
    
    if ! command -v python3 &> /dev/null; then
        error "Python 3 is not installed"
        install_python
        return
    fi
    
    PYTHON_VERSION=$(python3 -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')")
    
    if [[ $(echo "$PYTHON_VERSION >= $REQUIRED_PYTHON_VERSION" | bc -l) -eq 1 ]]; then
        success "Python $PYTHON_VERSION is installed"
    else
        error "Python $PYTHON_VERSION is installed, but $REQUIRED_PYTHON_VERSION+ is required"
        install_python
    fi
}

# Install Python
install_python() {
    log "Installing Python $REQUIRED_PYTHON_VERSION..."
    
    case $OS in
        "ubuntu")
            sudo apt-get update
            sudo apt-get install -y python3 python3-pip python3-venv
            ;;
        "centos"|"fedora")
            sudo $PACKAGE_MANAGER install -y python3 python3-pip
            ;;
        "macos")
            if ! command -v brew &> /dev/null; then
                error "Homebrew is required for macOS installation"
                exit 1
            fi
            brew install python@3.11
            ;;
        *)
            error "Unsupported OS for automatic Python installation"
            exit 1
            ;;
    esac
}

# Check and install system packages
check_system_packages() {
    log "Checking system packages..."
    
    for package in "${REQUIRED_PACKAGES[@]}"; do
        if command -v "$package" &> /dev/null; then
            success "$package is installed"
        else
            warning "$package is not installed"
            install_system_package "$package"
        fi
    done
    
    for package in "${OPTIONAL_PACKAGES[@]}"; do
        if command -v "$package" &> /dev/null; then
            success "$package is installed (optional)"
        else
            warning "$package is not installed (optional)"
        fi
    done
}

# Install system package
install_system_package() {
    local package=$1
    
    case $OS in
        "ubuntu")
            case $package in
                "python3")
                    sudo apt-get install -y python3
                    ;;
                "pip3")
                    sudo apt-get install -y python3-pip
                    ;;
                "git")
                    sudo apt-get install -y git
                    ;;
                "curl")
                    sudo apt-get install -y curl
                    ;;
                "wget")
                    sudo apt-get install -y wget
                    ;;
                "docker")
                    curl -fsSL https://get.docker.com -o get-docker.sh
                    sudo sh get-docker.sh
                    sudo usermod -aG docker $USER
                    ;;
                "docker-compose")
                    sudo curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
                    sudo chmod +x /usr/local/bin/docker-compose
                    ;;
                "postgresql-client")
                    sudo apt-get install -y postgresql-client
                    ;;
            esac
            ;;
        "centos"|"fedora")
            case $package in
                "python3")
                    sudo $PACKAGE_MANAGER install -y python3
                    ;;
                "pip3")
                    sudo $PACKAGE_MANAGER install -y python3-pip
                    ;;
                "git")
                    sudo $PACKAGE_MANAGER install -y git
                    ;;
                "curl")
                    sudo $PACKAGE_MANAGER install -y curl
                    ;;
                "wget")
                    sudo $PACKAGE_MANAGER install -y wget
                    ;;
                "docker")
                    sudo $PACKAGE_MANAGER install -y docker
                    sudo systemctl start docker
                    sudo systemctl enable docker
                    sudo usermod -aG docker $USER
                    ;;
                "postgresql-client")
                    sudo $PACKAGE_MANAGER install -y postgresql
                    ;;
            esac
            ;;
        "macos")
            if ! command -v brew &> /dev/null; then
                error "Homebrew is required for macOS installation"
                exit 1
            fi
            brew install "$package"
            ;;
    esac
}

# Install Chrome/Chromium
install_chrome() {
    log "Checking Chrome/Chromium installation..."
    
    if command -v google-chrome &> /dev/null || command -v chromium-browser &> /dev/null; then
        success "Chrome/Chromium is installed"
        return
    fi
    
    warning "Chrome/Chromium not found. Installing..."
    
    case $OS in
        "ubuntu")
            wget -q -O - https://dl.google.com/linux/linux_signing_key.pub | sudo apt-key add -
            echo "deb [arch=amd64] http://dl.google.com/linux/chrome/deb/ stable main" | sudo tee /etc/apt/sources.list.d/google-chrome.list
            sudo apt-get update
            sudo apt-get install -y google-chrome-stable
            ;;
        "centos"|"fedora")
            sudo $PACKAGE_MANAGER install -y chromium
            ;;
        "macos")
            brew install --cask google-chrome
            ;;
    esac
}

# Install ChromeDriver
install_chromedriver() {
    log "Checking ChromeDriver installation..."
    
    if command -v chromedriver &> /dev/null; then
        success "ChromeDriver is installed"
        return
    fi
    
    warning "ChromeDriver not found. Installing..."
    
    # Get Chrome version
    CHROME_VERSION=$(google-chrome --version 2>/dev/null | grep -oE "[0-9]+\.[0-9]+\.[0-9]+" | head -1)
    if [[ -z "$CHROME_VERSION" ]]; then
        CHROME_VERSION="120.0.6099.109"  # Default version
    fi
    
    log "Detected Chrome version: $CHROME_VERSION"
    
    # Extract major version
    CHROME_MAJOR=$(echo "$CHROME_VERSION" | cut -d. -f1)
    log "Chrome major version: $CHROME_MAJOR"
    
    # Try to get the latest ChromeDriver version for this Chrome version
    CHROMEDRIVER_LATEST_URL="https://chromedriver.storage.googleapis.com/LATEST_RELEASE_$CHROME_MAJOR"
    log "Fetching latest ChromeDriver version from: $CHROMEDRIVER_LATEST_URL"
    
    # Get latest version with error handling
    CHROMEDRIVER_LATEST=$(curl -s "$CHROMEDRIVER_LATEST_URL" 2>/dev/null)
    
    if [[ -z "$CHROMEDRIVER_LATEST" ]]; then
        warning "Could not fetch latest ChromeDriver version, using fallback"
        # Use a known working version
        CHROMEDRIVER_LATEST="120.0.6099.109"
    fi
    
    log "Using ChromeDriver version: $CHROMEDRIVER_LATEST"
    
    # Download and install with error handling
    CHROMEDRIVER_DOWNLOAD_URL="https://chromedriver.storage.googleapis.com/$CHROMEDRIVER_LATEST/chromedriver_linux64.zip"
    log "Downloading from: $CHROMEDRIVER_DOWNLOAD_URL"
    
    if wget -O /tmp/chromedriver.zip "$CHROMEDRIVER_DOWNLOAD_URL" 2>/dev/null; then
        if unzip -q /tmp/chromedriver.zip -d /tmp/ 2>/dev/null; then
            if [[ -f "/tmp/chromedriver" ]]; then
                sudo mv /tmp/chromedriver /usr/local/bin/
                sudo chmod +x /usr/local/bin/chromedriver
                rm -f /tmp/chromedriver.zip
                success "ChromeDriver installed successfully"
            else
                error "ChromeDriver binary not found in downloaded archive"
                return 1
            fi
        else
            error "Failed to extract ChromeDriver archive"
            return 1
        fi
    else
        warning "Failed to download ChromeDriver from Google Storage, trying alternative method..."
        
        # Alternative: Use Chrome for Testing (more reliable)
        CHROME_FOR_TESTING_URL="https://edgedl.me.gvt1.com/edgedl/chrome/chrome-for-testing/120.0.6099.109/linux64/chromedriver-linux64.zip"
        log "Trying Chrome for Testing: $CHROME_FOR_TESTING_URL"
        
        if wget -O /tmp/chromedriver.zip "$CHROME_FOR_TESTING_URL" 2>/dev/null; then
            if unzip -q /tmp/chromedriver.zip -d /tmp/ 2>/dev/null; then
                if [[ -f "/tmp/chromedriver-linux64/chromedriver" ]]; then
                    sudo mv /tmp/chromedriver-linux64/chromedriver /usr/local/bin/
                    sudo chmod +x /usr/local/bin/chromedriver
                    rm -rf /tmp/chromedriver.zip /tmp/chromedriver-linux64
                    success "ChromeDriver installed successfully (Chrome for Testing)"
                else
                    error "ChromeDriver binary not found in Chrome for Testing archive"
                    return 1
                fi
            else
                error "Failed to extract Chrome for Testing archive"
                return 1
            fi
        else
            error "Failed to download ChromeDriver from both sources"
            return 1
        fi
    fi
    
    # Verify installation
    if command -v chromedriver &> /dev/null; then
        CHROMEDRIVER_VERSION=$(chromedriver --version 2>/dev/null | head -1)
        success "ChromeDriver verified: $CHROMEDRIVER_VERSION"
    else
        error "ChromeDriver installation verification failed"
        return 1
    fi
}

# Create virtual environment
setup_venv() {
    log "Setting up Python virtual environment..."
    
    if [[ -d "venv" ]]; then
        warning "Virtual environment already exists. Removing..."
        rm -rf venv
    fi
    
    python3 -m venv venv
    source venv/bin/activate
    
    # Upgrade pip
    pip install --upgrade pip
    
    success "Virtual environment created"
}

# Install Python dependencies
install_python_deps() {
    log "Installing Python dependencies..."
    
    source venv/bin/activate
    
    # Install requirements
    pip install -r requirements.txt
    
    success "Python dependencies installed"
}

# Setup environment variables
setup_env() {
    log "Setting up environment variables..."
    
    if [[ ! -f ".env" ]]; then
        cat > .env << EOF
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
EOF
        success "Environment file created: .env"
    else
        warning "Environment file already exists: .env"
    fi
}

# Create directories
create_directories() {
    log "Creating necessary directories..."
    
    mkdir -p downloads/{images,videos,chats}
    mkdir -p logs
    mkdir -p backups
    mkdir -p web/templates
    
    success "Directories created"
}

# Setup database
setup_database() {
    log "Setting up database..."
    
    # Check if PostgreSQL is running
    if ! pg_isready -h localhost -p 5432 &> /dev/null; then
        warning "PostgreSQL is not running. You'll need to start it manually."
        warning "For Docker setup, run: docker-compose up -d db"
    else
        success "PostgreSQL is running"
    fi
}

# Run tests
run_tests() {
    log "Running tests..."
    
    source venv/bin/activate
    
    # Run basic tests
    python -c "
import sys
import os
sys.path.append(os.path.dirname(os.path.abspath('.')))

try:
    from config import Config
    print('✅ Config module imported successfully')
except Exception as e:
    print(f'❌ Config module failed: {e}')
    sys.exit(1)

try:
    config = Config.from_env()
    print('✅ Configuration loaded successfully')
except Exception as e:
    print(f'❌ Configuration failed: {e}')
    sys.exit(1)

try:
    from db.database import DatabaseManager
    print('✅ Database module imported successfully')
except Exception as e:
    print(f'❌ Database module failed: {e}')
    sys.exit(1)

print('✅ All basic tests passed')
"
    
    success "Tests completed"
}

# Create startup script
create_startup_script() {
    log "Creating startup script..."
    
    cat > start.sh << 'EOF'
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
EOF
    
    chmod +x start.sh
    success "Startup script created: start.sh"
}

# Create web startup script
create_web_script() {
    log "Creating web startup script..."
    
    cat > start_web.sh << 'EOF'
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
EOF
    
    chmod +x start_web.sh
    success "Web startup script created: start_web.sh"
}

# Main installation function
main() {
    echo -e "${BLUE}"
    echo "🎨 OAI CLI Installation Script"
    echo "================================"
    echo -e "${NC}"
    
    # Change to script directory
    cd "$SCRIPT_DIR"
    
    # Run installation steps
    check_root
    detect_os
    check_python
    check_system_packages
    install_chrome
    install_chromedriver
    setup_venv
    install_python_deps
    setup_env
    create_directories
    setup_database
    run_tests
    create_startup_script
    create_web_script
    
    echo -e "${GREEN}"
    echo "🎉 Installation completed successfully!"
    echo ""
    echo "Next steps:"
    echo "1. Edit .env file with your database credentials"
    echo "2. Start the database: docker-compose up -d db"
    echo "3. Run the CLI: ./start.sh archive images"
    echo "4. Start web interface: ./start_web.sh"
    echo "5. Access dashboard: http://localhost:8080"
    echo ""
    echo "For more information, see README.md"
    echo -e "${NC}"
}

# Run main function
main "$@" 