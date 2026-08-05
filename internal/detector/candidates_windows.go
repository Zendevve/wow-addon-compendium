//go:build windows

package detector

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// candidateRoots returns [root, flavor] pairs to probe on Windows:
// standard install dirs, Battle.net registry keys and Steam libraries.
func candidateRoots() [][2]string {
	var out [][2]string

	gameDirs := gameDirCandidates()
	for _, dir := range gameDirs {
		out = append(out,
			[2]string{dir, FlavorRoot},
			[2]string{dir, FlavorRetail},
			[2]string{dir, FlavorClassic},
			[2]string{dir, FlavorEra},
			[2]string{dir, FlavorTBC},
		)
	}

	// Battle.net / Blizzard registry install paths.
	if p, ok := registryString(registry.LOCAL_MACHINE,
		`SOFTWARE\WOW6432Node\Blizzard Entertainment\World of Warcraft`, "InstallPath"); ok && p != "" {
		out = append(out, flavorVariants(p)...)
	}
	if p, ok := registryString(registry.LOCAL_MACHINE,
		`SOFTWARE\WOW6432Node\Blizzard Entertainment\Battle.net`, "InstallPath"); ok && p != "" {
		out = append(out, flavorVariants(p)...)
	}

	// Steam libraries from the localconfig-free libraryfolders.vdf.
	if p, ok := registryString(registry.LOCAL_MACHINE,
		`SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"); ok && p != "" {
		for _, lib := range steamLibraries(p) {
			wow := filepath.Join(lib, "steamapps", "common", "World of Warcraft")
			if _, err := os.Stat(wow); err == nil {
				out = append(out, flavorVariants(wow)...)
			}
		}
	}

	return out
}

func registryString(root registry.Key, path, name string) (string, bool) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return "", false
	}
	return v, true
}

func flavorVariants(root string) [][2]string {
	return [][2]string{
		{root, FlavorRoot},
		{root, FlavorRetail},
		{root, FlavorClassic},
		{root, FlavorEra},
		{root, FlavorTBC},
	}
}

// gameDirCandidates builds the fixed-path candidate list for the
// machine's drives.
func gameDirCandidates() []string {
	var dirs []string
	add := func(d string) {
		if d != "" {
			dirs = append(dirs, d)
		}
	}

	add(filepath.Join(os.Getenv("ProgramFiles"), "World of Warcraft"))
	add(filepath.Join(os.Getenv("ProgramFiles(x86)"), "World of Warcraft"))
	add(filepath.Join(os.Getenv("ProgramFiles(x86)"), "WowClassic"))
	add("C:\\Games\\World of Warcraft")
	add("C:\\Games\\WoW")
	add("C:\\Games\\Wow")
	add("D:\\Games\\World of Warcraft")
	add("D:\\Games\\WoW")
	add("D:\\Games\\Wow")
	add("D:\\World of Warcraft")
	add("D:\\WoW")
	add("E:\\Games\\World of Warcraft")
	add("E:\\Games\\WoW")
	add("E:\\World of Warcraft")
	return dirs
}

// steamLibraries parses libraryfolders.vdf for absolute library paths.
func steamLibraries(steamRoot string) []string {
	paths := []string{filepath.Join(steamRoot, "steamapps")}
	data, err := os.ReadFile(filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return paths
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, `"path"`) {
			continue
		}
		q1 := strings.IndexByte(line, '"')
		if q1 < 0 {
			continue
		}
		if q2 := strings.IndexByte(line[q1+1:], '"'); q2 >= 0 {
			p := line[q1+1 : q1+1+q2]
			paths = append(paths, filepath.Join(p, "steamapps"))
		}
	}
	return paths
}
