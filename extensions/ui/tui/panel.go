package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// PanelPlacement determines where a panel is rendered relative to the editor.
type PanelPlacement int

const (
	AsOverlay PanelPlacement = iota
	AboveEditor
	BelowEditor
)

// PanelConfig configures a panel.
type PanelConfig struct {
	ID        string
	Placement PanelPlacement
	Blocking  bool
	Width     int
	Height    int
	Title     string
}

// PanelDrawer is the interface for panel content rendering and interaction.
type PanelDrawer interface {
	Draw(scr uv.Screen, area uv.Rectangle)
	Update(msg tea.Msg) (PanelDrawer, tea.Cmd)
	Handles(msg tea.Msg) bool
}

// panelEntry holds a registered panel's state.
type panelEntry struct {
	Config  PanelConfig
	Drawer  PanelDrawer
	Visible bool
}

// PanelManager tracks registered panels (show/hide/remove/visible).
type PanelManager struct {
	panels map[string]*panelEntry
	order  []string
	active string
}

// NewPanelManager creates a new PanelManager.
func NewPanelManager() *PanelManager {
	return &PanelManager{
		panels: make(map[string]*panelEntry),
	}
}

// Register registers a panel. If a panel with the same ID exists, it is replaced.
func (pm *PanelManager) Register(config PanelConfig, drawer PanelDrawer) {
	pm.panels[config.ID] = &panelEntry{
		Config:  config,
		Drawer:  drawer,
		Visible: false,
	}

	if !slices.Contains(pm.order, config.ID) {
		pm.order = append(pm.order, config.ID)
	}
}

// Show makes a panel visible and sets it as the active panel.
func (pm *PanelManager) Show(id string) {
	entry, ok := pm.panels[id]
	if !ok {
		return
	}

	entry.Visible = true
	pm.active = id
}

// Hide makes a panel invisible.
func (pm *PanelManager) Hide(id string) {
	entry, ok := pm.panels[id]
	if !ok {
		return
	}

	entry.Visible = false

	if pm.active == id {
		pm.active = ""
	}
}

// Remove fully removes a panel from the manager.
func (pm *PanelManager) Remove(id string) {
	delete(pm.panels, id)

	newOrder := make([]string, 0, len(pm.order))
	for _, oid := range pm.order {
		if oid != id {
			newOrder = append(newOrder, oid)
		}
	}

	pm.order = newOrder

	if pm.active == id {
		pm.active = ""
	}
}

// PanelVisible returns true if a panel is currently visible.
func (pm *PanelManager) PanelVisible(id string) bool {
	entry, ok := pm.panels[id]
	if !ok {
		return false
	}

	return entry.Visible
}

// IsRegistered returns true if a panel is registered.
func (pm *PanelManager) IsRegistered(id string) bool {
	_, ok := pm.panels[id]
	return ok
}

// Active returns the currently active panel ID.
func (pm *PanelManager) Active() string {
	return pm.active
}

// VisiblePanels returns IDs of all visible panels in tab order.
func (pm *PanelManager) VisiblePanels() []string {
	var result []string

	for _, id := range pm.order {
		if entry, ok := pm.panels[id]; ok && entry.Visible {
			result = append(result, id)
		}
	}

	return result
}

// AllPanels returns all registered panel IDs.
func (pm *PanelManager) AllPanels() []string {
	result := make([]string, len(pm.order))
	copy(result, pm.order)

	return result
}

// Get returns a panel entry by ID.
func (pm *PanelManager) Get(id string) (*panelEntry, bool) {
	entry, ok := pm.panels[id]
	return entry, ok
}

// ActivePanelHeight returns the height of the active panel, or 0 if none.
func (pm *PanelManager) ActivePanelHeight() int {
	if pm.active == "" {
		return 0
	}

	entry, ok := pm.panels[pm.active]
	if !ok || !entry.Visible {
		return 0
	}

	if entry.Config.Height > 0 {
		return entry.Config.Height
	}

	return 10 // default panel height
}

// ActivePanelPlacement returns the placement of the active panel.
func (pm *PanelManager) ActivePanelPlacement() PanelPlacement {
	if pm.active == "" {
		return AsOverlay
	}

	entry, ok := pm.panels[pm.active]
	if !ok {
		return AsOverlay
	}

	return entry.Config.Placement
}
