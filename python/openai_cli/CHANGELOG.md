# Changelog

All notable changes to the OAI CLI project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.1.0] - 2024-01-16

### 🎬 Sora Video Downloader Implementation

#### ✨ Added
- **Complete Sora Video Scraper** (`sora_downloader.py`)
  - Browser automation for OpenAI Sora interface
  - Video metadata extraction and downloading
  - Prompt text association with videos
  - Video file metadata extraction using OpenCV
  - Retry logic and error handling
  - Database integration for video metadata
  - Manual login workflow for Sora access

- **Enhanced Video Processing**
  - Video duration and resolution detection
  - FPS and frame count extraction
  - Automatic filename generation
  - Duplicate detection and conflict resolution
  - Batch processing with configurable delays

- **Comprehensive Testing**
  - Unit tests for Sora downloader functionality
  - Mock testing for browser automation
  - Video metadata extraction testing
  - Error handling and edge case testing

#### 🔧 Improved
- **Video Archiving Workflow**
  - Seamless integration with existing CLI
  - Consistent error handling and logging
  - Progress reporting and status updates
  - Resource cleanup and memory management

#### 📚 Documentation
- **Updated README** with Sora video archiving details
- **Enhanced API documentation** for video endpoints
- **Usage examples** for Sora video archiving

## [2.0.0] - 2024-01-15

### 🎉 Major Release - Complete Repository Overhaul

#### ✨ Added
- **Comprehensive Installation Script** (`install.sh`)
  - Automated dependency detection and installation
  - Chrome/Chromium and ChromeDriver setup
  - Virtual environment creation and package installation
  - Configuration file generation
  - Startup script creation
  - Basic system validation

- **Advanced Monitoring System** (`monitoring/health_check.py`)
  - Database connection health checks
  - Disk space monitoring
  - Memory usage tracking
  - Web interface availability checks
  - Chrome/Chromium availability verification
  - Download directory permissions validation
  - Comprehensive health reporting with JSON and text formats

- **Structured Logging System** (`logging_config.py`)
  - Colored console output with different log levels
  - Rotating file logs with size limits
  - Structured JSON logging for machine processing
  - Separate error log files
  - Archive session logging
  - Database operation logging
  - Web request logging
  - System metrics logging
  - Security event logging
  - Automatic log cleanup

- **Comprehensive Test Suite** (`tests/`)
  - Unit tests for configuration management
  - Database operation tests with mocking
  - Integration test framework
  - Test categorization (unit, integration, database, web)
  - Coverage reporting with HTML output
  - Pytest configuration with custom markers

- **CI/CD Pipeline** (`.github/workflows/ci.yml`)
  - Multi-Python version testing (3.11, 3.12)
  - Automated dependency installation
  - Code linting with flake8, black, mypy
  - Security scanning with CodeQL and Bandit
  - Docker image building and pushing
  - Integration testing with PostgreSQL
  - Coverage reporting to Codecov
  - Automated deployment pipeline

- **Development Tools** (`scripts/setup_dev.sh`)
  - Development environment setup script
  - Pre-commit hooks for code quality
  - Git hooks for automated checks
  - Development configuration management
  - Test database setup
  - Code formatting and linting scripts
  - Development documentation generation

- **Enhanced Documentation**
  - Complete README rewrite with comprehensive sections
  - Installation guides for different scenarios
  - Usage examples and command reference
  - API documentation with endpoint details
  - Troubleshooting guide with common issues
  - Development setup instructions
  - Contributing guidelines
  - Architecture overview

#### 🔧 Improved
- **Database Layer** (`db/database.py`)
  - Fixed type annotations and null handling
  - Improved connection pooling with proper error handling
  - Enhanced query execution with better error recovery
  - Comprehensive metadata insertion with conflict resolution
  - Full-text search capabilities across all content types
  - Archive statistics with database functions
  - Better transaction management

- **Configuration System** (`config.py`)
  - Enhanced environment variable handling
  - Comprehensive validation with detailed error messages
  - Support for all system components
  - Flexible configuration for different environments
  - Type-safe configuration with dataclasses

- **Web Interface** (`web/app.py`)
  - Fixed null handling issues
  - Enhanced error handling and logging
  - Improved API endpoint structure
  - Better health check implementation
  - Comprehensive error responses

- **CLI Application** (`oai_cli.py`)
  - Better error handling and user feedback
  - Improved command structure and help text
  - Enhanced progress reporting
  - Comprehensive cleanup procedures
  - Better integration with monitoring systems

#### 🐛 Fixed
- **Type Safety Issues**
  - Fixed all linter errors in database operations
  - Resolved null handling in web interface
  - Improved type annotations throughout codebase
  - Fixed optional parameter handling

- **Import Resolution**
  - Added missing dependencies to requirements.txt
  - Fixed import paths and module resolution
  - Enhanced error handling for missing modules

- **Configuration Validation**
  - Improved environment variable validation
  - Better error messages for missing configuration
  - Enhanced database connection validation

#### 📚 Documentation
- **Complete README Overhaul**
  - Comprehensive feature descriptions
  - Step-by-step installation guides
  - Usage examples for all features
  - API reference with endpoint details
  - Troubleshooting section with common issues
  - Development setup instructions
  - Contributing guidelines

- **Development Documentation**
  - Development setup guide
  - Contributing guidelines
  - Code style standards
  - Testing procedures
  - Release process documentation

#### 🚀 Deployment
- **Docker Improvements**
  - Enhanced multi-stage Dockerfile
  - Better health checks and monitoring
  - Improved security with non-root user
  - Optimized image size and build process

- **CI/CD Pipeline**
  - Comprehensive testing across Python versions
  - Security scanning and code quality checks
  - Automated Docker image building
  - Integration testing with real database
  - Coverage reporting and monitoring

#### 🔒 Security
- **Enhanced Security Features**
  - Non-root user execution in containers
  - Input validation and sanitization
  - Secure defaults for all configurations
  - Comprehensive logging for security events
  - Automated security scanning in CI/CD

#### 📊 Monitoring
- **Comprehensive Monitoring**
  - System health checks for all components
  - Performance metrics and resource monitoring
  - Automated alerting for critical issues
  - Structured logging for analysis
  - Health check endpoints for external monitoring

## [1.0.0] - 2024-01-01

### 🎉 Initial Release

#### ✨ Added
- **Core Archiving Features**
  - Image scraping and downloading from OpenAI
  - Video archiving with metadata tracking
  - Chat conversation export with full message history
  - JSON export capabilities for conversations

- **Database Integration**
  - PostgreSQL storage with connection pooling
  - Comprehensive metadata tracking
  - Full-text search capabilities
  - Archive session management

- **Web Dashboard**
  - Real-time monitoring interface
  - Search and discovery features
  - Responsive design with modern UI
  - API endpoints for external integration

- **Docker Support**
  - Multi-stage Docker builds
  - Docker Compose for easy deployment
  - Health checks and monitoring
  - Production-ready containerization

- **Basic CLI**
  - Command-line interface for archiving
  - Statistics and reporting features
  - Configuration management
  - Error handling and logging

---

## Version History

- **v2.0.0**: Complete repository overhaul with comprehensive monitoring, testing, and documentation
- **v1.0.0**: Initial release with basic archiving capabilities

## Migration Guide

### Upgrading from v1.0.0 to v2.0.0

1. **Backup your data**
   ```bash
   pg_dump oai_archive > backup_v1.sql
   cp -r downloads/ downloads_backup/
   ```

2. **Update the codebase**
   ```bash
   git pull origin main
   ```

3. **Run the new installation script**
   ```bash
   chmod +x install.sh
   ./install.sh
   ```

4. **Update your configuration**
   - Review the new `.env` file
   - Update any custom configurations
   - Test the new monitoring features

5. **Verify the upgrade**
   ```bash
   python monitoring/health_check.py
   python oai_cli.py stats
   ```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details on how to contribute to this project.

## Support

- **Issues**: [GitHub Issues](https://github.com/hairglasses/oai_cli/issues)
- **Discussions**: [GitHub Discussions](https://github.com/hairglasses/oai_cli/discussions)
- **Documentation**: [Wiki](https://github.com/hairglasses/oai_cli/wiki) 