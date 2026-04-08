//go:build windows

package hooks

import "os/exec"

func setCommandProcessGroup(cmd *exec.Cmd) {}

func killCommandProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
