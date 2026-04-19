package main

import (
	"os"
	"strings"
	"testing"
)

func TestMergeUnique(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, []string{}},
		{"single", []string{"a"}, []string{"a"}},
		{"no dupes", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"removes dupes", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"all same", []string{"x", "x", "x"}, []string{"x"}},
		{"preserves order", []string{"loop", "anthropic", "bash"}, []string{"loop", "anthropic", "bash"}},
		{"core plus optional overlap", []string{"loop", "anthropic", "anthropic"}, []string{"loop", "anthropic"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeUnique(tt.in)
			if len(got) != len(tt.want) {
				t.Errorf("mergeUnique(%v) = %v, want %v", tt.in, got, tt.want)
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("mergeUnique(%v)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

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

func TestRunCoreDefaultsUsed(t *testing.T) {
	dir := t.TempDir()

	cfgFile := dir + "/.weave.yaml"
	if err := os.WriteFile(cfgFile, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create a go.mod so findModuleRoot succeeds and the test gets to the launcher.
	if err := os.WriteFile(dir+"/go.mod", []byte("module weave\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.Chdir(origWd) }()

	// Capture stderr to verify the error mentions core default extension names.
	old := os.Stderr

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	os.Stderr = w

	exitCode := run()

	_ = w.Close()

	os.Stderr = old

	buf := make([]byte, 4096)

	n, _ := r.Read(buf)

	_ = r.Close()

	stderr := string(buf[:n])

	if exitCode != 1 {
		t.Errorf("run() = %d, want 1", exitCode)
	}

	if !strings.Contains(stderr, "loop") && !strings.Contains(stderr, "anthropic") {
		t.Errorf("stderr = %q, want mention of 'loop' or 'anthropic' (core defaults)", stderr)
	}
}
