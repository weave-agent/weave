package ripgrep

import (
	"os/exec"
	"sync"
)

// Find returns the path to the rg binary, or empty string if not found.
// The result is cached after the first call.
var Find = sync.OnceValue(func() string {
	path, err := exec.LookPath("rg")
	if err != nil {
		return ""
	}
	return path
})
