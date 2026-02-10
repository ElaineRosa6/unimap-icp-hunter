//go:build gui && darwin
// +build gui,darwin

package main

// candidateFontPaths returns a list of local font file paths that commonly contain
// CJK glyphs on macOS.
func candidateFontPaths() []string {
	return []string{
		// System fonts
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/STHeiti Light.ttc",
		"/System/Library/Fonts/STHeiti Medium.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		// User-installed fonts
		"/Library/Fonts/Arial Unicode.ttf",
		"/Library/Fonts/Arial Unicode MS.ttf",
		"/Library/Fonts/NotoSansCJK-Regular.ttc",
	}
}
