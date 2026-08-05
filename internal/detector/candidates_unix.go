//go:build !windows

package detector

import (
	"os"
	"path/filepath"
)

// candidateRoots returns [root, flavor] pairs to probe on Linux and
// macOS: Wine prefixes, Lutris, Steam, Bottles and standard locations.
func candidateRoots() [][2]string {
	home, _ := os.UserHomeDir()
	var roots []string

	add := func(p string) {
		if p != "" {
			roots = append(roots, p)
		}
	}

	// Flatpak / system Steam.
	add(filepath.Join(home, ".steam", "steam", "steamapps", "common", "World of Warcraft"))
	add(filepath.Join(home, ".local", "share", "Steam", "steamapps", "common", "World of Warcraft"))
	add(filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".steam", "steam", "steamapps", "common", "World of Warcraft"))

	// Native Linux installs.
	add(filepath.Join(home, "Games", "World of Warcraft"))
	add(filepath.Join(home, "Games", "WoW"))
	add("/opt/World of Warcraft")
	add("/opt/wow")
	add("/games/World of Warcraft")

	// Wine prefixes (classic layout).
	add(filepath.Join(home, ".wine", "drive_c", "Program Files", "World of Warcraft"))
	add(filepath.Join(home, ".wine", "drive_c", "Program Files (x86)", "World of Warcraft"))
	add(filepath.Join(home, ".wine64", "drive_c", "Program Files (x86)", "World of Warcraft"))

	// Lutris.
	add(filepath.Join(home, ".local", "share", "lutris", "games", "World of Warcraft", "drive_c", "Program Files (x86)", "World of Warcraft"))
	add(filepath.Join(home, ".var", "app", "net.lutris.Lutris", "data", "lutris", "games", "World of Warcraft", "drive_c", "Program Files (x86)", "World of Warcraft"))

	// Bottles (Wine prefix manager).
	add(filepath.Join(home, ".local", "share", "bottles", "data", "wineprefixes", "World of Warcraft", "drive_c", "Program Files (x86)", "World of Warcraft"))

	// Proton prefixes inside Steam libraries.
	for _, lib := range []string{
		filepath.Join(home, ".steam", "steam", "steamapps"),
		filepath.Join(home, ".local", "share", "Steam", "steamapps"),
	} {
		entries, err := os.ReadDir(lib)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !isPrefixDir(e.Name()) {
				continue
			}
			add(filepath.Join(lib, e.Name(), "pfx", "drive_c", "Program Files (x86)", "World of Warcraft"))
			add(filepath.Join(lib, e.Name(), "pfx", "drive_c", "Program Files", "World of Warcraft"))
		}
	}

	// macOS.
	add("/Applications/World of Warcraft")
	add(filepath.Join(home, "Applications", "World of Warcraft"))
	add(filepath.Join(home, "Library", "Application Support", "CrossOver", "Bottles"))
	add("/Applications/World of Warcraft.app")

	// macOS Wine-style prefixes.
	add(filepath.Join(home, "Games", "World of Warcraft", "drive_c", "Program Files", "World of Warcraft"))

	var out [][2]string
	for _, root := range roots {
		out = append(out,
			[2]string{root, FlavorRoot},
			[2]string{root, FlavorRetail},
			[2]string{root, FlavorClassic},
			[2]string{root, FlavorEra},
			[2]string{root, FlavorTBC},
		)
	}
	return out
}

func isPrefixDir(name string) bool {
	return len(name) >= len("prefix") && name[:6] == "prefix"
}
