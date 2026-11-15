package winui

import "testing"

func TestCombination(t *testing.T) {
	const f9, letterS = 0x78, 0x53
	for _, c := range []struct {
		name          string
		key, held     uint32
		action        int
		wantModifiers uint32
	}{
		{"Ctrl and Shift with a key are kept as pressed", f9, Ctrl | Shift, keySets, Ctrl | Shift},
		{"Alt with a key is kept", letterS, Alt, keySets, Alt},
		{"all three are kept", f9, Ctrl | Shift | Alt, keySets, Ctrl | Shift | Alt},
		{"a key on its own has Ctrl and Alt added", f9, 0, keySets, Ctrl | Alt},
		{"a key with Shift alone has Ctrl and Alt added", letterS, Shift, keySets, Ctrl | Alt | Shift},
		{"Backspace removes the shortcut", vkBack, 0, keyClears, 0},
		{"Delete removes the shortcut", vkDelete, Ctrl, keyClears, 0},
		{"Shift on its own is not a shortcut", vkShift, Shift, keyIgnored, 0},
		{"Ctrl on its own is not a shortcut", vkControl, Ctrl, keyIgnored, 0},
		{"the right Alt key on its own is not a shortcut", vkRMenu, Alt, keyIgnored, 0},
		{"the Windows key is left to Windows", vkLWin, 0, keyIgnored, 0},
		{"Tab moves on and is not taken", vkTab, 0, keyIgnored, 0},
		{"Escape is not taken", vkEscape, 0, keyIgnored, 0},
		{"Enter is not taken", vkReturn, Ctrl, keyIgnored, 0},
		{"Space is not taken", vkSpace, Ctrl, keyIgnored, 0},
		{"Caps Lock is not taken", vkCapital, 0, keyIgnored, 0},
		{"no key is no shortcut", 0, Ctrl, keyIgnored, 0},
		{"a number Windows does not use for keys is refused", 0xFF, Ctrl, keyIgnored, 0},
	} {
		action, modifiers := combination(c.key, c.held)
		if action != c.action || modifiers != c.wantModifiers {
			t.Errorf("%s: got action %d modifiers %d, want %d and %d", c.name, action, modifiers, c.action, c.wantModifiers)
		}
	}
}

func TestStepThroughAList(t *testing.T) {
	for _, c := range []struct {
		key            uint32
		current, count int
		want           int
		moves          bool
	}{
		{vkDown, 0, 5, 1, true},
		{vkDown, 4, 5, 4, true}, // stops at the end
		{vkUp, 3, 5, 2, true},
		{vkUp, 0, 5, 0, true}, // stops at the start
		{vkHome, 3, 5, 0, true},
		{vkEnd, 1, 5, 4, true},
		{vkPrior, 3, 5, 0, true},
		{vkNext, 1, 5, 4, true},
		{vkLeft, 2, 5, 2, false},
		{vkDown, 0, 0, 0, false}, // an empty list has nowhere to go
	} {
		if got, moves := step(c.key, c.current, c.count); got != c.want || moves != c.moves {
			t.Errorf("key %#x from %d of %d: got %d %v, want %d %v", c.key, c.current, c.count, got, moves, c.want, c.moves)
		}
	}
}
