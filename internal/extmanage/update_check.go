package extmanage

import (
	"fmt"
	"os"
	"sync"

	"github.com/weave-agent/weave/sdk"
)

// FireUpdateCheck scans user-installed extensions for available updates.
// It lists git-sourced extensions, compares HEAD to the remote, and
// publishes an "extension.outdated" event if any are behind.
// Skipped entirely when WEAVE_OFFLINE=1 is set.
func FireUpdateCheck(bus sdk.Bus) {
	if os.Getenv("WEAVE_OFFLINE") == "1" {
		fmt.Fprintln(os.Stderr, "weave: skipping update check (offline mode)")
		return
	}

	fmt.Fprintln(os.Stderr, "weave: checking for extension updates...")

	exts, err := listExtensionsDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: update check: %v\n", err)
		return
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		outdated []sdk.OutdatedInfo
	)

	for i := range exts {
		if exts[i].Source != sourceGit {
			continue
		}

		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			if err := checkOutdated(&exts[i]); err != nil {
				fmt.Fprintf(os.Stderr, "weave: update check: %v\n", err)
				return
			}

			if exts[i].Outdated {
				mu.Lock()

				outdated = append(outdated, sdk.OutdatedInfo{
					Name:       exts[i].Name,
					LocalHead:  exts[i].LocalHead,
					RemoteHead: exts[i].RemoteHead,
				})
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	if len(outdated) > 0 {
		bus.Publish(sdk.NewEvent("extension.outdated", sdk.OutdatedEvent{Extensions: outdated}))
	}

	fmt.Fprintf(os.Stderr, "weave: update check complete (%d outdated)\n", len(outdated))
}
