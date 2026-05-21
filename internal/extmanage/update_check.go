package extmanage

import (
	"log/slog"
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
		slog.Info("skipping update check", "reason", "offline mode")
		return
	}

	slog.Info("checking for extension updates")

	exts, err := listExtensionsDir()
	if err != nil {
		slog.Warn("extension update check failed", "error", err)
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
				slog.Warn("extension update check failed", "extension", exts[i].Name, "error", err)
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

	slog.Info("extension update check complete", "outdated", len(outdated))
}
