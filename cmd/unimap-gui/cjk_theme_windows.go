//go:build gui && windows
// +build gui,windows

package main

func candidateFontPaths() []string {
	return []string{
		`C:\Windows\Fonts\simhei.ttf`,
		`C:\Windows\Fonts\Deng.ttf`,
		`C:\Windows\Fonts\Dengl.ttf`,
		`C:\Windows\Fonts\Dengb.ttf`,
		`C:\Windows\Fonts\simsunb.ttf`,
		`C:\Windows\Fonts\SimsunExtG.ttf`,
		`C:\Windows\Fonts\msyh.ttc`,
		`C:\Windows\Fonts\simsun.ttc`,
	}
}

func candidateMonoFontPaths() []string {
	return []string{
		`C:\Windows\Fonts\consola.ttf`,
		`C:\Windows\Fonts\CascadiaMono.ttf`,
		`C:\Windows\Fonts\JetBrainsMono-Regular.ttf`,
		`C:\Windows\Fonts\FiraCode-Regular.ttf`,
	}
}
