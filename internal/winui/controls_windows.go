//go:build windows

package winui

import "math"

// The kinds of control.
const (
	kindText     = iota // words
	kindPanel           // a card behind a group of controls
	kindRule            // a dividing line
	kindStrip           // the track the section buttons sit in
	kindNote            // a box of text on several lines
	kindSection         // a button that shows a page
	kindButton          //
	kindSwitch          // on or off
	kindChoice          // a list that drops down
	kindShortcut        // a box that takes a key combination
	kindTable           // rows under column titles
	kindPath            // a box that shows a path
	kindBar             // a level
	kindLight           // a small round lamp
	kindMinimise        // the window's own buttons
	kindClose           //
)

// Tone is what a light shows.
type Tone int

const (
	Off     Tone = iota // a ring: nothing is happening
	Live                // red: recording
	Waiting             // amber: starting, or held up
)

// Control is one thing in the window: a line of text, a button, a list.
type Control struct {
	w                   *Window
	kind                int
	page                int // the page it is on, or -1 for every page
	x, y, width, height int // in the units of the layout, before scaling
	text                string
	font                uintptr
	colour              Colour
	flags               uint32 // how its text is placed
	action              func()
	disabled            bool
	lead                bool     // a button drawn as the thing to do next
	on                  bool     // a switch
	entries             []string // a list
	chosen              int      //
	key, modifiers      uint32   // a shortcut box
	level               float64  // a bar, from 0 to 1
	tone                Tone     // a light
	target              int      // the page a section button shows
	empty               string   // what a path box says while it has no path
	table               *table   // a table
}

func (w *Window) add(kind, x, y, width, height int, text string, action func()) *Control {
	c := &Control{w: w, kind: kind, page: w.page, x: x, y: y, width: width, height: height, text: text,
		font: w.fonts.body, colour: theme.Ink, flags: textMiddle | textEllipsis, action: action}
	w.controls = append(w.controls, c)
	return c
}

// Title is the name of the program, in large bold letters.
func (w *Window) Title(text string, x, y, width, height int) *Control {
	c := w.add(kindText, x, y, width, height, text, nil)
	c.font = w.fonts.title
	return c
}

// Heading starts a page.
func (w *Window) Heading(text string, x, y, width, height int) *Control {
	c := w.add(kindText, x, y, width, height, text, nil)
	c.font = w.fonts.heading
	return c
}

// Label is text that takes as many lines as it needs.
func (w *Window) Label(text string, x, y, width, height int) *Control {
	c := w.add(kindText, x, y, width, height, text, nil)
	c.colour, c.flags = theme.Soft, textWrap
	return c
}

// Caption names the control beside it.
func (w *Window) Caption(text string, x, y, width, height int) *Control {
	c := w.add(kindText, x, y, width, height, text, nil)
	c.colour = theme.Soft
	return c
}

// Line is one line of text. Text that is too long for it is cut short with
// an ellipsis at the end.
func (w *Window) Line(text string, x, y, width, height int) *Control {
	return w.add(kindText, x, y, width, height, text, nil)
}

// Quiet is a Line in a softer colour, for the tagline and the line at the
// foot of the window.
func (w *Window) Quiet(text string, x, y, width, height int) *Control {
	c := w.add(kindText, x, y, width, height, text, nil)
	c.colour = theme.Quiet
	return c
}

// Note is a Quiet in smaller letters, for what a setting costs.
func (w *Window) Note(text string, x, y, width, height int) *Control {
	c := w.Quiet(text, x, y, width, height)
	c.font = w.fonts.small
	return c
}

// Panel is a card that sets a group of controls apart. Create it before them.
func (w *Window) Panel(x, y, width, height int) *Control {
	return w.add(kindPanel, x, y, width, height, "", nil)
}

// Rule is a dividing line.
func (w *Window) Rule(x, y, width int) *Control { return w.add(kindRule, x, y, width, 1, "", nil) }

// Strip is the track the section buttons sit in. Create it before them.
func (w *Window) Strip(x, y, width, height int) *Control {
	return w.add(kindStrip, x, y, width, height, "", nil)
}

// Section adds a button that shows a page. The button of the page on show
// is drawn raised.
func (w *Window) Section(text string, page, x, y, width, height int) *Control {
	c := w.add(kindSection, x, y, width, height, text, nil)
	c.target, c.flags = page, textMiddle|textCentre
	return c
}

func (w *Window) Button(text string, x, y, width, height int, action func()) *Control {
	c := w.add(kindButton, x, y, width, height, text, action)
	c.flags = textMiddle | textCentre | textEllipsis
	return c
}

// Switch is a setting that is on or off. action runs when it is flipped.
func (w *Window) Switch(text string, x, y, width, height int, action func()) *Control {
	return w.add(kindSwitch, x, y, width, height, text, action)
}

// Choice is a list that drops down. action runs when another entry is chosen.
func (w *Window) Choice(entries []string, x, y, width, height int, action func()) *Control {
	c := w.add(kindChoice, x, y, width, height, "", action)
	c.entries = entries
	return c
}

// Shortcut is a box that shows a key combination and takes a new one when
// keys are pressed in it. action runs when the combination changes.
func (w *Window) Shortcut(x, y, width, height int, action func()) *Control {
	return w.add(kindShortcut, x, y, width, height, "", action)
}

// Path is a box that shows a file or a folder. A path too long for it
// keeps its start and its last part. While it has no path to show it says
// what empty says.
func (w *Window) Path(empty string, x, y, width, height int) *Control {
	c := w.add(kindPath, x, y, width, height, "", nil)
	c.colour, c.flags, c.empty = theme.Soft, textMiddle|textPath, empty
	return c
}

// Note is a box that shows a few lines of text.
func (w *Window) Box(text string, x, y, width, height int) *Control {
	c := w.add(kindNote, x, y, width, height, text, nil)
	c.colour, c.flags = theme.Soft, textWrap
	return c
}

// Bar is a level bar: a thin track filled from the left.
func (w *Window) Bar(x, y, width, height int) *Control {
	return w.add(kindBar, x, y, width, height, "", nil)
}

// Light is a small lamp that shows a Tone.
func (w *Window) Light(x, y, width, height int) *Control {
	return w.add(kindLight, x, y, width, height, "", nil)
}

// Frame adds the window's own buttons, minimise and close, at its top
// right corner.
func (w *Window) Frame() {
	const across, down = 46, 34
	w.add(kindMinimise, w.width-2*across, 0, across, down, "", func() { showWindow.Call(w.Handle, 6) })
	w.add(kindClose, w.width-across, 0, across, down, "", func() { postMessage.Call(w.Handle, wmClose, 0, 0) })
}

// SetText changes what a control shows. A control that already shows the
// text is left alone, so a status refreshed several times a second is
// redrawn only when it changes.
func (c *Control) SetText(text string) {
	if c.text != text {
		c.text = text
		c.redraw()
	}
}

// Do sets what a button does when it is pressed.
func (c *Control) Do(action func()) { c.action = action }

// Text returns what a control shows.
func (c *Control) Text() string { return c.text }

// Enable makes a control usable, or not.
func (c *Control) Enable(enabled bool) {
	if c.disabled == enabled {
		c.disabled = !enabled
		if !enabled && c.w.focus == c {
			c.w.focus = nil
		}
		c.redraw()
	}
}

// Lead draws a button as the thing to do next, or as an ordinary button.
func (c *Control) Lead(lead bool) {
	if c.lead != lead {
		c.lead = lead
		c.redraw()
	}
}

// On reports whether a switch is on.
func (c *Control) On() bool { return c.on }

// Turn sets a switch without running its action.
func (c *Control) Turn(on bool) {
	if c.on != on {
		c.on = on
		c.redraw()
	}
}

// SetEntries replaces the entries of a list.
func (c *Control) SetEntries(entries []string) {
	if c.w.open == c {
		c.w.closeList()
	}
	c.entries = entries
	c.chosen = min(c.chosen, len(entries)-1)
	c.redraw()
}

// Chosen returns the number of the chosen entry of a list, or -1.
func (c *Control) Chosen() int {
	if c.chosen >= len(c.entries) {
		return -1
	}
	return c.chosen
}

// Choose selects an entry of a list without running its action.
func (c *Control) Choose(entry int) {
	if c.chosen != entry {
		c.chosen = entry
		c.redraw()
	}
}

// Keys returns the virtual key code and modifiers shown in a shortcut box.
// key is zero when the box is empty.
func (c *Control) Keys() (key, modifiers uint32) { return c.key, c.modifiers }

// SetKeys shows a key combination in a shortcut box.
func (c *Control) SetKeys(key, modifiers uint32) {
	c.key, c.modifiers = key, modifiers
	c.redraw()
}

// SetLevel moves a bar to a level between 0 and 1. The bar is redrawn only
// when its filled part changes by a whole pixel.
func (c *Control) SetLevel(level float64) {
	before := c.filled()
	c.level = math.Max(0, math.Min(1, level))
	if c.filled() != before {
		c.redraw()
	}
}

// filled returns how many pixels of a bar are filled.
func (c *Control) filled() int {
	_, _, width, height := c.box()
	filled := int(math.Round(float64(width) * c.level))
	if c.level > 0 {
		filled = max(filled, height) // so the first of it shows as a dot
	}
	return filled
}

// Show sets what a light shows.
func (c *Control) Show(tone Tone) {
	if c.tone != tone {
		c.tone = tone
		c.redraw()
	}
}

// box returns where the control is, in pixels.
func (c *Control) box() (x, y, width, height int) {
	x, y = c.w.px(c.x), c.w.px(c.y)
	return x, y, c.w.px(c.x+c.width) - x, c.w.px(c.y+c.height) - y
}

// reach is how far outside its box a control may draw: the ring that shows
// where the keyboard is.
const reach = 5

func (c *Control) redraw() {
	x, y, width, height := c.box()
	margin := c.w.px(reach)
	c.w.redraw(x-margin, y-margin, width+2*margin, height+2*margin)
}

// touches reports whether the control would draw inside a rectangle.
func (c *Control) touches(area rect) bool {
	x, y, width, height := c.box()
	margin := c.w.px(reach)
	return x-margin < int(area.Right) && x+width+margin > int(area.Left) && y-margin < int(area.Bottom) && y+height+margin > int(area.Top)
}

func (c *Control) showing() bool { return c.page == -1 || c.page == c.w.current }

// usable reports whether the pointer and the keyboard can work the control.
func (c *Control) usable() bool {
	return c.kind >= kindSection && c.kind != kindPath && c.kind != kindBar && c.kind != kindLight && c.showing() && !c.disabled
}

// reachable reports whether the Tab key stops at the control. The window's
// own buttons are left out, as they are in every Windows program.
func (c *Control) reachable() bool {
	return c.usable() && c.kind != kindMinimise && c.kind != kindClose
}

// shortcutText writes a key combination out, such as Ctrl + Shift + F9.
func shortcutText(key, modifiers uint32) string {
	if key == 0 {
		return ""
	}
	text := ""
	for _, part := range []struct {
		modifier uint32
		name     string
	}{{Ctrl, "Ctrl"}, {Shift, "Shift"}, {Alt, "Alt"}} {
		if modifiers&part.modifier != 0 {
			text += part.name + " + "
		}
	}
	return text + KeyName(key)
}

// ShortcutName writes a key combination out, such as Ctrl + Shift + F9.
func ShortcutName(key, modifiers uint32) string { return shortcutText(key, modifiers) }

// draw puts the control on the canvas as it is now.
func (c *Control) draw(cv *Canvas) {
	w := c.w
	x, y, width, height := c.box()
	hover := w.hover == c
	down := hover && w.pressed == c
	hair := w.pxf(1)
	radius := w.pxf(9)
	switch c.kind {
	case kindText:
		w.write(cv, c.text, c.font, x, y, width, height, c.flags, c.colour)
	case kindPanel:
		cv.Round(x, y, width, height, w.pxf(12), theme.Panel)
		cv.Ring(x, y, width, height, w.pxf(12), hair, theme.PanelEdge)
	case kindRule:
		cv.Fill(x, y, width, max(int(math.Round(hair)), 1), theme.Rule)
	case kindStrip:
		cv.Round(x, y, width, height, w.pxf(12), theme.Strip)
		cv.Ring(x, y, width, height, w.pxf(12), hair, theme.PanelEdge)
	case kindNote:
		cv.Round(x, y, width, height, radius, theme.Well)
		cv.Ring(x, y, width, height, radius, hair, theme.WellEdge)
		w.write(cv, c.text, c.font, x+w.px(12), y+w.px(8), width-w.px(24), height-w.px(12), c.flags, c.colour)
	case kindTable:
		c.drawTable(cv)
	case kindSection:
		colour, font := theme.Soft, w.fonts.body
		switch {
		case c.disabled && c.target != w.current:
			colour = theme.Faint
		case c.target == w.current:
			cv.Round(x, y, width, height, radius, theme.TabChosen)
			colour, font = theme.Ink, w.fonts.strong
		case hover:
			cv.Round(x, y, width, height, radius, theme.TabHover)
			colour = theme.Ink
		}
		w.write(cv, c.text, font, x, y, width, height, c.flags, colour)
	case kindButton:
		colour := theme.Ink
		switch {
		case c.disabled:
			cv.Round(x, y, width, height, radius, theme.PlainOff)
			cv.Ring(x, y, width, height, radius, hair, theme.PanelEdge)
			colour = theme.Faint
		case c.lead:
			cv.Round(x, y, width, height, radius, pick(down, theme.LeadDown, pick(hover, theme.LeadHover, theme.Lead)))
			colour = theme.OnLight
		default:
			cv.Round(x, y, width, height, radius, pick(down, theme.PlainDown, pick(hover, theme.PlainHover, theme.Plain)))
			cv.Ring(x, y, width, height, radius, hair, theme.PlainEdge)
		}
		w.write(cv, c.text, w.fonts.strong, x, y, width, height, c.flags, colour)
	case kindSwitch:
		// A pill with a knob: at the right and lit when on, at the left when off.
		across, tall := w.px(38), w.px(22)
		top := y + (height-tall)/2
		knob := float64(tall)/2 - w.pxf(4)
		if c.on {
			cv.Round(x, top, across, tall, float64(tall)/2, pick(hover, theme.LeadHover, theme.Lead))
			cv.Disc(float64(x+across)-float64(tall)/2, float64(top)+float64(tall)/2, knob, theme.OnLight)
		} else {
			cv.Round(x, top, across, tall, float64(tall)/2, pick(hover, theme.PlainHover, theme.SwitchOff))
			cv.Ring(x, top, across, tall, float64(tall)/2, hair, theme.SwitchEdge)
			cv.Disc(float64(x)+float64(tall)/2, float64(top)+float64(tall)/2, knob, theme.KnobOff)
		}
		gap := across + w.px(12)
		w.write(cv, c.text, c.font, x+gap, y, width-gap, height, c.flags, pick(c.disabled, theme.Faint, theme.Ink))
	case kindChoice, kindShortcut:
		lit := w.open == c || (c.kind == kindShortcut && w.focus == c && w.active)
		cv.Round(x, y, width, height, radius, pick(hover || lit, theme.FieldHover, theme.Field))
		cv.Ring(x, y, width, height, radius, hair, pick(lit, theme.FieldLit, theme.FieldEdge))
		inset := w.px(12)
		if c.kind == kindShortcut {
			text, colour := shortcutText(c.key, c.modifiers), theme.Ink
			if text == "" {
				text, colour = pick(lit, "Press the keys to use", "None"), theme.Soft
			}
			w.write(cv, text, c.font, x+inset, y, width-2*inset, height, c.flags, colour)
			break
		}
		if c.chosen >= 0 && c.chosen < len(c.entries) {
			w.write(cv, c.entries[c.chosen], c.font, x+inset, y, width-inset-w.px(34), height, c.flags, theme.Ink)
		}
		// The arrow that says there is a list.
		ax, ay, arm := float64(x+width)-w.pxf(19), float64(y)+float64(height)/2, w.pxf(4)
		cv.Line(ax-arm, ay-arm/2, ax, ay+arm/2, w.pxf(1.5), theme.Soft)
		cv.Line(ax, ay+arm/2, ax+arm, ay-arm/2, w.pxf(1.5), theme.Soft)
	case kindPath:
		cv.Round(x, y, width, height, radius, theme.Well)
		cv.Ring(x, y, width, height, radius, hair, theme.WellEdge)
		inset := w.px(12)
		if c.text == "" {
			w.write(cv, c.empty, c.font, x+inset, y, width-2*inset, height, textMiddle|textEllipsis, theme.Quiet)
		} else {
			w.write(cv, c.text, c.font, x+inset, y, width-2*inset, height, c.flags, c.colour)
		}
	case kindBar:
		cv.Round(x, y, width, height, float64(height)/2, theme.Track)
		if filled := c.filled(); filled > 0 {
			cv.Round(x, y, filled, height, float64(height)/2, theme.Lead)
		}
	case kindLight:
		across := 2 * w.px(5)
		left, top := x+(width-across)/2, y+(height-across)/2
		switch c.tone {
		case Live:
			cv.Round(left, top, across, across, float64(across)/2, theme.Live)
		case Waiting:
			cv.Round(left, top, across, across, float64(across)/2, theme.Wait)
		default:
			cv.Ring(left, top, across, across, float64(across)/2, w.pxf(1.5), theme.LightOff)
		}
	case kindMinimise, kindClose:
		colour := theme.Soft
		if hover {
			colour = theme.Ink
			fill := pick(c.kind == kindClose, theme.CloseHover, theme.PlainHover)
			if down {
				fill = fill.Thinned(0.7)
			}
			cv.Fill(x, y, width, height, fill)
		}
		cx, cy, arm := float64(x)+float64(width)/2, float64(y)+float64(height)/2, w.pxf(5)
		if c.kind == kindMinimise {
			cv.Line(cx-arm, cy, cx+arm, cy, hair, colour)
		} else {
			cv.Line(cx-arm, cy-arm, cx+arm, cy+arm, hair, colour)
			cv.Line(cx-arm, cy+arm, cx+arm, cy-arm, hair, colour)
		}
	}
	// A ring shows where the keyboard is, once the keyboard has been used
	// to get there.
	if w.focus == c && w.keyboard && w.active {
		gap := w.px(3)
		if c.kind == kindSwitch {
			gap = w.px(reach)
		}
		cv.Ring(x-gap, y-gap, width+2*gap, height+2*gap, radius+float64(gap), w.pxf(2), theme.Focus)
	}
}

// pick returns one of two values.
func pick[T any](condition bool, yes, no T) T {
	if condition {
		return yes
	}
	return no
}
