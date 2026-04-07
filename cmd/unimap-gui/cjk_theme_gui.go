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
	mono    fyne.Resource
}

func (t *cjkTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	return t.base.Color(name, variant)
}

func (t *cjkTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *cjkTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Monospace && t.mono != nil {
		return t.mono
	}
	if !style.Monospace && t.regular != nil {
		return t.regular
	}
	return t.base.Font(style)
}

func (t *cjkTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.base.Size(name)
}

func newCJKTheme() fyne.Theme {
	var regular, mono fyne.Resource

	fontPath := os.Getenv("UNIMAP_GUI_FONT")
	if fontPath != "" {
		if res := loadFontResource(fontPath); res != nil {
			log.Printf("[unimap-gui] using font from UNIMAP_GUI_FONT: %s", fontPath)
			regular = res
		}
	}

	if regular == nil {
		for _, p := range candidateFontPaths() {
			if res := loadFontResource(p); res != nil {
				log.Printf("[unimap-gui] using font: %s", p)
				regular = res
				break
			}
		}
	}

	monoPath := os.Getenv("UNIMAP_GUI_MONO_FONT")
	if monoPath != "" {
		if res := loadFontResource(monoPath); res != nil {
			log.Printf("[unimap-gui] using mono font from UNIMAP_GUI_MONO_FONT: %s", monoPath)
			mono = res
		}
	}

	if mono == nil {
		for _, p := range candidateMonoFontPaths() {
			if res := loadFontResource(p); res != nil {
				log.Printf("[unimap-gui] using mono font: %s", p)
				mono = res
				break
			}
		}
	}

	if regular == nil && mono == nil {
		return nil
	}

	if mono == nil {
		mono = regular
	}

	return &cjkTheme{base: theme.DefaultTheme(), regular: regular, mono: mono}
}

func loadFontResource(path string) fyne.Resource {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return fyne.NewStaticResource(filepath.Base(path), data)
}
