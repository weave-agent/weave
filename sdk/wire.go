package sdk

import (
	"errors"
	"fmt"
)

// Wired holds all extensions wired to a bus. Call Close to shut down.
type Wired struct {
	extensions []Extension
	bus        Bus
}

func Wire(extNames []string, bus Bus, cfg Config) (*Wired, error) {
	exts := make([]Extension, 0, len(extNames))

	for _, name := range extNames {
		ext, err := GetExtension(name, cfg)
		if err != nil {
			for i := len(exts) - 1; i >= 0; i-- {
				_ = exts[i].Close()
			}

			return nil, fmt.Errorf("wire: %w", err)
		}

		ext.Subscribe(bus)
		exts = append(exts, ext)
	}

	return &Wired{extensions: exts, bus: bus}, nil
}

func (w *Wired) Close() error {
	var errs []error

	for i := len(w.extensions) - 1; i >= 0; i-- {
		if err := w.extensions[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close: %w", errors.Join(errs...))
	}

	return nil
}
