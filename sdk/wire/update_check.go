package wire

import (
	"fmt"
	"os"

	"weave/sdk"
)

// OutdatedInfo describes a single extension that has a newer version available.
type OutdatedInfo struct {
	Name       string
	LocalHead  string
	RemoteHead string
}

// OutdatedEvent is the payload for the "extension.outdated" bus event.
type OutdatedEvent struct {
	Extensions []OutdatedInfo
}

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

	var outdated []OutdatedInfo

	for i := range exts {
		if exts[i].Source != sourceGit {
			continue
		}

		if err := checkOutdated(&exts[i]); err != nil {
			fmt.Fprintf(os.Stderr, "weave: update check: %v\n", err)
			continue
		}

		if exts[i].Outdated {
			outdated = append(outdated, OutdatedInfo{
				Name:       exts[i].Name,
				LocalHead:  exts[i].LocalHead,
				RemoteHead: exts[i].RemoteHead,
			})
		}
	}

	if len(outdated) > 0 {
		bus.Publish(sdk.Event{
			Topic:   "extension.outdated",
			Payload: OutdatedEvent{Extensions: outdated},
		})
	}

	fmt.Fprintf(os.Stderr, "weave: update check complete (%d outdated)\n", len(outdated))
}
