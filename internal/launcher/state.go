// Package launcher owns connection-profile launcher state and intents.
package launcher

type Mode string

const (
	ModeProfiles Mode = "profiles"
	ModeEdit     Mode = "edit"
)

type State struct {
	Mode          Mode
	SelectedIndex int
}

func NewState() State {
	return State{Mode: ModeProfiles}
}
