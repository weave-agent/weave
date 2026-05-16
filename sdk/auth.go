package sdk

import (
	"fmt"

	"weave/internal/auth"
)

// ClearProviderAuth removes all authentication data for a provider from the
// auth file.
func ClearProviderAuth(providerName string) error {
	if err := auth.ClearProviderAuth(providerName); err != nil {
		return fmt.Errorf("clear provider auth: %w", err)
	}

	return nil
}
