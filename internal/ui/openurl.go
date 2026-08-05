package ui

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openURL opens a URL in the user's default browser.
func openURL(url string) error {
	if url == "" {
		return fmt.Errorf("empty URL")
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
