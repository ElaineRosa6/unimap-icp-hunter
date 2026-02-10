//go:build gui && linux
// +build gui,linux

package main

// candidateFontPaths returns a list of local font file paths that commonly contain
// CJK glyphs on Linux distributions.
func candidateFontPaths() []string {
	return []string{
		// Noto CJK (common on many distros)
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf",
		"/usr/share/fonts/opentype/noto/NotoSansCJKtc-Regular.otf",
		"/usr/share/fonts/opentype/noto/NotoSansCJKjp-Regular.otf",
		// WenQuanYi
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
		// DejaVu fallback (may not cover full CJK, but helps in some setups)
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
}
