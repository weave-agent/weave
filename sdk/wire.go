package sdk

import (
	"fmt"
)

func Wire(config Config, bus Bus) error {
	names := config.GetStringSlice("extensions")
	if len(names) == 0 {
		return nil
	}

	for _, name := range names {
		ext, err := GetExtension(name)
		if err != nil {
			return fmt.Errorf("wire: %w", err)
		}
		ext.Subscribe(bus)
	}

	return nil
}
