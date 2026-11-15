package winui

// Shortcut modifier keys, as Windows numbers them for global shortcuts.
const (
	Alt   = 1
	Ctrl  = 2
	Shift = 4
)

// What a key pressed in a shortcut box does.
const (
	keyIgnored = iota // a modifier on its own, or a key that cannot be a shortcut
	keyClears         // the shortcut is removed
	keySets           // the key and the modifiers held become the shortcut
)

// Windows' numbers for the keys that are treated specially.
const (
	vkBack    = 0x08
	vkTab     = 0x09
	vkReturn  = 0x0D
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12 // Alt
	vkPause   = 0x13
	vkCapital = 0x14
	vkEscape  = 0x1B
	vkSpace   = 0x20
	vkPrior   = 0x21 // Page Up
	vkNext    = 0x22 // Page Down
	vkEnd     = 0x23
	vkHome    = 0x24
	vkLeft    = 0x25
	vkUp      = 0x26
	vkRight   = 0x27
	vkDown    = 0x28
	vkDelete  = 0x2E
	vkLWin    = 0x5B
	vkRWin    = 0x5C
	vkApps    = 0x5D
	vkF4      = 0x73
	vkNumLock = 0x90
	vkScroll  = 0x91
	vkLShift  = 0xA0
	vkRMenu   = 0xA5
)

// combination decides what a key pressed in a shortcut box means. held is
// the modifiers down at that moment.
//
// A key on its own, or with Shift alone, would be taken away from every
// other program, because a global shortcut swallows its key wherever it is
// pressed. Such a choice has Ctrl and Alt added to it.
func combination(key, held uint32) (action int, modifiers uint32) {
	switch {
	case key == vkBack || key == vkDelete:
		return keyClears, 0
	case key == vkShift || key == vkControl || key == vkMenu || (key >= vkLShift && key <= vkRMenu),
		key == vkLWin || key == vkRWin || key == vkApps,
		key == vkTab || key == vkReturn || key == vkEscape || key == vkSpace,
		key == vkCapital || key == vkNumLock || key == vkScroll,
		key == 0 || key > 0xFE:
		return keyIgnored, 0
	}
	held &= Alt | Ctrl | Shift
	if held&(Alt|Ctrl) == 0 {
		held |= Alt | Ctrl
	}
	return keySets, held
}

// step returns the entry to move to from entry current of a list of count
// entries when a key is pressed, and whether the key moves at all.
func step(key uint32, current, count int) (int, bool) {
	if count == 0 {
		return current, false
	}
	switch key {
	case vkUp:
		return max(current-1, 0), true
	case vkDown:
		return min(current+1, count-1), true
	case vkHome, vkPrior:
		return 0, true
	case vkEnd, vkNext:
		return count - 1, true
	}
	return current, false
}
