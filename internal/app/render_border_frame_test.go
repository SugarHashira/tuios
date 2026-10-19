package app

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/Gaurav-Gosain/tuios/internal/config"
)

// appearance.shared_borders_frame closes the shared dividers with a border round
// the whole pane area. The frame is one cell of chrome on every side, reserved
// like the dock so the panes are laid out inside it, and it is drawn from the
// same grid as the dividers so a divider that runs into it meets it.

var sgrSequence = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// frameOS is sharedBorderOS with the frame option on while the layout is built,
// which is when the panes take their place inside it.
func frameOS(t *testing.T, n int, frame bool) *OS {
	t.Helper()
	original := config.Global.SharedBordersFrame
	config.Global.SharedBordersFrame = frame
	t.Cleanup(func() { config.Global.SharedBordersFrame = original })
	return sharedBorderOS(t, n)
}

// overlayCells reads the divider overlay back as the glyph at each cell, with
// the cells that carry the focus styling reported separately.
func overlayCells(layers []*lipgloss.Layer) (glyphs map[[2]int]rune, focused map[[2]int]bool) {
	glyphs, focused = map[[2]int]rune{}, map[[2]int]bool{}
	for _, l := range layers {
		content := l.GetContent()
		bold := false
		x := l.GetX()
		for len(content) > 0 {
			if loc := sgrSequence.FindStringIndex(content); loc != nil && loc[0] == 0 {
				seq := content[:loc[1]]
				switch {
				case strings.HasPrefix(seq, "\x1b[1m"):
					bold = true
				case seq == "\x1b[0m":
					bold = false
				}
				content = content[loc[1]:]
				continue
			}
			r := []rune(content)[0]
			glyphs[[2]int{x, l.GetY()}] = r
			focused[[2]int{x, l.GetY()}] = bold
			content = content[len(string(r)):]
			x++
		}
	}
	return glyphs, focused
}

func TestSharedBorderFrameReservesOneCellOnEverySide(t *testing.T) {
	plain := frameOS(t, 2, false)
	framed := frameOS(t, 2, true)

	if got, want := framed.PaneLeft(), plain.PaneLeft()+1; got != want {
		t.Errorf("PaneLeft = %d, want %d", got, want)
	}
	if got, want := framed.PaneTop(), plain.PaneTop()+1; got != want {
		t.Errorf("PaneTop = %d, want %d", got, want)
	}
	if got, want := framed.PaneWidth(), plain.PaneWidth()-2; got != want {
		t.Errorf("PaneWidth = %d, want %d", got, want)
	}
	if got, want := framed.PaneHeight(), plain.PaneHeight()-2; got != want {
		t.Errorf("PaneHeight = %d, want %d", got, want)
	}
	for i, w := range framed.Windows {
		if w.X < framed.PaneLeft() || w.Y < framed.PaneTop() ||
			w.X+w.Width > framed.PaneLeft()+framed.PaneWidth() ||
			w.Y+w.Height > framed.PaneTop()+framed.PaneHeight() {
			t.Errorf("window %d at %d,%d %dx%d spills out of the framed pane area", i, w.X, w.Y, w.Width, w.Height)
		}
	}
}

func TestSharedBorderFrameNeedsSharedBorders(t *testing.T) {
	m := frameOS(t, 2, true)
	m.SharedBorders = false
	if m.sharedFrame() {
		t.Error("the frame closes shared dividers, so it has nothing to do without shared borders")
	}
	if r := m.OwnLayoutReserve(); r.Left != 0 || r.Right != 0 {
		t.Errorf("reserve without shared borders = %+v, want no frame cells", r)
	}
}

func TestSharedBorderFrameIsDrawnRoundThePaneArea(t *testing.T) {
	m := frameOS(t, 2, true)
	glyphs, _ := overlayCells(m.renderSeparatorOverlay())

	b := m.GetBSPBounds()
	x0, x1 := b.X-1, b.X+b.W
	y0, y1 := b.Y-1, b.Y+b.H

	corners := map[[2]int]rune{
		{x0, y0}: '╭', {x1, y0}: '╮', {x0, y1}: '╰', {x1, y1}: '╯',
	}
	for at, want := range corners {
		if got := glyphs[at]; got != want {
			t.Errorf("corner %v = %q, want %q", at, got, want)
		}
	}
	for y := b.Y; y < b.Y+b.H; y++ {
		if glyphs[[2]int{x0, y}] == 0 || glyphs[[2]int{x1, y}] == 0 {
			t.Fatalf("row %d has a gap in the left or right edge of the frame", y)
		}
	}
	for x := b.X; x < b.X+b.W; x++ {
		if glyphs[[2]int{x, y0}] == 0 || glyphs[[2]int{x, y1}] == 0 {
			t.Fatalf("column %d has a gap in the top or bottom edge of the frame", x)
		}
	}

	// The divider between the panes runs into the frame instead of stopping a
	// cell short of it. Where the focused pane's ring turns on that cell it bends
	// into the pane, as it does for any shared divider, so the focus is put on a
	// pane whose ring does not reach the top edge to read the plain tee, and on
	// the pane the divider belongs to to read the bend.
	div := m.Windows[0].X + m.Windows[0].Width
	three := frameOS(t, 3, true)
	three.FocusedWindow = 2
	tg, _ := overlayCells(three.renderSeparatorOverlay())
	tb := three.GetBSPBounds()
	tdiv := three.Windows[0].X + three.Windows[0].Width
	if got := tg[[2]int{tdiv, tb.Y - 1}]; got != '┬' {
		t.Errorf("divider meets the top edge as %q, want a tee ┬", got)
	}
	m.FocusedWindow = 0
	glyphs, _ = overlayCells(m.renderSeparatorOverlay())
	if got := glyphs[[2]int{div, y1}]; got != '╯' {
		t.Errorf("focused ring meets the bottom edge as %q, want the corner ╯ bending into the pane", got)
	}
	m.FocusedWindow = 1
	glyphs, _ = overlayCells(m.renderSeparatorOverlay())
	if got := glyphs[[2]int{div, y0}]; got != '╭' {
		t.Errorf("focused ring meets the top edge as %q, want the corner ╭ bending into the pane", got)
	}
}

func TestSharedBorderFrameOfALonePaneIsStillDrawn(t *testing.T) {
	m := frameOS(t, 1, true)
	if layers := m.renderSeparatorOverlay(); len(layers) == 0 {
		t.Fatal("a single framed pane drew no frame: the frame only appeared once there was a divider")
	}
	plain := frameOS(t, 1, false)
	if layers := plain.renderSeparatorOverlay(); len(layers) != 0 {
		t.Errorf("a single pane without the frame must stay undrawn, got %d layers", len(layers))
	}
}

func TestSharedBorderFrameFollowsTheFocusedPane(t *testing.T) {
	m := frameOS(t, 2, true)
	b := m.GetBSPBounds()
	x0, x1 := b.X-1, b.X+b.W
	mid := b.Y + b.H/2

	m.FocusedWindow = 0
	_, focused := overlayCells(m.renderSeparatorOverlay())
	if !focused[[2]int{x0, mid}] {
		t.Error("focusing the left pane did not light the left edge of the frame")
	}
	if focused[[2]int{x1, mid}] {
		t.Error("focusing the left pane lit the right edge of the frame")
	}

	m.FocusedWindow = 1
	_, focused = overlayCells(m.renderSeparatorOverlay())
	if focused[[2]int{x0, mid}] {
		t.Error("focusing the right pane lit the left edge of the frame")
	}
	if !focused[[2]int{x1, mid}] {
		t.Error("focusing the right pane did not light the right edge of the frame")
	}
}

func TestSharedBorderFrameZoomCoversTheFrame(t *testing.T) {
	m := frameOS(t, 2, true)
	x, y, w, h := m.zoomRect()
	if x != m.PaneLeft()-1 || y != m.PaneTop()-1 ||
		w != m.PaneWidth()+2 || h != m.PaneHeight()+2 {
		t.Errorf("zoom box = %d,%d %dx%d, want the pane area plus the frame ring", x, y, w, h)
	}

	plain := frameOS(t, 2, false)
	x, y, w, h = plain.zoomRect()
	if x != plain.PaneLeft() || y != plain.PaneTop() ||
		w != plain.PaneWidth() || h != plain.PaneHeight() {
		t.Errorf("zoom box without the frame = %d,%d %dx%d, want exactly the pane area", x, y, w, h)
	}
}
