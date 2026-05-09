//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func bwrapAvailable() error {
	return nil
}

func wrapCommandDarwin(cmd, dir string) (string, error) {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return cmd, nil
	}

	profile := generateSeatbeltProfile(dir)
	escaped := strings.ReplaceAll(profile, "'", "'\\''")
	return fmt.Sprintf("sandbox-exec -p '%s' bash -c '%s'", escaped, strings.ReplaceAll(cmd, "'", "'\\''")), nil
}

func generateSeatbeltProfile(dir string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	b.WriteString("(allow file-read*)\n")

	if dir == "" {
		dir, _ = os.Getwd()
	}
	b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", dir))

	b.WriteString("(allow process-exec)\n")
	b.WriteString("(allow process-fork)\n")

	b.WriteString("(allow network*)\n")

	home, _ := os.UserHomeDir()

	denyDirs := []string{
		filepath.Join(home, ".ssh"),
	}
	for _, d := range denyDirs {
		b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", d))
	}

	denyFiles := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".gitconfig"),
	}
	for _, f := range denyFiles {
		b.WriteString(fmt.Sprintf("(deny file-write* (literal %q))\n", f))
	}

	b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", filepath.Join(dir, ".git", "hooks")))
	b.WriteString(fmt.Sprintf("(deny file-write* (literal %q))\n", filepath.Join(dir, ".git", "config")))
	b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", filepath.Join(dir, ".weave")))

	return b.String()
}

func wrapCommandLinux(cmd, dir string) (string, error) {
	return cmd, nil
}
