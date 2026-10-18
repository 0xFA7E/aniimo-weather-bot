//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"runtime"
)

func defaultScreenshotDirs() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".local", "share", "Steam", "userdata"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam", "userdata"),
	}
}
