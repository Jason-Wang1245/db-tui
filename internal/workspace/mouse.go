package workspace

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

type HitboxKind string

const (
	HitboxTree       HitboxKind = "tree"
	HitboxTab        HitboxKind = "tab"
	HitboxCloseTab   HitboxKind = "close-tab"
	HitboxNewSQL     HitboxKind = "new-sql"
	HitboxContent    HitboxKind = "content"
	HitboxHelp       HitboxKind = "help"
	HitboxReconnect  HitboxKind = "reconnect"
	HitboxDisconnect HitboxKind = "disconnect"
	HitboxModalSafe  HitboxKind = "modal-safe"
	HitboxModalApply HitboxKind = "modal-apply"
)

type Hitbox struct {
	Rect  ui.Rect
	Kind  HitboxKind
	Index int
}

func (model *Model) rebuildHitboxes() {
	model.hitboxes = model.hitboxes[:0]
	if model.width <= 0 || model.height <= 0 {
		return
	}
	if model.modal.Kind != modalNone {
		safe, destructive := modalHitboxes(model.width, model.height)
		model.hitboxes = append(model.hitboxes,
			Hitbox{Rect: safe, Kind: HitboxModalSafe},
			Hitbox{Rect: destructive, Kind: HitboxModalApply},
		)
		return
	}
	contentX := 0
	contentY := 2
	contentWidth := model.width
	if model.layout == LayoutWide {
		navigatorWidth := navigatorWidth(model.width)
		model.appendTreeHitboxes(0, 2, navigatorWidth)
		contentX = navigatorWidth + 1
		contentWidth = model.width - contentX
	} else if model.focus == FocusNavigator {
		model.appendTreeHitboxes(0, 2, min(model.width, 34))
	}

	x := contentX
	if model.focus == FocusTabs {
		x += 2
	}
	indices := model.visibleTabIndices(contentWidth)
	if len(indices) > 0 && indices[0] > 0 {
		x += 2
	}
	for _, index := range indices {
		tab := model.tabs[index]
		width := min(contentWidth, displayWidth(model.tabLabel(tab)))
		if width < 4 || x+width > contentX+contentWidth {
			break
		}
		model.hitboxes = append(model.hitboxes,
			Hitbox{Rect: ui.Rect{X: x + width - 2, Y: 1, Width: 1, Height: 1}, Kind: HitboxCloseTab, Index: index},
			Hitbox{Rect: ui.Rect{X: x, Y: 1, Width: width - 2, Height: 1}, Kind: HitboxTab, Index: index},
		)
		x += width + 1
	}
	if x+7 <= contentX+contentWidth {
		model.hitboxes = append(model.hitboxes, Hitbox{Rect: ui.Rect{X: x, Y: 1, Width: 7, Height: 1}, Kind: HitboxNewSQL})
	}
	model.hitboxes = append(model.hitboxes,
		Hitbox{Rect: ui.Rect{X: contentX, Y: contentY, Width: contentWidth, Height: max(0, model.height-4)}, Kind: HitboxContent},
		Hitbox{Rect: ui.Rect{X: max(0, model.width-8), Y: model.height - 1, Width: 8, Height: 1}, Kind: HitboxHelp},
	)
	if model.connection == ConnectionLost {
		model.hitboxes = append(model.hitboxes,
			Hitbox{Rect: ui.Rect{X: 0, Y: model.height - 2, Width: 11, Height: 1}, Kind: HitboxReconnect},
			Hitbox{Rect: ui.Rect{X: 12, Y: model.height - 2, Width: 12, Height: 1}, Kind: HitboxDisconnect},
		)
	}
}

func (model *Model) appendTreeHitboxes(x, y, width int) {
	items := model.visibleItems()
	visible := max(0, model.height-4)
	for row := 0; row < visible && model.treeOffset+row < len(items); row++ {
		model.hitboxes = append(model.hitboxes, Hitbox{
			Rect: ui.Rect{X: x, Y: y + row, Width: width, Height: 1}, Kind: HitboxTree, Index: model.treeOffset + row,
		})
	}
}

func (model *Model) updateMouse(mouse tea.Mouse) tea.Cmd {
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	for _, hitbox := range model.hitboxes {
		if !hitbox.Rect.Contains(mouse.X, mouse.Y) {
			continue
		}
		switch hitbox.Kind {
		case HitboxTree:
			model.focus = FocusNavigator
			model.selectedTree = hitbox.Index
			return model.activateTreeItem()
		case HitboxTab:
			if hitbox.Index >= 0 && hitbox.Index < len(model.tabs) {
				model.focus = FocusTabs
				model.activeTab = model.tabs[hitbox.Index].Envelope.ID
				model.status = "Active tab: " + model.tabs[hitbox.Index].Envelope.Title + "."
			}
		case HitboxCloseTab:
			if hitbox.Index >= 0 && hitbox.Index < len(model.tabs) {
				model.focus = FocusTabs
				model.activeTab = model.tabs[hitbox.Index].Envelope.ID
				return model.closeActiveTab()
			}
		case HitboxNewSQL:
			model.focus = FocusTabs
			model.openSQLTab()
		case HitboxContent:
			if model.activeTab != "" {
				model.focus = FocusContent
				model.rememberFocus()
			}
		case HitboxHelp:
			model.previousFocus = model.focus
			model.help = true
		case HitboxReconnect:
			return model.reconnect()
		case HitboxDisconnect:
			return model.requestLeave(modalDisconnect)
		case HitboxModalSafe:
			model.modal.Destructive = false
			return model.updateModalKey("enter")
		case HitboxModalApply:
			model.modal.Destructive = true
			return model.updateModalKey("enter")
		}
		model.rebuildHitboxes()
		return nil
	}
	return nil
}

func modalHitboxes(width, height int) (ui.Rect, ui.Rect) {
	y := min(max(0, height-1), max(0, height/2-3)+4)
	totalWidth := 33
	x := max(0, (width-totalWidth)/2)
	return ui.Rect{X: x, Y: y, Width: 12, Height: 1}, ui.Rect{X: x + 13, Y: y, Width: 20, Height: 1}
}

func (model *Model) updateWheel(mouse tea.Mouse) tea.Cmd {
	if model.layout != LayoutWide && model.focus != FocusNavigator {
		return nil
	}
	if model.focus != FocusNavigator && model.layout == LayoutWide {
		// Wheel scrolling is routed to the pane under the pointer without moving focus.
		if mouse.X > navigatorWidth(model.width) {
			return nil
		}
	}
	if mouse.Button == tea.MouseWheelUp {
		model.moveTree(-1)
	} else if mouse.Button == tea.MouseWheelDown {
		model.moveTree(1)
	}
	return nil
}

func displayWidth(value string) int {
	return ansi.StringWidth(value)
}
