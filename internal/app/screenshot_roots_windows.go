//go:build windows

package app

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

func defaultScreenshotDirs() []string {
	var roots []string
	addInstall := func(install string) {
		if install != "" {
			roots = append(roots, filepath.Join(filepath.FromSlash(install), "userdata"))
		}
	}
	if base := os.Getenv("ProgramFiles(x86)"); base != "" {
		addInstall(filepath.Join(base, "Steam"))
	}
	if base := os.Getenv("ProgramFiles"); base != "" {
		addInstall(filepath.Join(base, "Steam"))
	}

	for _, source := range []struct {
		root registry.Key
		path string
		view uint32
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, 0},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, registry.WOW64_32KEY},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, registry.WOW64_64KEY},
	} {
		key, err := registry.OpenKey(source.root, source.path, registry.QUERY_VALUE|source.view)
		if err != nil {
			continue
		}
		if value, _, err := key.GetStringValue("SteamPath"); err == nil {
			addInstall(value)
		} else if value, _, err := key.GetStringValue("InstallPath"); err == nil {
			addInstall(value)
		}
		_ = key.Close()
	}
	return mergePaths(nil, roots)
}
