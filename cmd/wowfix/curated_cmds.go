package main

import (
	"fmt"
	"strings"

	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/models"
)

// runCurated implements the `wowfix curated` command family:
//
//	curated list [--flavor <family>]  list the curated set for a game family
//	curated install <name>            install a curated addon from its manifest source
func runCurated(args []string) error {
	opts, flavor, rest, err := curatedArgs(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("curated requires a subcommand: list or install")
	}
	switch rest[0] {
	case "list":
		return runCuratedList(opts, flavor, rest[1:])
	case "install":
		return runCuratedInstall(opts, flavor, rest[1:])
	default:
		return fmt.Errorf("unknown curated subcommand %q (expected list or install)", rest[0])
	}
}

// curatedArgs parses the curated command's flags, which extend the
// shared set with --flavor. Flags may appear anywhere on the command
// line.
func curatedArgs(args []string) (*cliOptions, string, []string, error) {
	opts := &cliOptions{}
	var flavor string
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--flavor" || a == "-flavor":
			if i+1 >= len(args) {
				return nil, "", nil, fmt.Errorf("--flavor requires a family argument (vanilla, wrath, ...)")
			}
			i++
			flavor = args[i]
		case a == "--path" || a == "-path":
			if i+1 >= len(args) {
				return nil, "", nil, fmt.Errorf("--path requires a directory argument")
			}
			i++
			opts.path = args[i]
		case a == "--yes" || a == "-y":
			opts.yes = true
		case a == "--json" || a == "-j":
			opts.json = true
		case strings.HasPrefix(a, "-"):
			return nil, "", nil, fmt.Errorf("unknown flag %q", a)
		default:
			rest = append(rest, a)
		}
	}
	return opts, flavor, rest, nil
}

// curatedFamily resolves the family to show or install into: the
// --flavor override wins, otherwise the active profile's family.
// --flavor accepts a profile id (e.g. "turtle") or a family name
// (e.g. "vanilla").
func curatedFamily(flavor string, profile *models.Profile) string {
	if flavor != "" {
		if p := models.ProfileByID(flavor); p != nil {
			return p.Family
		}
		return strings.ToLower(strings.TrimSpace(flavor))
	}
	if profile != nil {
		return profile.Family
	}
	return ""
}

// runCuratedList prints the curated set for the active profile's
// family, or the --flavor override.
func runCuratedList(opts *cliOptions, flavor string, rest []string) error {
	if len(rest) > 0 {
		return fmt.Errorf("curated list takes no arguments")
	}
	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	family := curatedFamily(flavor, env.profile)

	m, err := catalog.LoadCurated()
	if err != nil {
		return err
	}
	set, ok := m.SetForFamily(family)
	if !ok {
		if flavor != "" {
			return fmt.Errorf("no curated set for family %q (known families: vanilla, wrath)", family)
		}
		fmt.Printf("No curated set for family %q.\n", family)
		return nil
	}

	if opts.json {
		return printJSON(set)
	}
	fmt.Printf("%s (%d addons)\n\n", set.Label, len(set.Addons))
	fmt.Printf("%-24s %-30s %s\n", "Name", "Source", "Summary")
	fmt.Println(strings.Repeat("-", 108))
	for _, a := range set.Addons {
		fmt.Printf("%-24s %-30s %s\n",
			truncateRunes(a.Name, 23),
			truncateRunes(a.Source, 29),
			truncateRunes(a.Summary, 55))
	}
	return nil
}

// runCuratedInstall resolves a curated addon by name within the
// active/--flavor family and installs it through the catalog's
// install-from-source flow.
func runCuratedInstall(opts *cliOptions, flavor string, rest []string) error {
	if len(rest) != 1 {
		return fmt.Errorf("curated install requires exactly one addon name")
	}
	name := rest[0]

	env, err := newEnvironment(opts)
	if err != nil {
		return err
	}
	if env.install == nil {
		return fmt.Errorf("no WoW installation found; use --path or set wow_path in config")
	}
	family := curatedFamily(flavor, env.profile)

	m, err := catalog.LoadCurated()
	if err != nil {
		return err
	}
	set, ok := m.SetForFamily(family)
	if !ok {
		return fmt.Errorf("no curated set for family %q (known families: vanilla, wrath)", family)
	}
	entry, ok := set.AddonByName(name)
	if !ok {
		names := make([]string, 0, len(set.Addons))
		for _, a := range set.Addons {
			names = append(names, a.Name)
		}
		return fmt.Errorf("unknown curated addon %q for family %q (valid: %s)",
			name, family, strings.Join(names, ", "))
	}

	return runInstallFromSource(env, opts, entry.Source)
}
