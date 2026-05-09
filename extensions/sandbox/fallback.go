//go:build !darwin && !linux

package sandbox

import "fmt"

func generateSeatbeltProfile(_ SandboxConfig, _ string) string {
	return ""
}

func wrapCommandDarwinWithConfig(cmd, _ string, _ SandboxConfig) (string, error) {
	return "", fmt.Errorf("sandbox: unsupported platform")
}

func wrapCommandLinuxWithConfig(cmd, _ string, _ SandboxConfig) (string, error) {
	return "", fmt.Errorf("sandbox: unsupported platform")
}
