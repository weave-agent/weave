//go:build !darwin && !linux

package sandbox

func wrapCommandDarwin(cmd, _ string) (string, error) {
	return cmd, nil
}

func generateSeatbeltProfile(_ SandboxConfig, _ string) string {
	return ""
}

func wrapCommandDarwinWithConfig(cmd, _ string, _ SandboxConfig) (string, error) {
	return cmd, nil
}

func wrapCommandLinux(cmd, _ string) (string, error) {
	return cmd, nil
}

func wrapCommandLinuxWithConfig(cmd, _ string, _ SandboxConfig) (string, error) {
	return cmd, nil
}
