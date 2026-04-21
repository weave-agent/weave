package sdk

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAndGetUI(t *testing.T) {
	ResetUIRegistry()

	RegisterUI("tui", NoopUI{})

	got, err := GetUI("tui")
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestDuplicateUIRegistration(t *testing.T) {
	ResetUIRegistry()

	RegisterUI("dup", NoopUI{})

	defer func() {
		require.NotNil(t, recover(), "expected panic on duplicate UI registration")
	}()

	RegisterUI("dup", NoopUI{})
}

func TestMissingUI(t *testing.T) {
	ResetUIRegistry()

	_, err := GetUI("nonexistent")
	require.Error(t, err, "expected error for missing UI")
}

func TestListUIs(t *testing.T) {
	ResetUIRegistry()

	RegisterUI("tui", NoopUI{})
	RegisterUI("curses", NoopUI{})

	names := ListUIs()
	sort.Strings(names)

	assert.Equal(t, []string{"curses", "tui"}, names)
}

func TestResetUIRegistry(t *testing.T) {
	ResetUIRegistry()

	RegisterUI("temp", NoopUI{})

	ResetUIRegistry()

	assert.Empty(t, ListUIs())
}
