package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// pastelDarkTheme keeps Fyne's accessible widgets and icons while giving the
// companion a softer game-dashboard palette.
type pastelDarkTheme struct{ base fyne.Theme }

func newPastelDarkTheme() fyne.Theme { return &pastelDarkTheme{base: theme.DefaultTheme()} }

func (t *pastelDarkTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x14, G: 0x17, B: 0x24, A: 0xff}
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 0x1d, G: 0x21, B: 0x33, A: 0xff}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0x20, G: 0x24, B: 0x38, A: 0xff}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x29, G: 0x2e, B: 0x45, A: 0xff}
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return color.NRGBA{R: 0xac, G: 0x92, B: 0xf4, A: 0xff}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xee, G: 0xf0, B: 0xf8, A: 0xff}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 0x8f, G: 0x96, B: 0xad, A: 0xff}
	case theme.ColorNameHover:
		return color.NRGBA{R: 0xac, G: 0x92, B: 0xf4, A: 0x24}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0xac, G: 0x92, B: 0xf4, A: 0x46}
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 0x78, G: 0xd7, B: 0xb5, A: 0xff}
	case theme.ColorNameWarning:
		return color.NRGBA{R: 0xf2, G: 0xc8, B: 0x79, A: 0xff}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xf1, G: 0x8f, B: 0xa7, A: 0xff}
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 0x3a, G: 0x40, B: 0x59, A: 0x99}
	}
	return t.base.Color(name, theme.VariantDark)
}

func (t *pastelDarkTheme) Font(style fyne.TextStyle) fyne.Resource    { return t.base.Font(style) }
func (t *pastelDarkTheme) Icon(name fyne.ThemeIconName) fyne.Resource { return t.base.Icon(name) }
func (t *pastelDarkTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 8
	case theme.SizeNameText:
		return 15
	case theme.SizeNameHeadingText:
		return 24
	}
	return t.base.Size(name)
}
