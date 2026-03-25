package session

// Model cost rates per 1M tokens (USD). Single source of truth for all
// cost-aware subsystems (budget normalization, cascade routing, tier selection).
// Update here when provider pricing changes.
const (
	CostGeminiFlashLiteInput = 0.10
	CostGeminiFlashInput     = 0.30
	CostGeminiFlashOutput    = 2.50
	CostClaudeSonnetInput    = 3.00
	CostClaudeSonnetOutput   = 15.00
	CostClaudeOpusInput      = 15.00
	CostClaudeOpusOutput     = 75.00
	CostCodexInput           = 2.50
	CostCodexOutput          = 15.00
)
