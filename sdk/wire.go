package sdk

import (
	"fmt"
)

func Wire(extNames []string, bus Bus) error {
	for _, name := range extNames {
		ext, err := GetExtension(name)
		if err != nil {
			return fmt.Errorf("wire: %w", err)
		}
		ext.Subscribe(bus)
	}

	return nil
}
