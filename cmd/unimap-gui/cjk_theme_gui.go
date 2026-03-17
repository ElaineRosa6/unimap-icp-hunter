//go:build gui
// +build gui

package main

import (
	"image/color"
	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type cjkTheme struct {
	base    fyne.Theme
	regular fyne.Resource
}

func (t *cjkTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	return t.base.Color(name, variant)
}

func (t *cjkTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *cjkTheme) Font(style fyne.TextStyle) fyne.Resource {
	if t.regular != nil {
		return t.regular
	}
	return t.base.Font(style)
}

func (t *cjkTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.base.Size(name)
}

// newCJKTheme tries to load a Windows-available CJK font and returns a fyne.Theme
// that uses it as the regular font. If no font can be loaded, it returns nil.
func newCJKTheme() fyne.Theme {
	fontPath := os.Getenv("UNIMAP_GUI_FONT")
	if fontPath != "" {
		if res := loadFontResource(fontPath); res != nil {
			log.Printf("[unimap-gui] using font from UNIMAP_GUI_FONT: %s", fontPath)
			return &cjkTheme{base: theme.DefaultTheme(), regular: res}
		}
	}

	for _, p := range candidateFontPaths() {
		if res := loadFontResource(p); res != nil {
			log.Printf("[unimap-gui] using font: %s", p)
			return &cjkTheme{base: theme.DefaultTheme(), regular: res}
		}
	}

	return nil
}

func loadFontResource(path string) fyne.Resource {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return fyne.NewStaticResource(filepath.Base(path), data)
}
