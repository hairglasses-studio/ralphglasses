#!/bin/bash

# Development Setup Script for OAI CLI
# This script sets up a development environment for contributors

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
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

# Check if we're in the right directory
check_directory() {
    if [[ ! -f "oai_cli.py" ]]; then
        error "Please run this script from the oai_cli directory"
        exit 1
    fi
}

# Install development dependencies
install_dev_deps() {
    log "Installing development dependencies..."
    
    pip install pytest pytest-cov pytest-mock
    pip install black flake8 mypy
    pip install pre-commit
    pip install bandit safety
    
    success "Development dependencies installed"
}

# Setup pre-commit hooks
setup_pre_commit() {
    log "Setting up pre-commit hooks..."
    
    if command -v pre-commit &> /dev/null; then
        pre-commit install
        success "Pre-commit hooks installed"
    else
        warning "pre-commit not found, skipping hooks setup"
    fi
}

# Create development configuration
setup_dev_config() {
    log "Setting up development configuration..."
    
    # Create .env for development
    if [[ ! -f ".env" ]]; then
        cat > .env << EOF
# Development Configuration
PG_CONN_STR=postgresql://test:test@localhost:5432/test
DOWNLOAD_DIR=./test_downloads
IMAGES_DIR=./test_downloads/images
VIDEOS_DIR=./test_downloads/videos
CHATS_DIR=./test_downloads/chats
WEB_HOST=127.0.0.1
WEB_PORT=8080
WEB_DEBUG=true
CHROME_HEADLESS=true
MAX_RETRIES=3
BATCH_SIZE=10
LOG_LEVEL=DEBUG
LOG_FILE=./logs/dev.log
EOF
        success "Development .env created"
    else
        warning ".env already exists, skipping"
    fi
}

# Create test directories
create_test_dirs() {
    log "Creating test directories..."
    
    mkdir -p test_downloads/{images,videos,chats}
    mkdir -p logs
    mkdir -p backups
    
    success "Test directories created"
}

# Setup test database
setup_test_db() {
    log "Setting up test database..."
    
    # Check if PostgreSQL is running
    if ! pg_isready -h localhost -p 5432 &> /dev/null; then
        warning "PostgreSQL is not running. Please start it manually:"
        echo "  sudo systemctl start postgresql"
        echo "  or"
        echo "  docker-compose up -d db"
        return
    fi
    
    # Create test database
    createdb test 2>/dev/null || warning "Database 'test' already exists or creation failed"
    
    # Run schema
    psql -d test -f sora_archive_schema.sql 2>/dev/null || warning "Schema setup failed"
    
    success "Test database setup completed"
}

# Run initial tests
run_tests() {
    log "Running initial tests..."
    
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
    
    success "Initial tests completed"
}

# Setup git hooks
setup_git_hooks() {
    log "Setting up git hooks..."
    
    # Create .git/hooks directory if it doesn't exist
    mkdir -p .git/hooks
    
    # Create pre-commit hook
    cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash

# Pre-commit hook for OAI CLI

echo "Running pre-commit checks..."

# Run black formatting check
if command -v black &> /dev/null; then
    black --check . || {
        echo "❌ Code formatting check failed. Run 'black .' to fix."
        exit 1
    }
    echo "✅ Code formatting check passed"
fi

# Run flake8 linting
if command -v flake8 &> /dev/null; then
    flake8 . || {
        echo "❌ Linting check failed. Fix the issues above."
        exit 1
    }
    echo "✅ Linting check passed"
fi

# Run mypy type checking
if command -v mypy &> /dev/null; then
    mypy . --ignore-missing-imports || {
        echo "❌ Type checking failed. Fix the issues above."
        exit 1
    }
    echo "✅ Type checking passed"
fi

echo "✅ All pre-commit checks passed"
EOF
    
    chmod +x .git/hooks/pre-commit
    success "Git hooks installed"
}

# Create development scripts
create_dev_scripts() {
    log "Creating development scripts..."
    
    # Test runner script
    cat > run_tests.sh << 'EOF'
#!/bin/bash

# Test runner script for OAI CLI

set -e

echo "🧪 Running OAI CLI tests..."

# Run all tests
pytest tests/ -v --cov=. --cov-report=html --cov-report=term-missing

echo "✅ Tests completed"
echo "📊 Coverage report: htmlcov/index.html"
EOF
    
    chmod +x run_tests.sh
    
    # Code formatting script
    cat > format_code.sh << 'EOF'
#!/bin/bash

# Code formatting script for OAI CLI

set -e

echo "🎨 Formatting code..."

# Run black
black .

# Run isort (if available)
if command -v isort &> /dev/null; then
    isort .
fi

echo "✅ Code formatting completed"
EOF
    
    chmod +x format_code.sh
    
    # Linting script
    cat > lint_code.sh << 'EOF'
#!/bin/bash

# Linting script for OAI CLI

set -e

echo "🔍 Running linting checks..."

# Run flake8
flake8 . --count --select=E9,F63,F7,F82 --show-source --statistics

# Run mypy
mypy . --ignore-missing-imports

# Run bandit security check
if command -v bandit &> /dev/null; then
    bandit -r . -f json -o bandit-report.json || true
fi

echo "✅ Linting completed"
EOF
    
    chmod +x lint_code.sh
    
    success "Development scripts created"
}

# Create development documentation
create_dev_docs() {
    log "Creating development documentation..."
    
    mkdir -p docs
    
    # Development guide
    cat > docs/DEVELOPMENT.md << 'EOF'
# Development Guide

## Setup

1. Run the development setup script:
   ```bash
   ./scripts/setup_dev.sh
   ```

2. Activate virtual environment:
   ```bash
   source venv/bin/activate
   ```

3. Start test database:
   ```bash
   docker-compose up -d db
   ```

## Development Workflow

1. **Make changes** to the code
2. **Run tests**: `./run_tests.sh`
3. **Format code**: `./format_code.sh`
4. **Lint code**: `./lint_code.sh`
5. **Commit changes**: `git commit -m "Your message"`

## Testing

- **Unit tests**: `pytest tests/ -m unit`
- **Integration tests**: `pytest tests/ -m integration`
- **All tests**: `pytest tests/ -v`
- **With coverage**: `pytest tests/ --cov=. --cov-report=html`

## Code Style

- **Formatting**: Black
- **Linting**: Flake8
- **Type checking**: MyPy
- **Security**: Bandit

## Database

- **Test database**: `test`
- **Schema**: `sora_archive_schema.sql`
- **Connection**: `postgresql://test:test@localhost:5432/test`

## Logging

- **Development logs**: `logs/dev.log`
- **Structured logs**: `logs/dev_structured.json`
- **Error logs**: `logs/dev_errors.log`

## API Testing

```bash
# Start web interface
python web/app.py

# Test endpoints
curl http://localhost:8080/health
curl http://localhost:8080/api/stats
```
EOF
    
    # Contributing guide
    cat > CONTRIBUTING.md << 'EOF'
# Contributing to OAI CLI

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/your-username/oai_cli.git`
3. Run development setup: `./scripts/setup_dev.sh`
4. Create a feature branch: `git checkout -b feature/amazing-feature`

## Development Guidelines

### Code Style
- Follow PEP 8 with Black formatting
- Use type hints for all functions
- Write docstrings for all public functions
- Keep functions small and focused

### Testing
- Write tests for all new functionality
- Maintain test coverage above 80%
- Use descriptive test names
- Test both success and failure cases

### Documentation
- Update README.md for user-facing changes
- Add docstrings for new functions
- Update API documentation if needed
- Include examples in docstrings

### Git Workflow
1. Make your changes
2. Run tests: `./run_tests.sh`
3. Format code: `./format_code.sh`
4. Lint code: `./lint_code.sh`
5. Commit with descriptive message
6. Push to your fork
7. Open a Pull Request

## Pull Request Guidelines

- **Title**: Clear, descriptive title
- **Description**: Explain what and why, not how
- **Tests**: Include tests for new functionality
- **Documentation**: Update docs if needed
- **Screenshots**: Include for UI changes

## Code Review Process

1. **Automated checks** must pass
2. **Code review** by maintainers
3. **Address feedback** and update PR
4. **Merge** when approved

## Release Process

1. **Version bump** in appropriate files
2. **Changelog** update
3. **Tag release** on GitHub
4. **Docker image** build and push
5. **Documentation** update

## Questions?

- Open an issue for bugs
- Start a discussion for questions
- Join our community chat
EOF
    
    success "Development documentation created"
}

# Main function
main() {
    echo -e "${BLUE}"
    echo "🎨 OAI CLI Development Setup"
    echo "============================="
    echo -e "${NC}"
    
    # Check directory
    check_directory
    
    # Run setup steps
    install_dev_deps
    setup_pre_commit
    setup_dev_config
    create_test_dirs
    setup_test_db
    run_tests
    setup_git_hooks
    create_dev_scripts
    create_dev_docs
    
    echo -e "${GREEN}"
    echo "🎉 Development environment setup completed!"
    echo ""
    echo "Next steps:"
    echo "1. Activate virtual environment: source venv/bin/activate"
    echo "2. Start test database: docker-compose up -d db"
    echo "3. Run tests: ./run_tests.sh"
    echo "4. Format code: ./format_code.sh"
    echo "5. Start development: python oai_cli.py --help"
    echo ""
    echo "Documentation:"
    echo "- Development guide: docs/DEVELOPMENT.md"
    echo "- Contributing guide: CONTRIBUTING.md"
    echo ""
    echo "Happy coding! 🚀"
    echo -e "${NC}"
}

# Run main function
main "$@" 