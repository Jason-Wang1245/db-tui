package workspace

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/Jason-Wang1245/db-tui/internal/ui"
)

type ContentView struct {
	Lines              []string
	Hints              string
	Status             string
	ConsumesGlobalKeys bool
}

func (model Model) View(theme ui.Theme) string {
	return model.view(theme, ContentView{})
}

func (model Model) ViewWithContent(theme ui.Theme, content ContentView) string {
	return model.view(theme, content)
}

func (model Model) view(theme ui.Theme, content ContentView) string {
	if model.modal.Kind != modalNone {
		return model.modalView(theme)
	}
	if model.help {
		return model.helpView(theme)
	}
	width := model.width
	if width <= 0 {
		width = 100
	}
	height := model.height
	if height <= 0 {
		height = 24
	}
	lines := make([]string, height)
	lines[0] = fit(model.header(theme), width)
	if height < 4 {
		return strings.Join(lines, "\n")
	}

	navigatorWidth := 0
	contentX := 0
	if model.layout == LayoutWide {
		navigatorWidth = workspaceNavigatorWidth(width)
		contentX = navigatorWidth + 1
	}
	contentWidth := max(1, width-contentX)
	lines[1] = composeColumns(model.navigatorTitle(theme, navigatorWidth), model.tabStrip(theme, contentWidth), navigatorWidth, contentWidth, model.layout == LayoutWide)

	bodyHeight := max(0, height-4)
	navigatorLines := model.navigatorLines(navigatorWidth, bodyHeight)
	contentLines := model.contentLines(contentWidth, bodyHeight, theme, content)
	for row := 0; row < bodyHeight; row++ {
		left := ""
		right := contentLines[row]
		if model.layout == LayoutWide {
			left = navigatorLines[row]
		} else if model.focus == FocusNavigator {
			right = navigatorLinesForWidth(model, contentWidth, bodyHeight)[row]
		}
		lines[row+2] = composeColumns(left, right, navigatorWidth, contentWidth, model.layout == LayoutWide)
	}
	if height >= 2 {
		lines[height-2] = fit(model.hints(content), width)
		lines[height-1] = fit(model.statusLine(theme, content), width)
	}
	return strings.Join(lines, "\n")
}

func (model Model) modalView(theme ui.Theme) string {
	width := model.width
	if width <= 0 {
		width = 100
	}
	height := model.height
	if height <= 0 {
		height = 24
	}
	lines := make([]string, height)
	title := "Leave workspace?"
	body := "Dirty or running tabs will be discarded and active operations cancelled."
	destructive := model.modalDestructiveLabel()
	if model.modal.Kind == modalCloseTab {
		title = "Close tab?"
		for _, tab := range model.tabs {
			if tab.Envelope.ID == model.modal.Tab {
				body = tab.Envelope.Title + " has local changes or running work."
				if tab.Envelope.Kind == TabSQL {
					switch {
					case tab.Envelope.Lifecycle == TabRunning && tab.Envelope.Dirty:
						body = tab.Envelope.Title + " has SQL and a running query."
					case tab.Envelope.Lifecycle == TabRunning:
						body = tab.Envelope.Title + " has a running query."
					case tab.Envelope.Dirty:
						body = tab.Envelope.Title + " contains session-local SQL."
					}
				}
				break
			}
		}
	} else if model.modal.Kind == modalQuit {
		title = "Quit db-tui?"
	}
	top := max(0, height/2-3)
	if top < height {
		lines[top] = centered(theme.Title.Render(title), width)
	}
	if top+2 < height {
		lines[top+2] = centered(fit(body, min(width, 72)), width)
	}
	safeLabel := "[Keep open]"
	destructiveLabel := " " + destructive + " "
	if model.modal.Destructive {
		safeLabel = " Keep open "
		destructiveLabel = "[" + destructive + "]"
	}
	if top+4 < height {
		lines[top+4] = centered(safeLabel+"  "+destructiveLabel, width)
	}
	if top+6 < height {
		lines[top+6] = centered(theme.Muted.Render("Tab choose · Enter confirm · Esc keep open"), width)
	}
	for index := range lines {
		lines[index] = fit(lines[index], width)
	}
	return strings.Join(lines, "\n")
}

func (model Model) modalDestructiveLabel() string {
	switch model.modal.Kind {
	case modalQuit:
		return "Discard and quit"
	case modalDisconnect:
		return "Discard and leave"
	case modalCloseTab:
		for _, tab := range model.tabs {
			if tab.Envelope.ID != model.modal.Tab {
				continue
			}
			if tab.Envelope.Kind == TabSQL {
				switch {
				case tab.Envelope.Lifecycle == TabRunning && tab.Envelope.Dirty:
					return "Cancel run and discard SQL"
				case tab.Envelope.Lifecycle == TabRunning:
					return "Cancel run and close"
				case tab.Envelope.Dirty:
					return "Discard SQL"
				}
			}
			break
		}
		return "Discard and close"
	default:
		return "Discard"
	}
}

func (model Model) header(theme ui.Theme) string {
	state := string(model.connection)
	if model.layout == LayoutSingle {
		return theme.Title.Render("db-tui") + " · " + model.profileName + "/" + model.database +
			" · PostgreSQL " + model.serverVersion + " · " + state
	}
	return theme.Title.Render("db-tui") + " — " + model.profileName + " / " + model.database +
		" · " + model.server + " · PostgreSQL " + model.serverVersion + " · " + state
}

func (model Model) navigatorTitle(theme ui.Theme, width int) string {
	if model.layout != LayoutWide {
		return ""
	}
	label := "OBJECTS"
	if model.focus == FocusNavigator {
		label = "[OBJECTS]"
	}
	return theme.Title.Render(fit(label, width))
}

func (model Model) tabStrip(theme ui.Theme, width int) string {
	_ = theme
	indices := model.visibleTabIndices(width)
	parts := make([]string, 0, len(indices)+1)
	for _, index := range indices {
		parts = append(parts, model.tabLabel(model.tabs[index]))
	}
	parts = append(parts, "+ SQL")
	strip := strings.Join(parts, " ")
	if len(indices) > 0 && indices[0] > 0 {
		strip = "‹ " + strip
	}
	if len(indices) > 0 && indices[len(indices)-1] < len(model.tabs)-1 {
		strip += " ›"
	}
	if model.focus == FocusTabs {
		strip = "> " + strip
	}
	return fit(strip, width)
}

func (model Model) tabLabel(tab Tab) string {
	marker := tabMarker(tab)
	if tab.Envelope.ID == model.activeTab {
		return "[" + tab.Envelope.Title + marker + "×]"
	}
	return " " + tab.Envelope.Title + marker + "× "
}

func tabMarker(tab Tab) string {
	marker := ""
	if tab.Envelope.Dirty {
		if tab.Envelope.Kind == TabTable {
			marker = "•"
		} else {
			marker = "*"
		}
	}
	if tab.Envelope.Lifecycle == TabRunning {
		marker += "…"
	} else if tab.Envelope.Lifecycle == TabFailed {
		marker += "!"
	}
	return marker
}

func (model Model) visibleTabIndices(width int) []int {
	if len(model.tabs) == 0 || width <= 0 {
		return nil
	}
	budget := max(4, width-9) // reserve the focus marker, overflow marker, and + SQL
	allWidth := 0
	for index, tab := range model.tabs {
		if index > 0 {
			allWidth++
		}
		allWidth += ansi.StringWidth(model.tabLabel(tab))
	}
	if allWidth <= budget {
		indices := make([]int, len(model.tabs))
		for index := range indices {
			indices[index] = index
		}
		return indices
	}
	active := model.activeTabIndex()
	if active < 0 {
		active = 0
	}
	start := active
	end := active + 1
	used := ansi.StringWidth(model.tabLabel(model.tabs[active]))
	for start > 0 {
		candidate := ansi.StringWidth(model.tabLabel(model.tabs[start-1])) + 1
		if used+candidate > budget {
			break
		}
		start--
		used += candidate
	}
	for end < len(model.tabs) {
		candidate := ansi.StringWidth(model.tabLabel(model.tabs[end])) + 1
		if used+candidate > budget {
			break
		}
		end++
		used += candidate
	}
	indices := make([]int, 0, end-start)
	for index := start; index < end; index++ {
		indices = append(indices, index)
	}
	return indices
}

func (model Model) navigatorLines(width, height int) []string {
	if width <= 0 {
		return make([]string, height)
	}
	return navigatorLinesForWidth(model, width, height)
}

func navigatorLinesForWidth(model Model, width, height int) []string {
	lines := make([]string, height)
	items := model.visibleItems()
	for row := 0; row < height && model.treeOffset+row < len(items); row++ {
		index := model.treeOffset + row
		item := items[index]
		prefix := "  "
		if index == model.selectedTree {
			prefix = "> "
		}
		if item.Kind == treeSchemaItem {
			schema := model.schemas[item.Schema]
			disclosure := "▸"
			if schema.Expanded {
				disclosure = "▾"
			}
			suffix := ""
			if schema.Loading {
				suffix = " …"
			}
			lines[row] = fit(prefix+disclosure+" "+schema.Name+suffix, width)
			continue
		}
		relation := model.schemas[item.Schema].Relations[item.Relation]
		kind := relationGlyph(relation.Kind)
		lock := ""
		if !relation.CanSelect {
			lock = " (no SELECT)"
		}
		lines[row] = fit(prefix+"  "+kind+" "+relation.Name+lock, width)
	}
	if len(items) == 0 && height > 0 {
		if model.schemasRequest != 0 {
			lines[0] = fit("  Loading schemas…", width)
		} else {
			lines[0] = fit("  No visible schemas", width)
		}
	}
	return lines
}

func (model Model) contentLines(width, height int, theme ui.Theme, content ContentView) []string {
	lines := make([]string, height)
	if height == 0 {
		return lines
	}
	if model.connection == ConnectionLost || model.connection == ConnectionReconnecting {
		lines[0] = fit(theme.Title.Render("Connection unavailable"), width)
		if height > 1 {
			lines[1] = fit("Open tabs are preserved. Reconnect will not replay edits or SQL.", width)
		}
		if height > 2 {
			lines[2] = fit("r / Enter reconnect · d / Esc disconnect", width)
		}
		model.appendError(lines, width, 4)
		return lines
	}
	index := model.activeTabIndex()
	if index < 0 {
		lines[0] = fit(theme.Title.Render("Connected workspace"), width)
		if height > 1 {
			lines[1] = fit("Open a relation from Objects, or focus the tab strip and press n for SQL.", width)
		}
		model.appendError(lines, width, 3)
		return lines
	}
	tab := model.tabs[index]
	focus := ""
	if model.focus == FocusContent {
		focus = "> "
	}
	if content.Lines != nil {
		for index := 0; index < height && index < len(content.Lines); index++ {
			lines[index] = fit(content.Lines[index], width)
		}
		return lines
	}
	if tab.Table != nil {
		relation := tab.Table.Relation
		lines[0] = fit(focus+theme.Title.Render(relation.Schema+"."+relation.Name), width)
		if height > 1 {
			lines[1] = fit("Table tab ready. Paginated rows and staged edits arrive in the table-browsing slice.", width)
		}
		if !relation.CanSelect && height > 2 {
			lines[2] = fit("PostgreSQL reports that this role does not have SELECT privilege.", width)
		}
	} else {
		lines[0] = fit(focus+theme.Title.Render(tab.Envelope.Title), width)
		if height > 1 {
			lines[1] = fit("SQL tab ready. Editing, execution, results, and cancellation arrive in the SQL slice.", width)
		}
	}
	model.appendError(lines, width, 4)
	return lines
}

func (model Model) appendError(lines []string, width, start int) {
	if model.err == nil || start >= len(lines) {
		return
	}
	coreError := asCoreError(model.err)
	if coreError == nil {
		lines[start] = fit("Error: "+model.err.Error(), width)
		return
	}
	lines[start] = fit("Error: "+coreError.Summary, width)
	if coreError.PostgreSQL == nil {
		return
	}
	details := coreError.PostgreSQL
	next := start + 1
	if details.SQLState != "" && next < len(lines) {
		lines[next] = fit("SQLSTATE "+details.SQLState, width)
		next++
	}
	for _, detail := range []string{details.Detail, details.Hint, details.Context} {
		if detail != "" && next < len(lines) {
			lines[next] = fit(detail, width)
			next++
		}
	}
}

func (model Model) hints(content ContentView) string {
	if model.connection != ConnectionConnected {
		return "r reconnect · d disconnect · ? help · q quit"
	}
	if model.focus == FocusContent && content.Hints != "" {
		return content.Hints
	}
	if model.layout == LayoutSingle {
		switch model.focus {
		case FocusNavigator:
			return "Enter open · ? more"
		case FocusTabs:
			return "n SQL · ? more"
		default:
			return "Esc tabs · ? more"
		}
	}
	switch model.focus {
	case FocusNavigator:
		return "↑/↓ select · Space expand · Enter open · r refresh · Tab next"
	case FocusTabs:
		return "←/→ select · n new SQL · x close · Enter content · Tab next"
	default:
		return "[ / ] switch tab · Esc tab strip · Tab next"
	}
}

func (model Model) statusLine(theme ui.Theme, content ContentView) string {
	status := model.status
	if model.focus == FocusContent && content.Status != "" {
		status = content.Status
	}
	if status == "" {
		status = "Ready"
	}
	if model.warning != nil {
		status += " · " + model.warning.Summary
	}
	suffix := "? help · Ctrl+D disconnect · q quit"
	if content.ConsumesGlobalKeys && model.focus == FocusContent {
		suffix = "Esc navigation mode for workspace shortcuts"
	}
	return theme.Muted.Render(status + "                                      " + suffix)
}

func (model Model) helpView(theme ui.Theme) string {
	lines := []string{
		theme.Title.Render("db-tui workspace help"), "",
		"Tab / Shift+Tab     move between visible panes",
		"Arrows or h/j/k/l   move in the focused pane",
		"Enter               open or activate",
		"Space               expand/collapse a schema",
		"[ / ]               previous/next workspace tab",
		"n / x               new SQL / close tab (tab strip)",
		"SQL: F5 / F6 / F7   execute / run all / rerun",
		"SQL: Shift+arrows   select text; Esc leaves editor mode",
		"SQL results         n/p page; {/} output; Enter inspect; c copy",
		"r                   refresh objects; reconnect when offline",
		"Ctrl+C              cancel active work; quit when idle",
		"Ctrl+D              disconnect",
		"q                   quit",
		"",
		theme.Muted.Render("Esc, Enter, or ? closes help. Mouse actions mirror these commands."),
	}
	result := make([]string, max(1, model.height))
	for index := range result {
		if index < len(lines) {
			result[index] = fit(lines[index], max(1, model.width))
		}
	}
	return strings.Join(result, "\n")
}

func relationGlyph(kind RelationKind) string {
	switch kind {
	case RelationView:
		return "v"
	case RelationMaterializedView:
		return "m"
	case RelationForeignTable:
		return "f"
	case RelationPartitionedTable:
		return "p"
	default:
		return "t"
	}
}

func navigatorWidth(width int) int { return workspaceNavigatorWidth(width) }

func workspaceNavigatorWidth(width int) int {
	return min(32, max(22, width/4))
}

func composeColumns(left, right string, leftWidth, rightWidth int, split bool) string {
	if !split {
		return fit(right, rightWidth)
	}
	return fit(left, leftWidth) + "│" + fit(right, rightWidth)
}

func fit(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) > width {
		value = ansi.Truncate(value, width, "…")
	}
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
}

func centered(value string, width int) string {
	padding := max(0, (width-ansi.StringWidth(value))/2)
	return strings.Repeat(" ", padding) + value
}
