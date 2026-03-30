# Raspberry Pi Deployment

Run ralphglasses on a Raspberry Pi as a headless or minimal-display fleet controller.

## Supported Models

| Model | RAM | Status |
|-------|-----|--------|
| Pi 4B | 4 GB | Supported (single-session recommended) |
| Pi 4B | 8 GB | Supported |
| Pi 5 | 4 GB | Supported |
| Pi 5 | 8 GB | **Recommended** |

Models with less than 4 GB RAM are not supported.

## Prerequisites

- **OS**: Ubuntu Server 24.04 LTS (arm64)
- **Go**: 1.26 or later
- **RAM**: 4 GB minimum, 8 GB recommended
- **Storage**: 16 GB+ microSD (A2-rated) or USB SSD
- **Network**: Ethernet or Wi-Fi with stable internet (LLM API calls)

## Installation

### Option A: Cross-Compile from Dev Machine

Build on your development machine and copy the binary:

```bash
GOOS=linux GOARCH=arm64 go build -o ralphglasses-arm64 .
scp ralphglasses-arm64 pi@<pi-ip>:~/ralphglasses
```

This is the fastest approach -- builds take seconds on a modern dev machine.

### Option B: Build on Device

```bash
# Install Go (if not present)
wget https://go.dev/dl/go1.26.1.linux-arm64.tar.gz
sudo tar -C /usr/local -xzf go1.26.1.linux-arm64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Clone and build
git clone https://github.com/hairglasses-studio/ralphglasses.git
cd ralphglasses
go build -o ralphglasses .
```

On-device build times: ~5 min on Pi 5, ~12 min on Pi 4B.

## Configuration

### Reduced TUI Mode

For limited or headless displays, use the `--headless` flag or set reduced dimensions:

```bash
# Headless mode (MCP server only, no TUI)
./ralphglasses mcp

# Reduced TUI for small terminals
export RALPH_TUI_COMPACT=1
./ralphglasses --scan-path /path/to/repos
```

### Memory-Optimized Session Limits

Edit `~/.ralphrc` or pass flags to limit concurrent sessions:

```yaml
# ~/.ralphrc
max_sessions: 1        # Pi 4B (4 GB)
max_sessions: 3        # Pi 5 (8 GB)
provider: claude        # Single provider reduces memory
```

### GPIO Integration (Future)

GPIO pin assignments for status LEDs are planned but not yet implemented. Tracking in ROADMAP.md.

## Systemd Service

Create `/etc/systemd/system/ralphglasses.service`:

```ini
[Unit]
Description=Ralphglasses Fleet Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=ralph
WorkingDirectory=/home/ralph
ExecStart=/home/ralph/ralphglasses mcp
Restart=on-failure
RestartSec=5
Environment=GOMAXPROCS=4
Environment=RALPH_TUI_COMPACT=1

# Memory limits
MemoryMax=3G
MemoryHigh=2G

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ralphglasses
sudo journalctl -u ralphglasses -f
```

## Performance Tuning

### GOMAXPROCS

Set `GOMAXPROCS` to match your Pi's core count:

```bash
export GOMAXPROCS=4   # Pi 4B and Pi 5
```

The Go runtime defaults to all available cores, which is correct for the Pi. Setting it explicitly prevents issues if running inside cgroups or containers.

### Swap Configuration

Add swap for on-device builds (not needed for runtime):

```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

### Network Optimization

LLM API calls are latency-sensitive. Reduce overhead:

```bash
# Use Ethernet over Wi-Fi when possible
# Disable IPv6 if your network doesn't support it
sudo sysctl -w net.ipv6.conf.all.disable_ipv6=1

# Increase TCP keepalive for long-running sessions
sudo sysctl -w net.ipv4.tcp_keepalive_time=60
sudo sysctl -w net.ipv4.tcp_keepalive_intvl=10
```

### Storage

Use a USB 3.0 SSD instead of microSD for significantly better I/O. The Pi 5 supports NVMe via HAT.

## Known Limitations

- **No GPU acceleration** -- All LLM inference happens via API calls; the Pi acts as a controller only.
- **Single-session recommended on 4 GB models** -- Each Claude Code session consumes 500 MB-1 GB RSS. On 4 GB Pi 4B, limit to one concurrent session.
- **On-device build times** -- ~5 min on Pi 5, ~12 min on Pi 4B. Cross-compile instead.
- **TUI refresh rate** -- The compact TUI mode reduces update frequency to lower CPU usage.
- **No multi-monitor** -- Pi's display output is not used for the 7-monitor fleet setup. Use the Pi as a headless MCP server or single-terminal controller.

## Troubleshooting

### `signal: killed` During Build

The Go compiler ran out of memory. Add swap (see above) or cross-compile from your dev machine.

### High Memory Usage at Idle

Check session count. Reduce `max_sessions` in `~/.ralphrc`. The MCP server alone uses ~50 MB.

### Slow API Responses

- Verify network with `curl -o /dev/null -w '%{time_total}' https://api.anthropic.com/v1/messages`
- Switch to Ethernet if on Wi-Fi
- Check `journalctl -u ralphglasses` for timeout errors

### `permission denied` on GPIO (Future)

Add the user to the `gpio` group:

```bash
sudo usermod -aG gpio ralph
```

### Service Fails to Start

```bash
# Check logs
sudo journalctl -u ralphglasses --no-pager -n 50

# Verify binary runs manually
/home/ralph/ralphglasses --version

# Check API keys are set
sudo systemctl show ralphglasses -p Environment
```

Ensure API keys are available to the service. Use `Environment=` directives in the unit file or an `EnvironmentFile=`:

```ini
EnvironmentFile=/home/ralph/.ralph-env
```
