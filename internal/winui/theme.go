package winui

// theme holds the colours of the window. The window is black and
// see-through: Sheet is its background and everything else is laid over it.
var theme = struct {
	Sheet      Colour // the background, which lets a fifth of what is behind show through
	SheetPlain Colour // the background where Windows cannot blur what is behind
	SheetSolid Colour // the background when high contrast is turned on in Windows

	Ink     Colour // text
	Soft    Colour // labels and explanations
	Quiet   Colour // notes and the line at the foot
	Faint   Colour // the text of something that cannot be used now
	OnLight Colour // text on a light button

	Edge      Colour // the rim of the window
	Panel     Colour // a card that groups lines: darker, so less shows through where the text is
	PanelEdge Colour
	Rule      Colour // a dividing line
	Well      Colour // a box that shows a path
	WellEdge  Colour

	Strip     Colour // the track the section buttons sit in
	TabChosen Colour
	TabHover  Colour
	RowChosen Colour // the chosen row of a table

	Plain      Colour // an ordinary button
	PlainHover Colour
	PlainDown  Colour
	PlainOff   Colour
	PlainEdge  Colour
	Lead       Colour // the button for the thing to do next
	LeadHover  Colour
	LeadDown   Colour

	Field      Colour // a list or the shortcut box
	FieldHover Colour
	FieldEdge  Colour
	FieldLit   Colour // its rim while it is open or being set
	List       Colour // the entries of an open list
	ListEdge   Colour
	ListHover  Colour

	Track      Colour // the empty part of the level bar
	SwitchOff  Colour
	SwitchEdge Colour
	KnobOff    Colour
	Focus      Colour // the ring round the control the keyboard is on
	CloseHover Colour

	Live     Colour // the light while recording
	Wait     Colour // the light while starting or held up
	LightOff Colour // the ring that stands for the light when it is off
}{
	Sheet:      0xCC0B0B0B,
	SheetPlain: 0xEB0B0B0B,
	SheetSolid: 0xFF0B0B0B,

	Ink:     0xFFF3F3F3,
	Soft:    0xFFC2C2C2,
	Quiet:   0xFFA9A9A9,
	Faint:   0x5CFFFFFF,
	OnLight: 0xFF0B0B0B,

	Edge:      0x2EFFFFFF,
	Panel:     0x40000000,
	PanelEdge: 0x1AFFFFFF,
	Rule:      0x1AFFFFFF,
	Well:      0x52000000,
	WellEdge:  0x1AFFFFFF,

	Strip:     0x47000000,
	TabChosen: 0x24FFFFFF,
	TabHover:  0x0FFFFFFF,
	RowChosen: 0x1AFFFFFF,

	Plain:      0x17FFFFFF,
	PlainHover: 0x26FFFFFF,
	PlainDown:  0x0DFFFFFF,
	PlainOff:   0x0AFFFFFF,
	PlainEdge:  0x24FFFFFF,
	Lead:       0xFFF3F3F3,
	LeadHover:  0xFFFFFFFF,
	LeadDown:   0xFFC9C9C9,

	Field:      0x14FFFFFF,
	FieldHover: 0x21FFFFFF,
	FieldEdge:  0x2EFFFFFF,
	FieldLit:   0x8CFFFFFF,
	List:       0xFA1B1B1B,
	ListEdge:   0x33FFFFFF,
	ListHover:  0x1FFFFFFF,

	Track:      0x24FFFFFF,
	SwitchOff:  0x1FFFFFFF,
	SwitchEdge: 0x61FFFFFF,
	KnobOff:    0xFFC2C2C2,
	Focus:      0xFFFFFFFF,
	CloseHover: 0xFFC42B1C,

	Live:     0xFFFF5449,
	Wait:     0xFFF2B33D,
	LightOff: 0x70FFFFFF,
}

// Colours for marking a row of a table, readable on its dark ground.
const (
	Red   Colour = 0xFFFF8F85
	Amber Colour = 0xFFF2B33D
	Green Colour = 0xFF6FD08C
	Grey  Colour = 0xFFB8B8B8
)
