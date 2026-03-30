package secrets

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestVaultProvider_AllMethodsReturnError(t *testing.T) {
	tests := []struct {
		name   string
		method string
		fn     func(v *VaultProvider) error
	}{
		{
			name:   "Get returns not-implemented error",
			method: "Get",
			fn: func(v *VaultProvider) error {
				_, err := v.Get("mykey")
				return err
			},
		},
		{
			name:   "Set returns not-implemented error",
			method: "Set",
			fn: func(v *VaultProvider) error {
				return v.Set("mykey", "myval")
			},
		},
		{
			name:   "Delete returns not-implemented error",
			method: "Delete",
			fn: func(v *VaultProvider) error {
				return v.Delete("mykey")
			},
		},
		{
			name:   "List returns not-implemented error",
			method: "List",
			fn: func(v *VaultProvider) error {
				_, err := v.List()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &VaultProvider{
				Address:    "https://vault.example.com:8200",
				Token:      "s.testtoken",
				MountPath:  "secret",
				SecretPath: "data/ralphglasses",
			}

			err := tt.fn(v)
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tt.method)
			}

			if !errors.Is(err, errNotImplemented) {
				t.Fatalf("%s: expected errNotImplemented, got %v", tt.method, err)
			}

			// Verify the error message includes the method name.
			if !strings.Contains(err.Error(), tt.method) {
				t.Fatalf("%s: error should mention method name, got: %s", tt.method, err.Error())
			}
		})
	}
}

func TestVaultProvider_GetIncludesKey(t *testing.T) {
	v := &VaultProvider{}
	_, err := v.Get("specific-key")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "specific-key") {
		t.Fatalf("error should include key name, got: %s", err.Error())
	}
}

func TestVaultProvider_SetIncludesKey(t *testing.T) {
	v := &VaultProvider{}
	err := v.Set("another-key", "val")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "another-key") {
		t.Fatalf("error should include key name, got: %s", err.Error())
	}
}

func TestVaultProvider_DeleteIncludesKey(t *testing.T) {
	v := &VaultProvider{}
	err := v.Delete("del-key")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "del-key") {
		t.Fatalf("error should include key name, got: %s", err.Error())
	}
}

func TestVaultProvider_ImplementsProvider(t *testing.T) {
	// Compile-time check is in secrets_test.go, but verify at runtime too.
	var p Provider = &VaultProvider{}
	_ = p
}

func TestVaultProvider_ErrorUnwrap(t *testing.T) {
	v := &VaultProvider{}

	_, err := v.Get("test")
	if err == nil {
		t.Fatal("expected error")
	}

	// errors.Is should find errNotImplemented in the chain.
	if !errors.Is(err, errNotImplemented) {
		t.Fatal("error chain should contain errNotImplemented")
	}

	// fmt.Errorf %w wrapping should produce an unwrappable error.
	unwrapped := errors.Unwrap(err)
	if unwrapped == nil {
		t.Fatal("expected unwrappable error")
	}

	// The unwrapped error should be errNotImplemented itself.
	if !errors.Is(unwrapped, errNotImplemented) {
		t.Fatalf("unwrapped error should be errNotImplemented, got %v", unwrapped)
	}
}

func TestVaultProvider_Fields(t *testing.T) {
	v := &VaultProvider{
		Address:    "https://vault:8200",
		Token:      "token123",
		MountPath:  "kv",
		SecretPath: "data/app",
	}

	if v.Address != "https://vault:8200" {
		t.Fatalf("Address mismatch")
	}
	if v.Token != "token123" {
		t.Fatalf("Token mismatch")
	}
	if v.MountPath != "kv" {
		t.Fatalf("MountPath mismatch")
	}
	if v.SecretPath != "data/app" {
		t.Fatalf("SecretPath mismatch")
	}
}

func TestVaultProvider_ErrorMessage(t *testing.T) {
	v := &VaultProvider{}
	expected := fmt.Sprintf("%s: Get(%q)", errNotImplemented.Error(), "x")
	_, err := v.Get("x")
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}
