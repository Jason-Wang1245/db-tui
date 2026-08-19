package grid

import tea "charm.land/bubbletea/v2"

func (model *Model) updateMouse(mouse tea.Mouse) tea.Cmd {
	if mouse.Button != tea.MouseLeft || model.relation == nil || mouse.Y < 4 {
		return nil
	}
	row := model.rowOffset + mouse.Y - 4
	if row < 0 || row >= len(model.page.Rows) {
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

func (model *Model) updateWheel(mouse tea.Mouse) tea.Cmd {
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
