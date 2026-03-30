package headless

import (
	"os"
	"syscall"
	"unsafe"
)

// IsHeadless returns true when stdout is not a terminal (piped, redirected,
// or running in a non-interactive context like cron/systemd).
func IsHeadless() bool {
	return !isTerminal(os.Stdout.Fd())
}

// isTerminal returns true if the file descriptor is a terminal.
// Uses the TIOCGETA ioctl on Darwin and TCGETS on Linux.
func isTerminal(fd uintptr) bool {
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		fd,
		ioctlReadTermios,
		uintptr(unsafe.Pointer(&termios)),
		0, 0, 0,
	)
	return err == 0
}

// IsTmuxSession returns true if the process is running inside a tmux session.
func IsTmuxSession() bool {
	return os.Getenv("TMUX") != ""
}

// IsSSH returns true if the process appears to be running over SSH.
func IsSSH() bool {
	return os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != ""
}

// IsWSL returns true if running inside Windows Subsystem for Linux.
func IsWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	for _, substr := range []string{"microsoft", "Microsoft", "WSL"} {
		if containsBytes(data, []byte(substr)) {
			return true
		}
	}
	return false
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
