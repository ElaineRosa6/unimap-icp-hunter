//go:build !gui
// +build !gui

package main

import "fmt"

func main() {
	fmt.Println("This is the GUI entry point but GUI build tags were not provided.")
	fmt.Println("To run the GUI: go run -tags gui ./cmd/unimap-gui")
	fmt.Println("To run the CLI: go run ./cmd/unimap-cli")
}
