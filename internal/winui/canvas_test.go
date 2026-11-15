package winui

import "testing"

func blank(width, height int) *Canvas {
	return NewCanvas(make([]uint32, width*height), width, height)
}

func (c *Canvas) at(x, y int) uint32 { return c.Pix[y*c.Width+x] }

func TestSetReplacesAndPremultiplies(t *testing.T) {
	c := blank(4, 4)
	c.Set(0, 0, 4, 4, 0x80FFFFFF)
	// Half-solid white is stored with each colour halved as well.
	if got := c.at(2, 2); got != 0x80808080 {
		t.Fatalf("got %08X", got)
	}
}

func TestOverLeavesAValidPixel(t *testing.T) {
	// However colours are laid over one another, no colour of a pixel may
	// exceed its opacity, or Windows would show it brighter than white.
	colours := []Colour{0x00000000, 0x33FF0000, 0x8000FF00, 0xCC0B0B0B, 0xFFF3F3F3, 0x0DFFFFFF, 0xFF000000}
	for _, under := range colours {
		for _, top := range colours {
			for _, share := range []uint32{0, 1, 77, 128, 254, 255} {
				c := blank(1, 1)
				c.Set(0, 0, 1, 1, under)
				a, r, g, b := top.parts()
				p := over(c.at(0, 0), a, r, g, b, share)
				alpha := p >> 24
				if p>>16&0xFF > alpha || p>>8&0xFF > alpha || p&0xFF > alpha {
					t.Errorf("%08X over %08X at %d gave %08X", top, under, share, p)
				}
			}
		}
	}
	// A solid colour hides what is under it; a clear one changes nothing.
	if p := over(0x80808080, 255, 10, 20, 30, 255); p != 0xFF0A141E {
		t.Errorf("solid over gave %08X", p)
	}
	if p := over(0x80808080, 0, 0, 0, 0, 255); p != 0x80808080 {
		t.Errorf("clear over gave %08X", p)
	}
}

func TestRoundFillsTheMiddleAndSparesTheCorners(t *testing.T) {
	c := blank(40, 20)
	c.Round(0, 0, 40, 20, 6, 0xFFFFFFFF)
	if c.at(20, 10) != 0xFFFFFFFF || c.at(0, 10) != 0xFFFFFFFF || c.at(20, 0) != 0xFFFFFFFF {
		t.Fatal("the middle and the straight edges should be solid")
	}
	for _, corner := range [][2]int{{0, 0}, {39, 0}, {0, 19}, {39, 19}} {
		if p := c.at(corner[0], corner[1]); p != 0 {
			t.Errorf("corner %v was drawn on: %08X", corner, p)
		}
	}
	// Somewhere along each curve a pixel is partly covered: that is the smoothing.
	smooth := false
	for x := 0; x < 6; x++ {
		for y := 0; y < 6; y++ {
			if a := c.at(x, y) >> 24; a > 0 && a < 255 {
				smooth = true
			}
		}
	}
	if !smooth {
		t.Error("the curve has no partly covered pixels, so it would look stepped")
	}
	// The four corners are mirror images of one another.
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			p := c.at(x, y)
			if c.at(39-x, y) != p || c.at(x, 19-y) != p || c.at(39-x, 19-y) != p {
				t.Fatalf("corners differ at %d,%d", x, y)
			}
		}
	}
}

func TestRingIsHollow(t *testing.T) {
	c := blank(30, 30)
	c.Ring(0, 0, 30, 30, 6, 2, 0xFFFFFFFF)
	if c.at(15, 15) != 0 {
		t.Error("the middle of a ring should be empty")
	}
	if c.at(15, 0) != 0xFFFFFFFF || c.at(15, 1) != 0xFFFFFFFF || c.at(0, 15) != 0xFFFFFFFF || c.at(29, 15) != 0xFFFFFFFF {
		t.Error("the line of the ring should be solid")
	}
	if c.at(15, 3) != 0 {
		t.Errorf("the ring is thicker than asked: %08X", c.at(15, 3))
	}
}

func TestClipKeepsDrawingInside(t *testing.T) {
	c := blank(20, 20)
	if !c.Clip(5, 5, 10, 10) {
		t.Fatal("the clip is on the canvas")
	}
	c.Set(0, 0, 20, 20, 0xFF112233)
	c.Fill(0, 0, 20, 20, 0xFFFFFFFF)
	c.Round(0, 0, 20, 20, 4, 0xFFFFFFFF)
	c.Ring(0, 0, 20, 20, 4, 2, 0xFFFFFFFF)
	c.Disc(10, 10, 9, 0xFFFFFFFF)
	c.Line(0, 0, 20, 20, 3, 0xFFFFFFFF)
	c.Stencil(0, 0, make([]uint32, 400), 20, 20, 20, 0xFFFFFFFF, true)
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			inside := x >= 5 && x < 10 && y >= 5 && y < 10
			if p := c.at(x, y); (p != 0) != inside {
				t.Fatalf("pixel %d,%d is %08X", x, y, p)
			}
		}
	}
	if c.Clip(30, 30, 40, 40) {
		t.Error("a clip off the canvas should report that nothing can be drawn")
	}
}

func TestDiscAndLine(t *testing.T) {
	c := blank(21, 21)
	c.Disc(10.5, 10.5, 5, 0xFFFFFFFF)
	if c.at(10, 10) != 0xFFFFFFFF || c.at(10, 6) != 0xFFFFFFFF {
		t.Error("the inside of the disc should be solid")
	}
	if c.at(10, 4) != 0 || c.at(2, 2) != 0 {
		t.Error("outside the disc should be untouched")
	}
	c = blank(21, 21)
	c.Line(4, 10.5, 16, 10.5, 3, 0xFFFFFFFF)
	if c.at(10, 10) != 0xFFFFFFFF {
		t.Error("the middle of the line should be solid")
	}
	if c.at(10, 7) != 0 || c.at(19, 10) != 0 {
		t.Error("away from the line should be untouched")
	}
	if a := c.at(17, 10) >> 24; a == 0 {
		t.Error("the end of the line should be rounded, so it reaches a little past its end point")
	}
}

func TestStencilColoursInTheShape(t *testing.T) {
	// Light text arrives white on black; dark text black on white.
	stencil := []uint32{0x000000, 0xFFFFFF, 0x808080, 0x000000}
	c := blank(4, 1)
	c.Stencil(0, 0, stencil, 4, 4, 1, 0xFFFF0000, false)
	if c.at(0, 0) != 0 || c.at(1, 0) != 0xFFFF0000 || c.at(3, 0) != 0 {
		t.Errorf("light stencil gave %08X %08X %08X", c.at(0, 0), c.at(1, 0), c.at(3, 0))
	}
	if a := c.at(2, 0) >> 24; a < 120 || a > 136 {
		t.Errorf("half cover gave opacity %d", a)
	}
	c = blank(4, 1)
	c.Stencil(0, 0, stencil, 4, 4, 1, 0xFF000000, true)
	if c.at(0, 0) != 0xFF000000 || c.at(1, 0) != 0 {
		t.Errorf("dark stencil gave %08X %08X", c.at(0, 0), c.at(1, 0))
	}
}

func TestContrast(t *testing.T) {
	if got := Contrast(255, 255, 255, 0, 0, 0); got < 20.9 || got > 21.1 {
		t.Errorf("white on black is %.2f, want 21", got)
	}
	if got := Contrast(90, 90, 90, 90, 90, 90); got != 1 {
		t.Errorf("a colour on itself is %.2f, want 1", got)
	}
	if Contrast(0, 0, 0, 255, 255, 255) != Contrast(255, 255, 255, 0, 0, 0) {
		t.Error("the order of the two colours should not matter")
	}
}

func TestThinnedAndShades(t *testing.T) {
	if got := Colour(0x80112233).Thinned(0.5); got != 0x40112233 {
		t.Errorf("got %08X", got)
	}
	if White(1) != 0xFFFFFFFF || Black(0) != 0 || White(0.5)>>24 != 128 {
		t.Errorf("shades are wrong: %08X %08X %08X", White(1), Black(0), White(0.5))
	}
}
