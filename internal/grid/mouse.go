package grid

import tea "charm.land/bubbletea/v2"

func (model *Model) updateMouse(mouse tea.Mouse) tea.Cmd {
	if mouse.Button != tea.MouseLeft || model.relation == nil {
		return nil
	}
	if model.editor != nil {
		return nil
	}
	if model.overlay != overlayNone {
		return model.updateOverlayMouse(mouse)
	}
	if mouse.Y == 2 {
		switch gridActionAt(mouse.X) {
		case "insert":
			model.stageInsert()
		case "edit":
			model.openEditor()
		case "null":
			model.stageSpecial(ValueNull)
		case "default":
			model.stageSpecial(ValueDefault)
		case "delete":
			model.toggleDelete()
		case "apply":
			return model.requestApplyConfirmation()
		case "revert":
			model.revertSelectedRow()
		case "revert-all":
			if model.Dirty() {
				model.overlay = overlayRevertAll
				model.overlayScroll = 0
			}
		case "changes":
			if model.Dirty() {
				model.overlay = overlayChanges
				model.overlayScroll = 0
			}
		}
		return nil
	}
	if mouse.Y < 5 {
		return nil
	}
	row := model.rowOffset + mouse.Y - 5
	if row < 0 || row >= model.totalRows() {
		return nil
	}
	model.selectedRow = row
	x := 2
	for _, column := range model.visibleColumns() {
		if mouse.X >= x && mouse.X < x+model.columnWidths[column] {
			model.selectedColumn = column
			model.ensureColumnVisible()
			break
		}
		x += model.columnWidths[column] + 1
	}
	model.ensureRowVisible()
	return nil
}

func gridActionAt(x int) string {
	labels := []struct {
		name  string
		label string
	}{
		{"insert", "[Insert i]"},
		{"edit", "[Edit e]"},
		{"null", "[NULL z]"},
		{"default", "[Default f]"},
		{"delete", "[Delete d]"},
		{"apply", "[Apply a]"},
		{"revert", "[Revert row u]"},
		{"revert-all", "[Revert all U]"},
		{"changes", "[Changes v]"},
	}
	position := 0
	for _, item := range labels {
		if x >= position && x < position+len(item.label) {
			return item.name
		}
		position += len(item.label) + 1
	}
	return ""
}

func (model *Model) updateOverlayMouse(mouse tea.Mouse) tea.Cmd {
	switch model.overlay {
	case overlayApply:
		if mouse.Y == 4 {
			if mouse.X >= 0 && mouse.X < len("[Apply Enter/a]") {
				return model.startApply()
			}
			if mouse.X >= len("[Apply Enter/a]  ") && mouse.X < len("[Apply Enter/a]  [Cancel Esc]") {
				model.overlay = overlayNone
			}
		}
	case overlayRefresh:
		if mouse.Y == 4 {
			switch {
			case mouse.X < len("[Apply a]"):
				if issue := model.validateChanges(); issue != "" {
					model.overlay = overlayNone
					model.status = issue
					return nil
				}
				model.applyReloadFirst = true
				return model.startApply()
			case mouse.X >= len("[Apply a]  ") && mouse.X < len("[Apply a]  [Revert and refresh r]"):
				model.clearChanges()
				return model.startPage(PageRequest{Relation: model.relationID, Direction: PageFirst, Sort: model.sort, Limit: DefaultPageSize}, 1)
			default:
				model.overlay = overlayNone
			}
		}
	case overlayRevertAll:
		if mouse.Y == 4 {
			if mouse.X < len("[Revert all Enter/r]") {
				model.clearChanges()
				model.status = "Reverted every staged change in this tab."
			} else {
				model.overlay = overlayNone
			}
		}
	}
	return nil
}

func (model *Model) updateWheel(mouse tea.Mouse) tea.Cmd {
	if model.overlay == overlayApply || model.overlay == overlayChanges {
		if mouse.Button == tea.MouseWheelUp {
			model.overlayScroll = max(0, model.overlayScroll-3)
		} else if mouse.Button == tea.MouseWheelDown {
			model.overlayScroll = min(max(0, len(model.changeSummaries())-1), model.overlayScroll+3)
		}
		return nil
	}
	if mouse.Mod&tea.ModShift != 0 {
		if mouse.Button == tea.MouseWheelUp || mouse.Button == tea.MouseWheelLeft {
			model.moveColumn(-1)
		} else if mouse.Button == tea.MouseWheelDown || mouse.Button == tea.MouseWheelRight {
			model.moveColumn(1)
		}
		return nil
	}
	if mouse.Button == tea.MouseWheelUp {
		model.moveRow(-3)
	} else if mouse.Button == tea.MouseWheelDown {
		model.moveRow(3)
	}
	return nil
}
