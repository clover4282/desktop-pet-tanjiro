//go:build !windows

package main

import "fmt"

func main() {
	fmt.Println("Chibi Desktop Pet is a native Windows Win32 application.")
	fmt.Println("Build for Windows with:")
	fmt.Println("  GOOS=windows GOARCH=amd64 go build -ldflags=\"-H=windowsgui -s -w\" -o ChibiDesktopPet.exe ./cmd/chibipet")
}
