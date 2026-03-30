// Package autorandr wraps the autorandr command for automatic display profile management.
package autorandr

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// execCommand is the function used to create exec.Cmd instances.
// Tests replace this to mock command execution.
var execCommand = exec.Command

// execLookPath is the function used to look up executables.
// Tests replace this to mock availability checks.
var execLookPath = exec.LookPath

// Profile represents a saved autorandr display profile.
type Profile struct {
	Name     string
	Current  bool
	Detected bool
}

// Client wraps the autorandr command-line tool.
type Client struct {
	// bin is the path to the autorandr binary. Defaults to "autorandr".
	bin string
}

// NewClient returns a new autorandr Client.
func NewClient() *Client {
	return &Client{bin: "autorandr"}
}

// IsAvailable returns true if autorandr is installed and on PATH.
func (c *Client) IsAvailable() bool {
	_, err := execLookPath(c.bin)
	return err == nil
}

// ListProfiles returns all saved autorandr profiles.
// Output format: one profile per line, with optional "(current)" and "(detected)" suffixes.
func (c *Client) ListProfiles() ([]Profile, error) {
	out, err := execCommand(c.bin).Output()
	if err != nil {
		return nil, fmt.Errorf("autorandr list: %w", err)
	}
	return parseProfiles(out), nil
}

// parseProfiles parses the output of `autorandr` (no args) into Profile structs.
func parseProfiles(data []byte) []Profile {
	var profiles []Profile
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		p := Profile{}
		if strings.Contains(line, "(current)") {
			p.Current = true
		}
		if strings.Contains(line, "(detected)") {
			p.Detected = true
		}
		// Name is the first field before any parenthetical.
		name := strings.Fields(line)[0]
		p.Name = name
		profiles = append(profiles, p)
	}
	return profiles
}

// CurrentProfile returns the name of the currently active profile.
// Returns empty string if no profile is active.
func (c *Client) CurrentProfile() (string, error) {
	profiles, err := c.ListProfiles()
	if err != nil {
		return "", err
	}
	for _, p := range profiles {
		if p.Current {
			return p.Name, nil
		}
	}
	return "", nil
}

// SaveProfile saves the current display configuration as a named profile.
func (c *Client) SaveProfile(name string) error {
	if name == "" {
		return fmt.Errorf("autorandr save: profile name must not be empty")
	}
	if err := execCommand(c.bin, "--save", name).Run(); err != nil {
		return fmt.Errorf("autorandr save %q: %w", name, err)
	}
	return nil
}

// LoadProfile applies a saved profile by name.
func (c *Client) LoadProfile(name string) error {
	if name == "" {
		return fmt.Errorf("autorandr load: profile name must not be empty")
	}
	if err := execCommand(c.bin, "--load", name).Run(); err != nil {
		return fmt.Errorf("autorandr load %q: %w", name, err)
	}
	return nil
}

// DeleteProfile removes a saved profile by name.
func (c *Client) DeleteProfile(name string) error {
	if name == "" {
		return fmt.Errorf("autorandr delete: profile name must not be empty")
	}
	if err := execCommand(c.bin, "--remove", name).Run(); err != nil {
		return fmt.Errorf("autorandr remove %q: %w", name, err)
	}
	return nil
}

// DetectAndApply auto-detects the best matching profile and applies it.
// Returns the name of the applied profile, or empty string if none matched.
func (c *Client) DetectAndApply() (string, error) {
	out, err := execCommand(c.bin, "--change").Output()
	if err != nil {
		return "", fmt.Errorf("autorandr --change: %w", err)
	}
	result := strings.TrimSpace(string(out))
	return result, nil
}

// WatchChanges polls for display changes and calls fn with the applied profile name
// whenever autorandr detects and applies a new configuration. It checks every 5 seconds.
// The function blocks until ctx is cancelled.
func (c *Client) WatchChanges(ctx context.Context, fn func(profile string)) error {
	if fn == nil {
		return fmt.Errorf("autorandr watch: callback must not be nil")
	}

	var lastProfile string
	// Get initial profile.
	if cur, err := c.CurrentProfile(); err == nil {
		lastProfile = cur
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			applied, err := c.DetectAndApply()
			if err != nil {
				continue
			}
			// Determine current profile after detect.
			cur, err := c.CurrentProfile()
			if err != nil {
				continue
			}
			if cur != "" && cur != lastProfile {
				lastProfile = cur
				name := cur
				if applied != "" {
					name = applied
				}
				fn(name)
			}
		}
	}
}
