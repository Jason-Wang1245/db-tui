// Package ui contains focused, presentation-only primitives shared by feature
// renderers.
package ui

type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}
