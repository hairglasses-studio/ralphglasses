# Multi-stage build for better security and smaller image
FROM python:3.11-slim as builder

# Install build dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    g++ \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# Create virtual environment
RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

# Copy requirements first for better caching
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Production stage
FROM python:3.11-slim

# Create non-root user for security
RUN groupadd -r oaiuser && useradd -r -g oaiuser oaiuser

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    wget \
    unzip \
    gnupg \
    curl \
    ffmpeg \
    rsync \
    cron \
    chromium-driver \
    chromium \
    libpq5 \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Copy virtual environment from builder
COPY --from=builder /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

# Set up Chrome environment
ENV PATH="/usr/lib/chromium:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:$PATH"
ENV CHROMEDRIVER_PATH="/usr/bin/chromedriver"
ENV GOOGLE_CHROME_BIN="/usr/bin/chromium"

# Create app directory and set permissions
WORKDIR /app
RUN mkdir -p /app/downloads /app/logs /app/backups \
    && chown -R oaiuser:oaiuser /app

# Copy application code
COPY --chown=oaiuser:oaiuser . .

# Create necessary directories
RUN mkdir -p /app/downloads/images /app/downloads/videos /app/web/templates \
    && chown -R oaiuser:oaiuser /app

# Switch to non-root user
USER oaiuser

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python -c "import requests; requests.get('http://localhost:8080/health')" || exit 1

# Default command
CMD ["python", "oai_cli.py", "--help"]
