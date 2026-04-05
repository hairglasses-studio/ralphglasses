package session

// DefaultPrimaryProvider is the provider used when callers omit a provider.
// Ralphglasses now treats Codex as the primary command-and-control runtime.
func DefaultPrimaryProvider() Provider {
	return ProviderCodex
}
