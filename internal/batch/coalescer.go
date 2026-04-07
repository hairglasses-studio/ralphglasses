package batch

import (
	"context"
	"fmt"
	"sync"
)

// Coalescer groups batch requests by provider and delegates them to provider-specific BatchManagers.
type Coalescer struct {
	mu       sync.RWMutex
	managers map[Provider]*BatchManager
	cfg      BatchManagerConfig
	factory  func(provider Provider) (Client, error)
}

// NewCoalescer creates a new Coalescer. The factory function is used to create
// provider-specific Clients on demand.
func NewCoalescer(cfg BatchManagerConfig, factory func(provider Provider) (Client, error)) *Coalescer {
	return &Coalescer{
		managers: make(map[Provider]*BatchManager),
		cfg:      cfg,
		factory:  factory,
	}
}

// Submit routes a request to the appropriate provider's BatchManager.
func (c *Coalescer) Submit(ctx context.Context, provider Provider, bmr BatchManagerRequest) (string, error) {
	manager, err := c.getOrCreateManager(provider)
	if err != nil {
		return "", fmt.Errorf("failed to get manager for provider %q: %w", provider, err)
	}

	return manager.Submit(ctx, bmr)
}

// Flush forces all underlying BatchManagers to flush their queues.
func (c *Coalescer) Flush(ctx context.Context) (map[Provider]*BatchManagerResult, error) {
	c.mu.RLock()
	// Snapshot the current managers to avoid holding the lock during flush
	snapshot := make(map[Provider]*BatchManager, len(c.managers))
	for p, m := range c.managers {
		snapshot[p] = m
	}
	c.mu.RUnlock()

	results := make(map[Provider]*BatchManagerResult)
	var firstErr error

	for p, m := range snapshot {
		res, err := m.Flush(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("flush failed for provider %q: %w", p, err)
			}
			continue
		}
		if res != nil {
			results[p] = res
		}
	}

	return results, firstErr
}

// getOrCreateManager returns the BatchManager for the given provider,
// creating it if it doesn't already exist.
func (c *Coalescer) getOrCreateManager(provider Provider) (*BatchManager, error) {
	c.mu.RLock()
	m, ok := c.managers[provider]
	c.mu.RUnlock()
	if ok {
		return m, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if m, ok := c.managers[provider]; ok {
		return m, nil
	}

	client, err := c.factory(provider)
	if err != nil {
		return nil, err
	}

	// Make a copy of the base config and set the provider
	cfg := c.cfg
	cfg.Provider = provider

	m = NewBatchManager(cfg, client)
	c.managers[provider] = m

	return m, nil
}
