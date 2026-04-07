//go:build gui && darwin
// +build gui,darwin

package main

func candidateFontPaths() []string {
	return []string{
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/STHeiti Light.ttc",
		"/System/Library/Fonts/STHeiti Medium.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/Library/Fonts/Arial Unicode.ttf",
		"/Library/Fonts/Arial Unicode MS.ttf",
		"/Library/Fonts/NotoSansCJK-Regular.ttc",
	}
}

func candidateMonoFontPaths() []string {
	return []string{
		"/System/Library/Fonts/Menlo.ttc",
		"/System/Library/Fonts/SFMono-Regular.otf",
		"/Library/Fonts/Monaco.ttf",
	}
}
