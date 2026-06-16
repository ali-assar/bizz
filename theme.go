package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Brand palette: #1A1108 ink, #F5A623 amber, #F5C842 gold.
var (
	colorInk       = color.NRGBA{R: 0x1a, G: 0x11, B: 0x08, A: 0xff}
	colorInkSoft   = color.NRGBA{R: 0x1a, G: 0x11, B: 0x08, A: 0xff}
	colorAmber     = color.NRGBA{R: 0xf5, G: 0xa6, B: 0x23, A: 0xff}
	colorAmberGlow = color.NRGBA{R: 0xf5, G: 0xc8, B: 0x42, A: 0xff}
	colorAmberDim  = color.NRGBA{R: 0xf5, G: 0xa6, B: 0x23, A: 0x99}
	colorMuted     = color.NRGBA{R: 0xf5, G: 0xc8, B: 0x42, A: 0x99}
)

type bizzTheme struct {
	base fyne.Theme
}

func newBizzTheme() fyne.Theme {
	return &bizzTheme{base: theme.DefaultTheme()}
}

func (t *bizzTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colorInk
	case theme.ColorNameButton:
		return colorAmber
	case theme.ColorNameDisabledButton:
		return colorAmberDim
	case theme.ColorNameForeground:
		return colorAmberGlow
	case theme.ColorNameDisabled:
		return colorMuted
	case theme.ColorNameInputBackground:
		return colorInkSoft
	case theme.ColorNameInputBorder:
		return colorAmber
	case theme.ColorNamePlaceHolder:
		return colorMuted
	case theme.ColorNamePrimary:
		return colorAmber
	case theme.ColorNameHover:
		return colorAmberGlow
	case theme.ColorNamePressed:
		return colorAmber
	case theme.ColorNameSelection:
		return colorAmberDim
	case theme.ColorNameSeparator:
		return colorAmberDim
	case theme.ColorNameHeaderBackground:
		return colorInk
	case theme.ColorNameScrollBar:
		return colorAmberDim
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0x1a, G: 0x11, B: 0x08, A: 0xcc}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 0x1a, G: 0x11, B: 0x08, A: 0xee}
	case theme.ColorNameMenuBackground:
		return colorInk
	}
	return t.base.Color(name, variant)
}

func (t *bizzTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.base.Font(style)
}

func (t *bizzTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *bizzTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameInnerPadding:
		return 6
	case theme.SizeNameText:
		return 13
	case theme.SizeNameHeadingText:
		return 16
	case theme.SizeNameSubHeadingText:
		return 14
	case theme.SizeNameInputBorder:
		return 2
	case theme.SizeNameSeparatorThickness:
		return 2
	}
	return t.base.Size(name)
}
