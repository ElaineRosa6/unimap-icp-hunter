//go:build gui && !windows && !linux && !darwin
// +build gui,!windows,!linux,!darwin

package main

func candidateFontPaths() []string {
	return nil
}
