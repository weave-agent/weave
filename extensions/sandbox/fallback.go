//go:build !darwin && !linux

package sandbox

import "fmt"

func wrapCommandDarwin(cmd, _ string) (string, error) {
	return "", fmt.Errorf("sandbox: unsupported platform")
}

func generateSeatbeltProfile(_ SandboxConfig, _ string) string {
	return ""
}

func wrapCommandDarwinWithConfig(cmd, _ string, _ SandboxConfig) (string, error) {
	return "", fmt.Errorf("sandbox: unsupported platform")
}

func wrapCommandLinux(cmd, _ string) (string, error) {
	return "", fmt.Errorf("sandbox: unsupported platform")
}

func wrapCommandLinuxWithConfig(cmd, _ string, _ SandboxConfig) (string, error) {
	return "", fmt.Errorf("sandbox: unsupported platform")
}
