package main

import (
	"os"
	"testing"
)

func TestRunFlagParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"invalid flag", []string{"-xyz"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := run(tt.args...)
			if exitCode != tt.wantCode {
				t.Errorf("run(%v) = %d, want %d", tt.args, exitCode, tt.wantCode)
			}
		})
	}
}

func TestRunMissingConfig(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.Chdir(origWd) }()

	exitCode := run()
	if exitCode != 1 {
		t.Errorf("run() in empty dir = %d, want 1", exitCode)
	}
}

func TestRunExtensionOverride(t *testing.T) {
	dir := t.TempDir()

	cfgFile := dir + "/.weave.yaml"
	if err := os.WriteFile(cfgFile, []byte("extensions: [noop]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.Chdir(origWd) }()

	exitCode := run("-e", "ext1,ext2")
	if exitCode != 1 {
		t.Errorf("run with -e flag returned %d, want 1 (expected failure at discovery)", exitCode)
	}
}
