package winui

import "testing"

// The window is see-through, so what its text is read against depends on
// what happens to be behind it. These tests lay the window's colours over
// the two worst cases, a white screen and a black one, and measure what
// would be seen. 4.5 is the least contrast the accessibility guidelines for
// the web accept for body text.

func seen(behind Colour, layers ...Colour) (r, g, b uint32) {
	c := blank(1, 1)
	c.Set(0, 0, 1, 1, layers[0])
	for _, layer := range layers[1:] {
		c.Fill(0, 0, 1, 1, layer)
	}
	return Shown(c.at(0, 0), behind)
}

func TestTextIsReadableOverAnything(t *testing.T) {
	// Each ground is what lies under some text, from the background up,
	// with the colours of the text that is written on it.
	for _, ground := range []struct {
		name   string
		layers []Colour
		texts  []Colour
	}{
		{"the window", []Colour{theme.Sheet}, []Colour{theme.Ink, theme.Soft, theme.Quiet}},
		{"the window without blur", []Colour{theme.SheetPlain}, []Colour{theme.Ink, theme.Soft, theme.Quiet}},
		{"a card", []Colour{theme.Sheet, theme.Panel}, []Colour{theme.Ink, theme.Soft, theme.Quiet}},
		{"the section buttons", []Colour{theme.Sheet, theme.Strip}, []Colour{theme.Soft}},
		{"a section button under the pointer", []Colour{theme.Sheet, theme.Strip, theme.TabHover}, []Colour{theme.Ink}},
		{"the chosen section button", []Colour{theme.Sheet, theme.Strip, theme.TabChosen}, []Colour{theme.Ink}},
		{"a button", []Colour{theme.Sheet, theme.Plain}, []Colour{theme.Ink}},
		{"a button under the pointer", []Colour{theme.Sheet, theme.PlainHover}, []Colour{theme.Ink}},
		{"a pressed button", []Colour{theme.Sheet, theme.PlainDown}, []Colour{theme.Ink}},
		{"a button on a card", []Colour{theme.Sheet, theme.Panel, theme.Plain}, []Colour{theme.Ink}},
		{"a button on a card, under the pointer", []Colour{theme.Sheet, theme.Panel, theme.PlainHover}, []Colour{theme.Ink}},
		{"a list box", []Colour{theme.Sheet, theme.Panel, theme.Field}, []Colour{theme.Ink, theme.Soft}},
		{"a list box under the pointer", []Colour{theme.Sheet, theme.Panel, theme.FieldHover}, []Colour{theme.Ink, theme.Soft}},
		{"a path box", []Colour{theme.Sheet, theme.Panel, theme.Well}, []Colour{theme.Soft, theme.Quiet}},
		{"a table", []Colour{theme.Sheet, theme.Well}, []Colour{theme.Ink, theme.Soft, Red, Amber, Green, Grey}},
		{"the chosen row of a table", []Colour{theme.Sheet, theme.Well, theme.RowChosen}, []Colour{theme.Ink, Red, Amber, Green, Grey}},
		{"an open list", []Colour{theme.Sheet, theme.List}, []Colour{theme.Ink, theme.Soft}},
		{"an entry under the pointer", []Colour{theme.Sheet, theme.List, theme.ListHover}, []Colour{theme.Ink}},
	} {
		for _, behind := range []Colour{0xFFFFFFFF, 0xFF000000} {
			r, g, b := seen(behind, ground.layers...)
			for _, text := range ground.texts {
				tr, tg, tb := seen(behind, append(append([]Colour{}, ground.layers...), text)...)
				if contrast := Contrast(tr, tg, tb, r, g, b); contrast < 4.5 {
					t.Errorf("text %06X on %s over %06X: contrast %.2f, want 4.5 or more", text&0xFFFFFF, ground.name, behind&0xFFFFFF, contrast)
				}
			}
		}
	}
}

func TestLightButtonsAreReadable(t *testing.T) {
	for name, fill := range map[string]Colour{"lead": theme.Lead, "under the pointer": theme.LeadHover, "pressed": theme.LeadDown} {
		r, g, b := seen(0xFFFFFFFF, theme.Sheet, fill)
		tr, tg, tb := seen(0xFFFFFFFF, theme.Sheet, fill, theme.OnLight)
		if contrast := Contrast(tr, tg, tb, r, g, b); contrast < 4.5 {
			t.Errorf("the light button, %s: contrast %.2f", name, contrast)
		}
	}
}

func TestStateLightsStandOut(t *testing.T) {
	// The recording light has to be seen at a glance, on the card it sits on.
	for name, colour := range map[string]Colour{"recording": theme.Live, "waiting": theme.Wait} {
		for _, behind := range []Colour{0xFFFFFFFF, 0xFF000000} {
			r, g, b := seen(behind, theme.Sheet, theme.Panel)
			lr, lg, lb := seen(behind, theme.Sheet, theme.Panel, colour)
			if contrast := Contrast(lr, lg, lb, r, g, b); contrast < 3 {
				t.Errorf("the %s light over %06X: contrast %.2f, want 3 or more", name, behind&0xFFFFFF, contrast)
			}
		}
	}
}

func TestTheWindowIsSeeThroughButNotTooMuch(t *testing.T) {
	if opacity := theme.Sheet >> 24; opacity < 0xB3 || opacity > 0xE6 {
		t.Errorf("the background is %d of 255 solid: it should let something through, and not so much that text suffers", opacity)
	}
	if theme.SheetPlain>>24 <= theme.Sheet>>24 {
		t.Error("without blur the background should let less through, not more")
	}
	if theme.SheetSolid>>24 != 0xFF {
		t.Error("with high contrast on, nothing should show through")
	}
}
