package session

import (
	"context"
	"fmt"
	"strings"
)

type launchProviderSelection struct {
	Provider     Provider
	Model        string
	AutoSelected bool
	Reason       string
}

func resolveLaunchProviderSelection(ctx context.Context, opts LaunchOptions) launchProviderSelection {
	if provider := normalizeSessionProvider(opts.Provider); provider != "" {
		return launchProviderSelection{
			Provider: provider,
			Model:    firstNonBlankModel(opts.Model, ProviderDefaults(provider)),
			Reason:   fmt.Sprintf("explicit provider %q requested", provider),
		}
	}

	if inferredProvider, ok := InferProviderFromModel(opts.Model); ok {
		return launchProviderSelection{
			Provider:     inferredProvider,
			Model:        firstNonBlankModel(opts.Model, ProviderDefaults(inferredProvider)),
			AutoSelected: true,
			Reason:       fmt.Sprintf("inferred provider %q from model %q", inferredProvider, strings.TrimSpace(opts.Model)),
		}
	}

	defaultProvider := DefaultPrimaryProvider()
	return launchProviderSelection{
		Provider: defaultProvider,
		Model:    firstNonBlankModel(opts.Model, ProviderDefaults(defaultProvider)),
		Reason:   fmt.Sprintf("using default primary provider %q", defaultProvider),
	}
}

func applyLaunchProviderSelection(ctx context.Context, opts *LaunchOptions) launchProviderSelection {
	if opts == nil {
		return launchProviderSelection{}
	}
	selection := resolveLaunchProviderSelection(ctx, *opts)
	opts.Provider = selection.Provider
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = selection.Model
	}
	return selection
}

func firstNonBlankModel(current, fallback string) string {
	if strings.TrimSpace(current) != "" {
		return current
	}
	return strings.TrimSpace(fallback)
}
