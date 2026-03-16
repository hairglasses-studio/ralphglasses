# Ralphglasses Marathon Plan

## How to Start the 12-Hour Loop

### Prerequisites
1. Ensure `ralph-claude-code` is installed: `which ralph` or check `~/hairglasses-studio/ralph-claude-code/ralph_loop.sh`
2. Set your API key: `export ANTHROPIC_API_KEY=<key>`

### Launch Options

**Option A: Via ralph directly**
```bash
cd ~/hairglasses-studio/ralphglasses
ralph --calls 80 --timeout 20 --verbose --auto-reset-circuit
```

**Option B: Via ralph with marathon mode**
```bash
cd ~/hairglasses-studio/ralphglasses
ralph --calls 80 --timeout 20 --verbose --monitor
```
This starts with a tmux session for monitoring.

**Option C: Via ralphglasses TUI (self-hosting)**
```bash
cd ~/hairglasses-studio/ralphglasses
go run . --scan-path ~/hairglasses-studio
# Then press S on the ralphglasses row to start its own loop
```

**Option D: Via ralphglasses MCP (from another Claude session)**
```bash
claude mcp add ralphglasses -- go run ~/hairglasses-studio/ralphglasses/cmd/ralphglasses-mcp
# Then in Claude: use ralphglasses_start with repo "ralphglasses"
```

### Monitor Progress
- **TUI**: `go run . --scan-path ~/hairglasses-studio` — watch the ralphglasses row
- **Logs**: `tail -f .ralph/logs/ralph.log`
- **Status**: `cat .ralph/status.json | jq`
- **Circuit**: `cat .ralph/.circuit_breaker_state | jq`

### Budget Tracking
At 80 calls/hour with sonnet model:
- ~$0.10-0.15 per call average → ~$8-12/hour
- 12 hours → ~$96-144 estimated (right around $100 budget)
- If spend exceeds budget, circuit breaker should trip via budget_status

### Recovery
If the loop stops unexpectedly:
```bash
# Check why
cat .ralph/status.json | jq '.exit_reason'
cat .ralph/.circuit_breaker_state | jq '.state,.reason'

# Reset circuit breaker if needed
ralph --reset-circuit

# Restart
ralph --calls 80 --timeout 20 --verbose --auto-reset-circuit
```

### What Gets Built
See `.ralph/PROMPT.md` for the full task list. In priority order:
1. Tests for all packages
2. MCP server hardening
3. TUI polish
4. Process manager improvements
5. Config editor enhancements
6. Documentation
