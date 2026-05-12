package extmanage

import (
	"fmt"
	"os"
	"sync"
)

// RunList prints a formatted table of installed extensions to stdout.
// It checks git-sourced extensions for available updates.
func RunList(args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: weave list")

		return 1
	}

	exts, err := listExtensionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave list: %v\n", err)
		return 1
	}

	if len(exts) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "no extensions installed")
		return 0
	}

	// Check outdated for git-sourced extensions.
	var wg sync.WaitGroup

	for i := range exts {
		if exts[i].Source != sourceGit {
			continue
		}

		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			if err := checkOutdated(&exts[i]); err != nil {
				exts[i].CheckErr = err.Error()
				fmt.Fprintf(os.Stderr, "weave list: warning: %v\n", err)
			}
		}(i)
	}

	wg.Wait()

	_, _ = fmt.Fprintf(os.Stdout, "%-20s %-10s %s\n", "NAME", "SOURCE", "STATUS")

	for _, ext := range exts {
		sourceLabel := "local"
		status := "ok"

		if ext.Source == sourceGit {
			sourceLabel = "git"

			switch {
			case ext.CheckErr != "":
				status = "unknown"
			case ext.Outdated:
				status = "outdated"
			}
		} else {
			status = "static"
		}

		_, _ = fmt.Fprintf(os.Stdout, "%-20s %-10s %s\n", ext.Name, sourceLabel, status)
	}

	return 0
}
