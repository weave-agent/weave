//go:build darwin

package sandbox

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed seatbelt_base_policy.sbpl
var seatbeltBasePolicy string

//go:embed seatbelt_network_policy.sbpl
var seatbeltNetworkPolicy string

//go:embed seatbelt_readonly_defaults.sbpl
var seatbeltReadonlyDefaults string

func generateSeatbeltProfile(cfg SandboxConfig, dir string) (string, []string, error) {
	home, _ := os.UserHomeDir()

	if dir == "" {
		dir, _ = os.Getwd()
	}

	var sections []string
	var params []string

	// Base policy: process rules, sysctls, mach lookups, /dev/null, PTYs
	sections = append(sections, seatbeltBasePolicy)

	// File read policy
	readPolicy, readParams := buildReadPolicy(cfg, home, dir)
	if readPolicy != "" {
		sections = append(sections, readPolicy)
	}
	params = append(params, readParams...)

	// File write policy
	writePolicy, writeParams := buildWritePolicy(cfg, home, dir)
	if writePolicy != "" {
		sections = append(sections, writePolicy)
	}
	params = append(params, writeParams...)

	// Deny read rules (mandatory + user-configured)
	denyReadPolicy := buildDenyReadPolicy(cfg, home, dir)
	if denyReadPolicy != "" {
		sections = append(sections, denyReadPolicy)
	}

	// Network policy
	networkPolicy := buildNetworkPolicy(cfg)
	if networkPolicy != "" {
		sections = append(sections, networkPolicy)
	}

	// User temp dir (xcrun, many tools use $TMPDIR under /var/folders)
	tmpDir := os.TempDir()
	if tmpDir != "" && tmpDir != "/tmp" && tmpDir != "/private/tmp" && tmpDir != "/var/tmp" && tmpDir != "/private/var/tmp" {
		sections = append(sections, fmt.Sprintf("(allow file-read* file-write* (subpath %q))", tmpDir))
	}

	// Platform defaults: system frameworks, /bin, /usr/bin, temp dirs, etc.
	sections = append(sections, seatbeltReadonlyDefaults)

	return strings.Join(sections, "\n"), params, nil
}

func buildReadPolicy(_ SandboxConfig, _, _ string) (string, []string) {
	// Broad read access with targeted deny rules (see buildDenyReadPolicy).
	// File reads are low-risk for a coding agent — the agent can already
	// read files via the read tool.  The real protection is around writes.
	return "(allow file-read*)\n(allow file-test-existence)", nil
}

func buildWritePolicy(cfg SandboxConfig, home, dir string) (string, []string) {
	var parts []string
	var params []string

	writable := cfg.Writable
	if len(writable) == 1 && writable[0] == "" {
		writable = nil
	} else if len(writable) == 0 {
		writable = []string{dir}
	}

	for i, w := range writable {
		if w == "." {
			w = dir
		}
		param := fmt.Sprintf("WRITABLE_ROOT_%d", i)
		params = append(params, fmt.Sprintf("%s=%s", param, w))
		parts = append(parts, fmt.Sprintf("(subpath (param \"%s\"))", param))
	}

	// Mandatory write deny rules: expand and add as require-not clauses
	for i, deny := range mandatoryDenyWritePaths {
		expanded := expandDenyRule(deny, home, dir)
		param := fmt.Sprintf("WRITABLE_DENY_%d", i)
		params = append(params, fmt.Sprintf("%s=%s", param, expanded))
		if strings.HasSuffix(deny, "/") {
			parts = append(parts, fmt.Sprintf("(require-not (subpath (param \"%s\")))", param))
		} else {
			parts = append(parts, fmt.Sprintf("(require-not (literal (param \"%s\")))", param))
		}
	}

	// User-configured deny write paths
	offset := len(mandatoryDenyWritePaths)
	for i, deny := range cfg.DenyWrite {
		expanded := expandDenyRule(deny, home, dir)
		param := fmt.Sprintf("WRITABLE_DENY_%d", offset+i)
		params = append(params, fmt.Sprintf("%s=%s", param, expanded))
		if strings.HasSuffix(deny, "/") {
			parts = append(parts, fmt.Sprintf("(require-not (subpath (param \"%s\")))", param))
		} else {
			parts = append(parts, fmt.Sprintf("(require-not (literal (param \"%s\")))", param))
		}
	}

	if len(parts) == 0 {
		return "", nil
	}

	policy := fmt.Sprintf("(allow file-write*\n  (require-all\n    %s\n  )\n)", strings.Join(parts, "\n    "))
	return policy, params
}

func buildDenyReadPolicy(cfg SandboxConfig, home, dir string) string {
	var parts []string

	// Mandatory read deny rules
	sshDir := filepath.Join(home, ".ssh")
	parts = append(parts, fmt.Sprintf("(deny file-read* (regex #\"^%s/id_.*$\"))", regexp.QuoteMeta(sshDir)))
	parts = append(parts, fmt.Sprintf("(deny file-read* (literal %q))", filepath.Join(home, ".aws", "credentials")))
	parts = append(parts, "(deny file-read* (regex #\"/\\.env$\") (regex #\"/\\.env\\.[^/]+$\"))")

	// User-configured deny read paths
	for _, deny := range cfg.DenyRead {
		expanded := expandDenyRule(deny, home, dir)
		if strings.HasSuffix(deny, "/") {
			parts = append(parts, fmt.Sprintf("(deny file-read* (subpath %q))", strings.TrimSuffix(expanded, "/")))
		} else {
			parts = append(parts, fmt.Sprintf("(deny file-read* (literal %q))", expanded))
		}
	}

	return strings.Join(parts, "\n")
}

func buildNetworkPolicy(cfg SandboxConfig) string {
	if cfg.Network {
		return "(allow network-outbound)\n(allow network-inbound)\n" + seatbeltNetworkPolicy
	}
	return "(deny network*)\n"
}

func wrapCommandDarwinWithConfig(cmd, dir string, cfg SandboxConfig) (string, error) {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return "", fmt.Errorf("sandbox-exec not found: %w", err)
	}

	profile, params, err := generateSeatbeltProfile(cfg, dir)
	if err != nil {
		return "", fmt.Errorf("generate seatbelt profile: %w", err)
	}

	escapedProfile := strings.ReplaceAll(profile, "'", "'\\''")

	var args []string
	args = append(args, "-p", "'"+escapedProfile+"'")
	for _, param := range params {
		args = append(args, "-D"+param)
	}
	args = append(args, "bash", "-c", "'"+strings.ReplaceAll(cmd, "'", "'\\''")+"'")

	return "sandbox-exec " + strings.Join(args, " "), nil
}

func wrapCommandLinuxWithConfig(cmd, dir string, cfg SandboxConfig) (string, error) {
	return cmd, nil
}
