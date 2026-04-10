package main

import (
	"context"

	"github.com/hairglasses-studio/ralphglasses/internal/observability"
)

func initObservability() func() {
	_, shutdown, err := observability.NewProvider("ralphglasses-prompt-improver", "")
	if err != nil {
		return func() {}
	}
	return func() {
		shutdown(context.Background())
	}
}
