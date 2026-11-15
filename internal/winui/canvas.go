package winui

import "math"

// Colour is a colour and how solid it is, written 0xAARRGGBB. AA is the
// opacity: 0xFF hides what is underneath, 0x00 leaves it as it was.
type Colour uint32

// White and Black return white or black at an opacity between 0 and 1.
func White(opacity float64) Colour { return Colour(level(opacity))<<24 | 0xFFFFFF }
func Black(opacity float64) Colour { return Colour(level(opacity)) << 24 }

func level(opacity float64) uint32 {
	return uint32(math.Round(255 * math.Max(0, math.Min(1, opacity))))
}

// Thinned returns the colour at a share of its opacity.
func (c Colour) Thinned(share float64) Colour {
	return Colour(level(float64(c>>24)/255*share))<<24 | c&0xFFFFFF
}

// parts returns the opacity and the three colours, each multiplied by the
// opacity already. That form, called premultiplied, is the one Windows takes
// for a see-through window, and it makes laying one colour over another a
// single sum.
func (c Colour) parts() (a, r, g, b uint32) {
	a = uint32(c >> 24)
	return a, div255(uint32(c>>16&0xFF) * a), div255(uint32(c>>8&0xFF) * a), div255(uint32(c&0xFF) * a)
}

func div255(n uint32) uint32 { return (n + 127) / 255 }

// Canvas is a picture in memory, one 32-bit word a pixel, premultiplied. The
// window is drawn on one and handed to Windows whole.
type Canvas struct {
	Pix            []uint32
	Width, Height  int
	x0, y0, x1, y1 int // drawing is kept inside these bounds
}

// NewCanvas wraps a block of pixels, row after row from the top.
func NewCanvas(pix []uint32, width, height int) *Canvas {
	return &Canvas{Pix: pix, Width: width, Height: height, x1: width, y1: height}
}

// Clip keeps what is drawn next inside a rectangle. It returns false when
// nothing of the rectangle is on the canvas.
func (c *Canvas) Clip(x0, y0, x1, y1 int) bool {
	c.x0, c.y0, c.x1, c.y1 = max(x0, 0), max(y0, 0), min(x1, c.Width), min(y1, c.Height)
	return c.x0 < c.x1 && c.y0 < c.y1
}

// over lays a premultiplied colour on a pixel. cover, from 0 to 255, is how
// much of the pixel the shape being drawn takes up, which is what makes
// curved edges smooth instead of stepped.
func over(under, a, r, g, b, cover uint32) uint32 {
	if cover < 255 {
		a, r, g, b = div255(a*cover), div255(r*cover), div255(g*cover), div255(b*cover)
	}
	if a == 255 {
		return 0xFF000000 | r<<16 | g<<8 | b
	}
	if a == 0 {
		return under
	}
	rest := 255 - a
	return (a+div255((under>>24)*rest))<<24 | (r+div255((under>>16&0xFF)*rest))<<16 |
		(g+div255((under>>8&0xFF)*rest))<<8 | (b + div255((under&0xFF)*rest))
}

// Set replaces every pixel of a rectangle, opacity included. It is how the
// see-through background of the window is laid down.
func (c *Canvas) Set(x, y, width, height int, colour Colour) {
	a, r, g, b := colour.parts()
	word := a<<24 | r<<16 | g<<8 | b
	for py := max(y, c.y0); py < min(y+height, c.y1); py++ {
		row := c.Pix[py*c.Width:]
		for px := max(x, c.x0); px < min(x+width, c.x1); px++ {
			row[px] = word
		}
	}
}

// Fill lays a colour over a rectangle.
func (c *Canvas) Fill(x, y, width, height int, colour Colour) {
	a, r, g, b := colour.parts()
	for py := max(y, c.y0); py < min(y+height, c.y1); py++ {
		row := c.Pix[py*c.Width:]
		for px := max(x, c.x0); px < min(x+width, c.x1); px++ {
			row[px] = over(row[px], a, r, g, b, 255)
		}
	}
}

// outside returns how far a point is outside a rectangle with rounded
// corners. It is negative inside.
func outside(px, py, x, y, width, height, radius float64) float64 {
	dx := math.Abs(px-(x+width/2)) - (width/2 - radius)
	dy := math.Abs(py-(y+height/2)) - (height/2 - radius)
	return math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) + math.Min(math.Max(dx, dy), 0) - radius
}

// cover turns a distance outside a shape into the share of a pixel the
// shape takes up: all of it half a pixel inside the edge, none of it half a
// pixel outside.
func cover(distance float64) uint32 {
	return uint32(math.Round(255 * math.Max(0, math.Min(1, 0.5-distance))))
}

// Round lays a colour over a rectangle with rounded corners.
func (c *Canvas) Round(x, y, width, height int, radius float64, colour Colour) {
	radius = math.Max(0, math.Min(radius, float64(min(width, height))/2))
	corner := int(math.Ceil(radius))
	a, r, g, b := colour.parts()
	for py := max(y, c.y0); py < min(y+height, c.y1); py++ {
		row := c.Pix[py*c.Width:]
		straight := py >= y+corner && py < y+height-corner
		for px := max(x, c.x0); px < min(x+width, c.x1); px++ {
			share := uint32(255)
			if !straight && (px < x+corner || px >= x+width-corner) {
				share = cover(outside(float64(px)+0.5, float64(py)+0.5, float64(x), float64(y), float64(width), float64(height), radius))
			}
			row[px] = over(row[px], a, r, g, b, share)
		}
	}
}

// Ring draws the outline of a rectangle with rounded corners, a line of the
// given thickness just inside its edge.
func (c *Canvas) Ring(x, y, width, height int, radius, thickness float64, colour Colour) {
	radius = math.Max(0, math.Min(radius, float64(min(width, height))/2))
	band := int(math.Ceil(math.Max(radius, thickness))) + 1
	a, r, g, b := colour.parts()
	for py := max(y, c.y0); py < min(y+height, c.y1); py++ {
		row := c.Pix[py*c.Width:]
		edge := py < y+band || py >= y+height-band
		for px := max(x, c.x0); px < min(x+width, c.x1); px++ {
			if !edge && px >= x+band && px < x+width-band {
				px = x + width - band - 1 // the middle of the row is empty
				continue
			}
			d := outside(float64(px)+0.5, float64(py)+0.5, float64(x), float64(y), float64(width), float64(height), radius)
			if share := cover(d) - cover(d+thickness); share > 0 {
				row[px] = over(row[px], a, r, g, b, share)
			}
		}
	}
}

// Disc lays a colour over a circle.
func (c *Canvas) Disc(cx, cy, radius float64, colour Colour) {
	a, r, g, b := colour.parts()
	for py := max(int(cy-radius-1), c.y0); py < min(int(cy+radius+2), c.y1); py++ {
		row := c.Pix[py*c.Width:]
		for px := max(int(cx-radius-1), c.x0); px < min(int(cx+radius+2), c.x1); px++ {
			if share := cover(math.Hypot(float64(px)+0.5-cx, float64(py)+0.5-cy) - radius); share > 0 {
				row[px] = over(row[px], a, r, g, b, share)
			}
		}
	}
}

// Line draws a straight line with rounded ends.
func (c *Canvas) Line(x0, y0, x1, y1, thickness float64, colour Colour) {
	half := thickness / 2
	a, r, g, b := colour.parts()
	dx, dy := x1-x0, y1-y0
	length := dx*dx + dy*dy
	left, right := int(math.Min(x0, x1)-half-1), int(math.Max(x0, x1)+half+2)
	top, bottom := int(math.Min(y0, y1)-half-1), int(math.Max(y0, y1)+half+2)
	for py := max(top, c.y0); py < min(bottom, c.y1); py++ {
		row := c.Pix[py*c.Width:]
		for px := max(left, c.x0); px < min(right, c.x1); px++ {
			// The nearest point of the line to the middle of this pixel.
			fx, fy := float64(px)+0.5-x0, float64(py)+0.5-y0
			along := 0.0
			if length > 0 {
				along = math.Max(0, math.Min(1, (fx*dx+fy*dy)/length))
			}
			if share := cover(math.Hypot(fx-along*dx, fy-along*dy) - half); share > 0 {
				row[px] = over(row[px], a, r, g, b, share)
			}
		}
	}
}

// Stencil colours in a shape that was drawn elsewhere in white on black, or
// in black on white when dark is set: text, which Windows draws. Each pixel
// of the stencil says how much of that pixel the shape covers. stride is
// the number of pixels in one row of the stencil.
func (c *Canvas) Stencil(x, y int, stencil []uint32, stride, width, height int, colour Colour, dark bool) {
	a, r, g, b := colour.parts()
	for py := max(y, c.y0); py < min(y+height, c.y1); py++ {
		row := c.Pix[py*c.Width:]
		from := stencil[(py-y)*stride:]
		for px := max(x, c.x0); px < min(x+width, c.x1); px++ {
			p := from[px-x]
			share := (p>>16&0xFF + p>>8&0xFF + p&0xFF + 1) / 3
			if dark {
				share = 255 - share
			}
			if share > 0 {
				row[px] = over(row[px], a, r, g, b, share)
			}
		}
	}
}

// Shown returns what a pixel of the canvas looks like on the screen when
// the given colour is behind the window: red, green and blue from 0 to 255.
func Shown(pixel uint32, behind Colour) (r, g, b uint32) {
	rest := 255 - pixel>>24
	return pixel>>16&0xFF + div255(uint32(behind>>16&0xFF)*rest),
		pixel>>8&0xFF + div255(uint32(behind>>8&0xFF)*rest),
		pixel&0xFF + div255(uint32(behind&0xFF)*rest)
}

// Contrast returns how far apart two colours are in brightness, by the
// measure the accessibility guidelines for the web use: 1 is no difference,
// 21 is black on white, and 4.5 is the least they accept for body text.
func Contrast(r1, g1, b1, r2, g2, b2 uint32) float64 {
	light := func(r, g, b uint32) float64 {
		part := func(v uint32) float64 {
			f := float64(v) / 255
			if f <= 0.04045 {
				return f / 12.92
			}
			return math.Pow((f+0.055)/1.055, 2.4)
		}
		return 0.2126*part(r) + 0.7152*part(g) + 0.0722*part(b)
	}
	high, low := light(r1, g1, b1), light(r2, g2, b2)
	if high < low {
		high, low = low, high
	}
	return (high + 0.05) / (low + 0.05)
}
