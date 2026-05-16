package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPanelDrawer is a test PanelDrawer implementation.
type mockPanelDrawer struct {
	id          string
	updateCount int
	drawCount   int
	lastArea    uv.Rectangle
	lastMsg     tea.Msg
}

func (m *mockPanelDrawer) Draw(_ uv.Screen, area uv.Rectangle) {
	m.drawCount++
	m.lastArea = area
}

func (m *mockPanelDrawer) Update(msg tea.Msg) (PanelDrawer, tea.Cmd) {
	m.updateCount++
	m.lastMsg = msg

	return m, nil
}
func (m *mockPanelDrawer) Handles(_ tea.Msg) bool { return true }

func TestPanelManager_Register(t *testing.T) {
	pm := NewPanelManager()
	assert.NotNil(t, pm.panels)
	assert.Empty(t, pm.AllPanels())

	drawer := &mockPanelDrawer{id: "test"}
	pm.Register(PanelConfig{ID: "p1", Title: "Panel 1"}, drawer)

	assert.True(t, pm.IsRegistered("p1"))
	assert.Equal(t, []string{"p1"}, pm.AllPanels())
}

func TestPanelManager_Register_Replace(t *testing.T) {
	pm := NewPanelManager()
	d1 := &mockPanelDrawer{id: "old"}
	d2 := &mockPanelDrawer{id: "new"}

	pm.Register(PanelConfig{ID: "p1", Title: "Old"}, d1)
	pm.Register(PanelConfig{ID: "p1", Title: "New"}, d2)

	entry, ok := pm.Get("p1")
	require.True(t, ok)
	assert.Equal(t, "New", entry.Config.Title)
}

func TestPanelManager_Register_Multiple(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p2"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p3"}, &mockPanelDrawer{})

	assert.Equal(t, []string{"p1", "p2", "p3"}, pm.AllPanels())
}

func TestPanelManager_Show(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})

	assert.False(t, pm.PanelVisible("p1"))

	pm.Show("p1")
	assert.True(t, pm.PanelVisible("p1"))
	assert.Equal(t, "p1", pm.Active())
}

func TestPanelManager_Show_Unknown(t *testing.T) {
	pm := NewPanelManager()
	pm.Show("unknown") // should not panic
	assert.Empty(t, pm.Active())
}

func TestPanelManager_Hide(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Show("p1")

	assert.True(t, pm.PanelVisible("p1"))

	pm.Hide("p1")
	assert.False(t, pm.PanelVisible("p1"))
	assert.Empty(t, pm.Active())
}

func TestPanelManager_Hide_ActiveSelectsNextVisible(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p2"}, &mockPanelDrawer{})
	pm.Show("p1")
	pm.Show("p2")

	assert.Equal(t, "p2", pm.Active())

	pm.Hide("p2")
	assert.Equal(t, "p1", pm.Active())
}

func TestPanelManager_Hide_LastVisibleClearsActive(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Show("p1")

	pm.Hide("p1")
	assert.Empty(t, pm.Active())
}

func TestPanelManager_Remove(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p2"}, &mockPanelDrawer{})
	pm.Show("p1")

	pm.Remove("p1")
	assert.False(t, pm.IsRegistered("p1"))
	assert.Empty(t, pm.Active())
	assert.Equal(t, []string{"p2"}, pm.AllPanels())
}

func TestPanelManager_Remove_ActiveSelectsNextVisible(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p2"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p3"}, &mockPanelDrawer{})
	pm.Show("p1")
	pm.Show("p2")
	pm.Show("p3")
	pm.Show("p2")

	pm.Remove("p2")
	assert.Equal(t, "p3", pm.Active())
}

func TestPanelManager_Remove_LastVisibleClearsActive(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Show("p1")

	pm.Remove("p1")
	assert.Empty(t, pm.Active())
}

func TestPanelManager_Remove_Unknown(t *testing.T) {
	pm := NewPanelManager()
	pm.Remove("unknown") // should not panic
}

func TestPanelManager_VisiblePanels(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p2"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p3"}, &mockPanelDrawer{})

	pm.Show("p1")
	pm.Show("p3")

	assert.Equal(t, []string{"p1", "p3"}, pm.VisiblePanels())
}

func TestPanelManager_VisiblePanels_Empty(t *testing.T) {
	pm := NewPanelManager()
	assert.Empty(t, pm.VisiblePanels())
}

func TestPanelManager_SetOrder_FiltersUnknownAndPreservesOmitted(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p2"}, &mockPanelDrawer{})
	pm.Register(PanelConfig{ID: "p3"}, &mockPanelDrawer{})
	pm.Show("p1")
	pm.Show("p2")
	pm.Show("p3")

	pm.SetOrder([]string{"p3", "missing", "p3"})

	assert.Equal(t, []string{"p3", "p1", "p2"}, pm.GetOrder())
	assert.Equal(t, []string{"p3", "p1", "p2"}, pm.VisiblePanels())
	assert.Equal(t, "p3", pm.Active())
}

func TestPanelManager_Get(t *testing.T) {
	pm := NewPanelManager()
	drawer := &mockPanelDrawer{id: "d1"}
	pm.Register(PanelConfig{ID: "p1", Title: "Test", Height: 5}, drawer)

	entry, ok := pm.Get("p1")
	require.True(t, ok)
	assert.Equal(t, "p1", entry.Config.ID)
	assert.Equal(t, "Test", entry.Config.Title)
	assert.Equal(t, 5, entry.Config.Height)
	assert.Equal(t, drawer, entry.Drawer)
	assert.False(t, entry.Visible)
}

func TestPanelManager_Get_Unknown(t *testing.T) {
	pm := NewPanelManager()
	_, ok := pm.Get("unknown")
	assert.False(t, ok)
}

func TestPanelManager_ActivePanelHeight(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1", Height: 15}, &mockPanelDrawer{})
	pm.Show("p1")

	assert.Equal(t, 15, pm.ActivePanelHeight())
}

func TestPanelManager_ActivePanelHeight_Default(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1"}, &mockPanelDrawer{})
	pm.Show("p1")

	assert.Equal(t, 10, pm.ActivePanelHeight())
}

func TestPanelManager_ActivePanelHeight_NoActive(t *testing.T) {
	pm := NewPanelManager()
	assert.Equal(t, 0, pm.ActivePanelHeight())
}

func TestPanelManager_ActivePanelPlacement(t *testing.T) {
	pm := NewPanelManager()
	pm.Register(PanelConfig{ID: "p1", Placement: BelowEditor}, &mockPanelDrawer{})
	pm.Show("p1")

	assert.Equal(t, BelowEditor, pm.ActivePanelPlacement())
}

func TestPanelManager_ActivePanelPlacement_Default(t *testing.T) {
	pm := NewPanelManager()
	assert.Equal(t, AsOverlay, pm.ActivePanelPlacement())
}

func TestPanelPlacement_Constants(t *testing.T) {
	assert.Equal(t, AsOverlay, PanelPlacement(0))
	assert.Equal(t, AboveEditor, PanelPlacement(1))
	assert.Equal(t, BelowEditor, PanelPlacement(2))
	assert.Equal(t, TrayOnly, PanelPlacement(3))
}

func TestPanelConfig_ZeroValues(t *testing.T) {
	cfg := PanelConfig{ID: "test"}
	assert.Equal(t, "test", cfg.ID)
	assert.Equal(t, AsOverlay, cfg.Placement)
	assert.Equal(t, 0, cfg.Width)
	assert.Equal(t, 0, cfg.Height)
	assert.Empty(t, cfg.Title)
}
