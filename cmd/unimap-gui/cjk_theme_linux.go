//go:build gui && linux
// +build gui,linux

package main

func candidateFontPaths() []string {
	return []string{
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf",
		"/usr/share/fonts/opentype/noto/NotoSansCJKtc-Regular.otf",
		"/usr/share/fonts/opentype/noto/NotoSansCJKjp-Regular.otf",
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
}

func candidateMonoFontPaths() []string {
	return []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/opentype/noto/NotoSansMono-Regular.ttf",
	}
}
