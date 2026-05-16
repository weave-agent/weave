package tui

import (
	"slices"
	"sync"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// PanelPlacement determines where a panel is rendered relative to the editor.
type PanelPlacement int

const (
	AsOverlay PanelPlacement = iota
	AboveEditor
	BelowEditor
	TrayOnly
)

// PanelConfig configures a panel.
type PanelConfig struct {
	ID        string
	Placement PanelPlacement
	Blocking  bool // true = modal, false = non-blocking
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
	mu     sync.RWMutex
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
// Returns false if drawer is nil (panel not registered).
func (pm *PanelManager) Register(config PanelConfig, drawer PanelDrawer) bool {
	if drawer == nil {
		return false
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	visible := false
	if old, ok := pm.panels[config.ID]; ok {
		visible = old.Visible
	}

	pm.panels[config.ID] = &panelEntry{
		Config:  config,
		Drawer:  drawer,
		Visible: visible,
	}

	if !slices.Contains(pm.order, config.ID) {
		pm.order = append(pm.order, config.ID)
	}

	return true
}

// Show makes a panel visible and sets it as the active panel.
func (pm *PanelManager) Show(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	entry, ok := pm.panels[id]
	if !ok {
		return
	}

	entry.Visible = true
	pm.active = id
}

// Hide makes a panel invisible.
func (pm *PanelManager) Hide(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	entry, ok := pm.panels[id]
	if !ok {
		return
	}

	entry.Visible = false

	if pm.active == id {
		pm.active = pm.nextVisibleAfterLocked(id, pm.order)
	}
}

// Remove fully removes a panel from the manager.
func (pm *PanelManager) Remove(id string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	oldOrder := append([]string(nil), pm.order...)

	delete(pm.panels, id)

	newOrder := make([]string, 0, len(pm.order))
	for _, oid := range pm.order {
		if oid != id {
			newOrder = append(newOrder, oid)
		}
	}

	pm.order = newOrder

	if pm.active == id {
		pm.active = pm.nextVisibleAfterLocked(id, oldOrder)
	}
}

// PanelVisible returns true if a panel is currently visible.
func (pm *PanelManager) PanelVisible(id string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	entry, ok := pm.panels[id]
	if !ok {
		return false
	}

	return entry.Visible
}

// IsRegistered returns true if a panel is registered.
func (pm *PanelManager) IsRegistered(id string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	_, ok := pm.panels[id]

	return ok
}

// IsBlocking returns true if a panel is registered and configured as blocking.
func (pm *PanelManager) IsBlocking(id string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	entry, ok := pm.panels[id]
	if !ok {
		return false
	}

	return entry.Config.Blocking
}

// HasBlockingPanel returns true if any visible panel is configured as blocking.
func (pm *PanelManager) HasBlockingPanel() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, entry := range pm.panels {
		if entry.Visible && entry.Config.Blocking {
			return true
		}
	}

	return false
}

// Active returns the currently active panel ID.
func (pm *PanelManager) Active() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.active
}

// VisiblePanels returns IDs of all visible panels in tab order.
func (pm *PanelManager) VisiblePanels() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

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
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]string, len(pm.order))
	copy(result, pm.order)

	return result
}

// Get returns a copy of a panel entry by ID.
func (pm *PanelManager) Get(id string) (panelEntry, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	entry, ok := pm.panels[id]
	if !ok {
		return panelEntry{}, false
	}

	return *entry, true
}

// UpdateDrawer routes a message to a panel's drawer and stores the updated drawer.
// Returns the command and true if the panel exists and its drawer handled the message.
func (pm *PanelManager) UpdateDrawer(id string, msg tea.Msg) (tea.Cmd, bool) {
	pm.mu.RLock()

	entry, ok := pm.panels[id]
	if !ok || entry.Drawer == nil || !entry.Drawer.Handles(msg) {
		pm.mu.RUnlock()
		return nil, false
	}

	// Copy the drawer to avoid holding the lock during Update,
	// which prevents deadlock if the drawer calls back into PanelManager.
	drawer := entry.Drawer

	pm.mu.RUnlock()

	newDrawer, cmd := drawer.Update(msg)

	pm.mu.Lock()
	if e, stillOk := pm.panels[id]; stillOk && e.Drawer == drawer {
		e.Drawer = newDrawer
	}
	pm.mu.Unlock()

	return cmd, true
}

// DrawPanel draws a panel's drawer if the panel is visible.
// Returns true if the panel was drawn.
func (pm *PanelManager) DrawPanel(id string, scr uv.Screen, area uv.Rectangle) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	entry, ok := pm.panels[id]
	if !ok || !entry.Visible || entry.Drawer == nil {
		return false
	}

	entry.Drawer.Draw(scr, area)

	return true
}

// ActivePanelHeight returns the height of the active panel, or 0 if none.
func (pm *PanelManager) ActivePanelHeight() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

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

// SetOrder updates the tab order. Unknown IDs are ignored; registered panels
// omitted by ids are preserved after the requested IDs in their previous order.
func (pm *PanelManager) SetOrder(ids []string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	seen := make(map[string]bool, len(ids))
	newOrder := make([]string, 0, len(pm.order))

	for _, id := range ids {
		if seen[id] {
			continue
		}

		if _, ok := pm.panels[id]; !ok {
			continue
		}

		seen[id] = true
		newOrder = append(newOrder, id)
	}

	for _, id := range pm.order {
		if seen[id] {
			continue
		}

		if _, ok := pm.panels[id]; !ok {
			continue
		}

		seen[id] = true
		newOrder = append(newOrder, id)
	}

	pm.order = newOrder
	pm.ensureActiveLocked()
}

// GetOrder returns a copy of the current tab order.
func (pm *PanelManager) GetOrder() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]string, len(pm.order))
	copy(result, pm.order)

	return result
}

// ActivePanelPlacement returns the placement of the active panel.
func (pm *PanelManager) ActivePanelPlacement() PanelPlacement {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.active == "" {
		return AsOverlay
	}

	entry, ok := pm.panels[pm.active]
	if !ok {
		return AsOverlay
	}

	return entry.Config.Placement
}

func (pm *PanelManager) ensureActiveLocked() {
	if pm.active != "" {
		if entry, ok := pm.panels[pm.active]; ok && entry.Visible {
			return
		}
	}

	pm.active = pm.firstVisibleLocked()
}

func (pm *PanelManager) firstVisibleLocked() string {
	for _, id := range pm.order {
		if entry, ok := pm.panels[id]; ok && entry.Visible {
			return id
		}
	}

	return ""
}

func (pm *PanelManager) nextVisibleAfterLocked(id string, order []string) string {
	if len(order) == 0 {
		return ""
	}

	idx := slices.Index(order, id)
	for offset := 1; offset <= len(order); offset++ {
		candidate := order[(idx+offset+len(order))%len(order)]
		if entry, ok := pm.panels[candidate]; ok && entry.Visible {
			return candidate
		}
	}

	return ""
}
