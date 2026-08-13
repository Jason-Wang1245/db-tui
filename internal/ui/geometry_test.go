package ui

import "testing"

func TestRectContainsUsesHalfOpenBounds(t *testing.T) {
	r := Rect{X: 2, Y: 3, Width: 4, Height: 5}
	for _, point := range [][2]int{{2, 3}, {5, 7}} {
		if !r.Contains(point[0], point[1]) {
			t.Fatalf("expected rectangle to contain %v", point)
		}
	}
	for _, point := range [][2]int{{1, 3}, {6, 3}, {2, 8}} {
		if r.Contains(point[0], point[1]) {
			t.Fatalf("expected rectangle not to contain %v", point)
		}
	}
}
