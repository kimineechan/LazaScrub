//go:build windows

package winui

import "testing"

// bench is a window that exists only in memory. It is enough for the
// controls to work out what the keyboard and the pointer do; nothing is
// shown and nothing is drawn.
type bench struct {
	*Window
	first, second       *Control // section buttons
	button, off, toggle *Control
	choice, shortcut    *Control
	other               *Control // a button on the second page
	pressed, flipped    int
	chose, set          int
}

func newBench() *bench {
	b := &bench{Window: &Window{scale: 1, width: 400, height: 400, page: -1, pick: -1, active: true}}
	w := b.Window
	w.Frame()
	w.Title("Name", 10, 10, 200, 30)
	b.first = w.Section("01  One", 0, 10, 50, 180, 30)
	b.second = w.Section("02  Two", 1, 200, 50, 180, 30)
	w.Page(0)
	w.Label("Words", 10, 90, 380, 20)
	b.button = w.Button("Press", 10, 120, 100, 30, func() { b.pressed++ })
	b.off = w.Button("Cannot", 120, 120, 100, 30, func() { b.pressed += 100 })
	b.off.Enable(false)
	b.toggle = w.Switch("Flip", 10, 160, 200, 26, func() { b.flipped++ })
	b.choice = w.Choice([]string{"zero", "one", "two"}, 10, 200, 200, 30, func() { b.chose++ })
	b.shortcut = w.Shortcut(10, 240, 200, 30, func() { b.set++ })
	w.Path("Nothing yet", 10, 280, 200, 30)
	w.Bar(10, 320, 200, 6)
	w.Page(1)
	b.other = w.Button("Other", 10, 120, 100, 30, func() { b.pressed += 1000 })
	return b
}

func (b *bench) key(key uint32) { b.keyDown(key, false, false) }
func (b *bench) click(x, y int) { b.pointerDown(x, y); b.pointerUp(x, y) }
func (b *bench) middle(c *Control) (int, int) {
	x, y, width, height := c.box()
	return x + width/2, y + height/2
}

func TestTabVisitsWhatCanBeUsedInOrder(t *testing.T) {
	b := newBench()
	want := []*Control{b.first, b.second, b.button, b.toggle, b.choice, b.shortcut, b.first}
	for i, c := range want {
		b.key(vkTab)
		if b.focus != c {
			t.Fatalf("stop %d: the keyboard is on %q, want %q", i, name(b.focus), name(c))
		}
		if !b.keyboard {
			t.Fatal("moving with Tab should ring the control the keyboard is on")
		}
	}
	// Backwards from the first stop wraps to the last.
	b.focusNext(true)
	if b.focus != b.shortcut {
		t.Errorf("backwards went to %q", name(b.focus))
	}
	b.focusNext(true)
	if b.focus != b.choice {
		t.Errorf("backwards again went to %q", name(b.focus))
	}
}

func name(c *Control) string {
	if c == nil {
		return "nothing"
	}
	return c.text
}

func TestOnlyThePageOnShowCanBeUsed(t *testing.T) {
	b := newBench()
	b.focusOn(b.button)
	b.ShowPage(1)
	if b.focus != nil {
		t.Error("the keyboard stayed on a control that is no longer on show")
	}
	x, y := b.middle(b.button) // the other page has a button in the same place
	b.click(x, y)
	if b.pressed != 1000 {
		t.Errorf("pressed %d: the click went to the wrong page", b.pressed)
	}
	for range 4 {
		b.key(vkTab)
		if b.focus == b.button || b.focus == b.toggle {
			t.Fatalf("Tab reached %q on the hidden page", name(b.focus))
		}
	}
}

func TestSectionKeys(t *testing.T) {
	b := newBench()
	shown := -1
	b.OnPage(func(page int) { shown = page })
	b.focusOn(b.first)
	b.key(vkRight)
	if b.current != 1 || shown != 1 || b.focus != b.second {
		t.Errorf("the right arrow: page %d, told %d, keyboard on %q", b.current, shown, name(b.focus))
	}
	b.key(vkLeft)
	if b.current != 0 || b.focus != b.first {
		t.Errorf("the left arrow: page %d, keyboard on %q", b.current, name(b.focus))
	}
	b.focusOn(b.second)
	b.key(vkReturn)
	if b.current != 1 {
		t.Error("Enter on a section button should show its page")
	}
}

func TestEnterAndSpacePress(t *testing.T) {
	b := newBench()
	b.focusOn(b.button)
	b.key(vkReturn)
	b.key(vkSpace)
	if b.pressed != 2 {
		t.Errorf("pressed %d times, want 2", b.pressed)
	}
	b.keyDown(vkReturn, true, false) // a key held down repeats; a button should not
	if b.pressed != 2 {
		t.Error("a held key pressed the button again")
	}
	b.focusOn(b.toggle)
	b.key(vkSpace)
	if !b.toggle.On() || b.flipped != 1 {
		t.Errorf("the switch: on %v, action ran %d times", b.toggle.On(), b.flipped)
	}
	b.toggle.Turn(false)
	if b.flipped != 1 {
		t.Error("setting a switch from the program should not run its action")
	}
}

func TestPointerPresses(t *testing.T) {
	b := newBench()
	x, y := b.middle(b.button)
	b.click(x, y)
	if b.pressed != 1 || b.focus != b.button || b.keyboard {
		t.Errorf("pressed %d, keyboard on %q, ringed %v", b.pressed, name(b.focus), b.keyboard)
	}
	// Pressed on the button but let go somewhere else: nothing happens.
	b.pointerDown(x, y)
	b.pointerUp(x+300, y+100)
	if b.pressed != 1 {
		t.Error("letting go off the button still pressed it")
	}
	ox, oy := b.middle(b.off)
	b.click(ox, oy)
	if b.pressed != 1 {
		t.Error("a button that cannot be used was pressed")
	}
	sx, sy := b.middle(b.toggle)
	b.click(sx, sy)
	if !b.toggle.On() || b.flipped != 1 {
		t.Error("a click should flip the switch")
	}
	// The window's own buttons take clicks but are not stops for Tab.
	for _, c := range b.controls {
		if (c.kind == kindMinimise || c.kind == kindClose) && (!c.usable() || c.reachable()) {
			t.Error("minimise and close should work with the pointer and be skipped by Tab")
		}
	}
}

func TestListWithTheKeyboard(t *testing.T) {
	b := newBench()
	b.focusOn(b.choice)
	b.key(vkDown) // opens the list; nothing is chosen yet
	if b.open != b.choice || b.pick != 0 || b.chose != 0 {
		t.Fatalf("open %v, on entry %d, action ran %d times", b.open == b.choice, b.pick, b.chose)
	}
	b.key(vkDown)
	b.key(vkDown)
	b.key(vkDown) // stops at the last entry
	if b.pick != 2 || b.choice.Chosen() != 0 {
		t.Fatalf("on entry %d, chosen %d", b.pick, b.choice.Chosen())
	}
	b.key(vkReturn)
	if b.open != nil || b.choice.Chosen() != 2 || b.chose != 1 {
		t.Fatalf("after Enter: open %v, chosen %d, action ran %d times", b.open != nil, b.choice.Chosen(), b.chose)
	}
	// Escape puts the list away and changes nothing.
	b.key(vkF4)
	b.key(vkHome)
	b.key(vkEscape)
	if b.open != nil || b.choice.Chosen() != 2 || b.chose != 1 {
		t.Errorf("after Escape: open %v, chosen %d, action ran %d times", b.open != nil, b.choice.Chosen(), b.chose)
	}
	// Choosing the entry already chosen is not a change.
	b.key(vkSpace)
	b.key(vkReturn)
	if b.chose != 1 {
		t.Error("choosing the same entry ran the action")
	}
	// While a list is open the keys stay in it.
	b.key(vkSpace)
	b.key(vkTab)
	if b.open != nil || b.focus != b.shortcut {
		t.Errorf("Tab from an open list: open %v, keyboard on %q", b.open != nil, name(b.focus))
	}
}

func TestListWithThePointer(t *testing.T) {
	b := newBench()
	x, y := b.middle(b.choice)
	b.click(x, y)
	if b.open != b.choice {
		t.Fatal("a click should open the list")
	}
	lx, ly, width, _, row := b.listBox()
	b.pointerMoved(lx+width/2, ly+4+row+row/2)
	if b.pick != 1 {
		t.Fatalf("the pointer is on entry %d, want 1", b.pick)
	}
	b.pointerDown(lx+width/2, ly+4+row+row/2)
	if b.open != nil || b.choice.Chosen() != 1 || b.chose != 1 {
		t.Fatalf("after a click on an entry: open %v, chosen %d, action ran %d times", b.open != nil, b.choice.Chosen(), b.chose)
	}
	// A click elsewhere only puts the list away. It does not press what
	// is under it, here the first button.
	b.click(x, y)
	bx, by := b.middle(b.button)
	b.click(bx, by)
	if b.open != nil || b.pressed != 0 || b.choice.Chosen() != 1 {
		t.Errorf("after a click outside: open %v, button pressed %d times, chosen %d", b.open != nil, b.pressed, b.choice.Chosen())
	}
	b.choice.SetEntries([]string{"only"})
	if b.choice.Chosen() != 0 {
		t.Errorf("after new entries the chosen one is %d", b.choice.Chosen())
	}
}

func TestShortcutBoxTakesKeysDown(t *testing.T) {
	if held() != 0 {
		t.Skip("a modifier key is being held on this computer")
	}
	b := newBench()
	b.focusOn(b.shortcut)
	b.key('S') // on its own, so Ctrl and Alt are added
	if key, modifiers := b.shortcut.Keys(); key != 'S' || modifiers != Ctrl|Alt || b.set != 1 {
		t.Fatalf("key %#x, modifiers %d, action ran %d times", key, modifiers, b.set)
	}
	if got := ShortcutName(b.shortcut.Keys()); got != "Ctrl + Alt + S" {
		t.Errorf("shown as %q", got)
	}
	if got := ShortcutName(0x78, Ctrl|Shift); got != "Ctrl + Shift + F9" {
		t.Errorf("the usual shortcut is shown as %q", got)
	}
	b.key(vkEscape) // not a shortcut, and not passed on either
	b.key(vkReturn)
	if key, _ := b.shortcut.Keys(); key != 'S' || b.set != 1 || b.focus != b.shortcut {
		t.Error("Escape or Enter changed the shortcut or moved the keyboard")
	}
	b.key(vkBack)
	if key, _ := b.shortcut.Keys(); key != 0 || b.set != 2 {
		t.Errorf("Backspace: key %#x, action ran %d times", key, b.set)
	}
	if ShortcutName(0, 0) != "" {
		t.Error("no shortcut should have no name")
	}
	b.key(vkTab)
	if b.focus == b.shortcut {
		t.Error("Tab should move on from the shortcut box")
	}
}

func TestOnlyChangesAreRedrawn(t *testing.T) {
	b := newBench()
	line := b.Line("same", 10, 10, 100, 20)
	bar := b.Bar(10, 40, 200, 6)
	light := b.Light(10, 60, 12, 20)
	b.stale = rect{}
	line.SetText("same")
	bar.SetLevel(0)
	light.Show(Off)
	b.button.Enable(true)
	b.button.Lead(false)
	b.toggle.Turn(false)
	b.choice.Choose(0)
	if b.stale != (rect{}) {
		t.Fatalf("setting what was already so asked for a redraw of %+v", b.stale)
	}
	bar.SetLevel(0.5)
	b.stale = rect{}
	bar.SetLevel(0.501) // less than a pixel of a bar 200 wide
	if b.stale != (rect{}) {
		t.Error("a change too small to see asked for a redraw")
	}
	line.SetText("new")
	if b.stale == (rect{}) {
		t.Error("a new text asked for no redraw")
	}
	if b.stale.Right > 200 || b.stale.Bottom > 60 {
		t.Errorf("one line of text asked for %+v to be redrawn", b.stale)
	}
}

func TestDisablingTheFocusedControlFreesTheKeyboard(t *testing.T) {
	b := newBench()
	b.focusOn(b.button)
	b.button.Enable(false)
	if b.focus != nil {
		t.Error("the keyboard stayed on a control that cannot be used")
	}
	b.key(vkReturn)
	if b.pressed != 0 {
		t.Error("Enter pressed a disabled button")
	}
}

// A real window, never shown, to see that nothing is drawn while it is
// hidden and that drawing produces a picture when asked.
func TestAHiddenWindowDrawsNothing(t *testing.T) {
	w, err := New("LazaScrub test", "LazaScrubTestWindow", 320, 200)
	if err != nil {
		t.Skip("no window can be made here:", err)
	}
	defer w.Close()
	line := w.Line("Before", 20, 20, 280, 30)
	w.Button("Button", 20, 80, 120, 40, nil)
	line.SetText("After")
	w.flush()
	if w.drawn {
		t.Fatal("a hidden window was drawn")
	}
	if w.stale == (rect{}) {
		t.Fatal("the change was forgotten instead of kept for when the window is shown")
	}
	w.draw(w.whole())
	if w.stale != (rect{}) {
		t.Error("drawing should leave nothing out of date")
	}
	a, r, g, b := w.sheet.parts()
	ground := a<<24 | r<<16 | g<<8 | b
	if got := w.canvas.at(w.px(310), w.px(190)); got != ground {
		t.Errorf("an empty part of the window is %08X, want the background %08X", got, ground)
	}
	// The text must have left marks where it was written, and only there.
	x, y, width, height := line.box()
	lettered := 0
	for py := y; py < y+height; py++ {
		for px := x; px < x+width; px++ {
			if w.canvas.at(px, py) != ground {
				lettered++
			}
		}
	}
	if lettered < 50 {
		t.Errorf("the text left %d marks; Windows' text drawing did not reach the picture", lettered)
	}
	for _, p := range w.canvas.Pix {
		if alpha := p >> 24; alpha < ground>>24 {
			t.Fatalf("a pixel is more see-through (%d) than the background (%d): text drawing punched a hole", alpha, ground>>24)
		}
	}
}

func TestTable(t *testing.T) {
	b := newBench()
	picked := -1
	b.Page(-1)
	table := b.Table(10, 40, 380, 32+4+26*4, []Column{{Title: "A", Width: 100}, {Title: "B"}}, func(row int) { picked = row })
	rows := make([][]string, 10)
	for i := range rows {
		rows[i] = []string{"a", "b"}
	}
	table.Fill(rows, nil)
	if _, _, fit, _ := table.tableMeasures(); fit != 4 {
		t.Fatalf("%d rows fit, want 4", fit)
	}
	// A click on the second row chooses it.
	x, y, _, _ := table.box()
	b.click(x+50, y+32+26+5)
	if picked != 1 || b.focus != table {
		t.Fatalf("picked %d, keyboard on %q", picked, name(b.focus))
	}
	// The arrow keys move the choice and bring it into view.
	for range 5 {
		b.key(vkDown)
	}
	if picked != 6 || table.table.top != 3 {
		t.Errorf("after five Down keys: picked %d, first row on show %d", picked, table.table.top)
	}
	b.key(vkEnd)
	if picked != 9 || table.table.top != 6 {
		t.Errorf("End: picked %d, first row on show %d", picked, table.table.top)
	}
	b.key(vkDown) // already at the last row
	b.key(vkHome)
	if picked != 0 || table.table.top != 0 {
		t.Errorf("Home: picked %d, first row on show %d", picked, table.table.top)
	}
	// Scrolling stops at both ends and does not change the choice.
	table.scrollTo(100)
	if table.table.top != 6 || picked != 0 {
		t.Errorf("scrolled past the end: first row %d, picked %d", table.table.top, picked)
	}
	table.scrollTo(-5)
	if table.table.top != 0 {
		t.Errorf("scrolled past the start: first row %d", table.table.top)
	}
	// A click below the last row chooses nothing new.
	table.Fill(rows[:2], nil)
	picked = -1
	b.click(x+50, y+32+26*3+5)
	if picked != -1 {
		t.Errorf("a click on empty space picked row %d", picked)
	}
	if _, length, _ := table.thumb(); length != 0 {
		t.Error("a table whose rows all fit should have no scroll bar")
	}
	table.SetCell(1, 1, "changed")
	if table.table.rows[1][1] != "changed" {
		t.Error("the cell was not changed")
	}
	table.SetCell(7, 0, "nowhere") // a row that is not there is ignored
}
