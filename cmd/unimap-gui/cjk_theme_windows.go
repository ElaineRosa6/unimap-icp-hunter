//go:build gui && windows
// +build gui,windows

package main

// candidateFontPaths returns a list of local font file paths (TTF preferred) that
// commonly contain CJK glyphs on Windows.
func candidateFontPaths() []string {
	return []string{
		// Prefer TTF fonts (TTC collections are not always supported by all font parsers).
		`C:\Windows\Fonts\simhei.ttf`,
		`C:\Windows\Fonts\Deng.ttf`,
		`C:\Windows\Fonts\Dengl.ttf`,
		`C:\Windows\Fonts\Dengb.ttf`,
		`C:\Windows\Fonts\simsunb.ttf`,
		`C:\Windows\Fonts\SimsunExtG.ttf`,
		// Fallbacks (may be TTC collections)
		`C:\Windows\Fonts\msyh.ttc`,
		`C:\Windows\Fonts\simsun.ttc`,
	}
}
