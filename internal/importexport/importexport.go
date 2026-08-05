// Package importexport implements collection sharing: exporting an
// addon list as a JSON manifest or a bundle ZIP (manifest + local addon
// folders + SavedVariables), importing those back, and installing
// addons from a GitHub repo list.
package importexport

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wowfix/wowfix/internal/catalog"
	"github.com/wowfix/wowfix/internal/utils"

	"gopkg.in/yaml.v3"
)

// Manifest describes an exported addon collection.
type Manifest struct {
	Version     int             `json:"version" yaml:"version"`
	Name        string          `json:"name" yaml:"name"`
	GameVersion string          `json:"game_version,omitempty" yaml:"game_version,omitempty"`
	Addons      []ManifestAddon `json:"addons" yaml:"addons"`
}

// ManifestAddon is one addon entry of a manifest. Local-only addons
// (no Provider) travel inside the bundle zip as addons/<Folder>/.
type ManifestAddon struct {
	// Folder is the folder name as installed.
	Folder string `json:"folder" yaml:"folder"`
	// Provider is one of github|curseforge|wowinterface|tukui.
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
	// ID is the provider-scoped id ("owner/repo" for github).
	ID string `json:"id,omitempty" yaml:"id,omitempty"`
	// Source is a URL form accepted by catalog.InstallFromSource.
	Source string `json:"source,omitempty" yaml:"source,omitempty"`
	// Version is the last known installed version, informational.
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

// manifestVersion is the current manifest schema version.
const manifestVersion = 1

// ExportManifest writes the manifest as indented JSON, atomically.
func ExportManifest(name, gameVersion string, addons []ManifestAddon, outPath string) error {
	mf := Manifest{
		Version:     manifestVersion,
		Name:        name,
		GameVersion: gameVersion,
		Addons:      addons,
	}
	data, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cannot write %q: %w", outPath, err)
	}
	return nil
}

// ExportZip writes a bundle zip: manifest.json at the root, the
// SavedVariables directory as savedvars/ when present, and every
// local-only addon (no Provider) as addons/<Folder>/ resolved from
// addonsDir.
func ExportZip(name, gameVersion string, addons []ManifestAddon, addonsDir, savedvarsDir string, outPath string) error {
	mf := Manifest{
		Version:     manifestVersion,
		Name:        name,
		GameVersion: gameVersion,
		Addons:      addons,
	}
	tmp := outPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			_ = os.Remove(tmp)
		}
	}()

	zw := zip.NewWriter(f)
	if err := addZipFile(zw, "manifest.json", func(w io.Writer) error {
		data, err := json.MarshalIndent(mf, "", "  ")
		if err != nil {
			return err
		}
		_, err = w.Write(append(data, '\n'))
		return err
	}); err != nil {
		return err
	}

	if savedvarsDir != "" && utils.IsDir(savedvarsDir) {
		if err := addZipDir(zw, savedvarsDir, "savedvars"); err != nil {
			return fmt.Errorf("cannot bundle SavedVariables: %w", err)
		}
	}

	for _, a := range addons {
		if a.Provider != "" || a.Folder == "" {
			continue // remote addons are installed from their source
		}
		src := filepath.Join(addonsDir, a.Folder)
		if !utils.IsDir(src) {
			return fmt.Errorf("local addon %q not found in %s", a.Folder, addonsDir)
		}
		if err := addZipDir(zw, src, path.Join("addons", a.Folder)); err != nil {
			return fmt.Errorf("cannot bundle addon %q: %w", a.Folder, err)
		}
	}

	if err := zw.Close(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, outPath)
}

// ImportManifest reads and parses a manifest.json file.
func ImportManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read manifest %q: %w", path, err)
	}
	mf := &Manifest{}
	if err := json.Unmarshal(data, mf); err != nil {
		return nil, fmt.Errorf("manifest %q is corrupt: %w", path, err)
	}
	return mf, nil
}

// ExportManifestYAML writes the manifest as YAML, atomically.
func ExportManifestYAML(name, gameVersion string, addons []ManifestAddon, outPath string) error {
	mf := Manifest{
		Version:     manifestVersion,
		Name:        name,
		GameVersion: gameVersion,
		Addons:      addons,
	}
	data, err := yaml.Marshal(mf)
	if err != nil {
		return err
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cannot write %q: %w", outPath, err)
	}
	return nil
}

// ImportManifestAny reads and parses a manifest file, dispatching on
// extension: .yaml/.yml parse as YAML, anything else as JSON.
func ImportManifestAny(path string) (*Manifest, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read manifest %q: %w", path, err)
		}
		mf := &Manifest{}
		if err := yaml.Unmarshal(data, mf); err != nil {
			return nil, fmt.Errorf("manifest %q is corrupt: %w", path, err)
		}
		return mf, nil
	default:
		return ImportManifest(path)
	}
}

// ImportZip installs a bundle zip: manifest.json entries with a
// Provider/Source go through catalog.InstallFromSource (which needs a
// non-nil cat), local-only entries are extracted from addons/<Folder>,
// and a savedvars/ subtree is restored into
// wtfRoot/Account/<first account>/SavedVariables. It returns the
// installed folder names.
func ImportZip(zipPath, addonsDir, wtfRoot string, cat *catalog.Catalog, progress func(done, total int64)) ([]string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open %q: %w", zipPath, err)
	}
	defer r.Close()

	var mf *Manifest
	var total int64
	for _, f := range r.File {
		if err := checkZipPath(f.Name); err != nil {
			return nil, err
		}
		if f.Name == "manifest.json" && mf == nil {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			m := &Manifest{}
			if err := json.Unmarshal(data, m); err != nil {
				return nil, fmt.Errorf("bundle manifest is corrupt: %w", err)
			}
			mf = m
			continue
		}
		total += int64(f.UncompressedSize64)
	}
	if mf == nil {
		return nil, fmt.Errorf("no manifest.json in %q", zipPath)
	}

	// The catalog gate runs before any mutation: a bundle with remote
	// addons cannot be half-installed.
	for _, a := range mf.Addons {
		if a.Provider != "" || a.Source != "" {
			if cat == nil {
				return nil, errors.New("catalog required")
			}
			break
		}
	}

	tmp, err := os.MkdirTemp("", "wowfix-import-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	// Extract everything into a temp root, replaying the same progress
	// over the total uncompressed payload.
	var done int64
	for _, f := range r.File {
		if f.Name == "manifest.json" {
			continue
		}
		if err := extractZipEntry(f, tmp, func(n int64) {
			done += n
			if progress != nil {
				progress(done, total)
			}
		}); err != nil {
			return nil, err
		}
	}

	var installed []string
	for _, a := range mf.Addons {
		switch {
		case a.Provider != "" || a.Source != "":
			source := a.Source
			if source == "" {
				source = a.ID
			}
			names, err := cat.InstallFromSource(context.Background(), source, addonsDir, progress)
			if err != nil {
				return nil, fmt.Errorf("install %q: %w", a.Folder, err)
			}
			installed = append(installed, names...)
		default:
			src := filepath.Join(tmp, "addons", a.Folder)
			dst := filepath.Join(addonsDir, a.Folder)
			if !utils.IsDir(src) {
				return nil, fmt.Errorf("bundle has no local addon %q", a.Folder)
			}
			if utils.Exists(dst) {
				return nil, fmt.Errorf("cannot import %q: %q already exists", a.Folder, dst)
			}
			if err := utils.CopyDir(src, dst); err != nil {
				return nil, fmt.Errorf("cannot copy %q: %w", a.Folder, err)
			}
			installed = append(installed, a.Folder)
		}
	}

	if err := restoreSavedVars(filepath.Join(tmp, "savedvars"), wtfRoot); err != nil {
		return nil, err
	}

	return installed, nil
}

// ImportGitHubList fetches listURL (a raw gist or repo file holding one
// "owner/repo" per line) and installs each entry through the catalog.
// Lines that are empty or start with "#" are ignored. A non-nil cat is
// required as soon as the list contains at least one source.
func ImportGitHubList(listURL, addonsDir string, cat *catalog.Catalog, progress func(done, total int64)) ([]string, error) {
	resp, err := http.Get(listURL)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch %q: %w", listURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %q: HTTP %d", listURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read %q: %w", listURL, err)
	}

	var sources []string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sources = append(sources, line)
	}
	if len(sources) == 0 {
		return nil, nil
	}
	if cat == nil {
		return nil, errors.New("catalog required")
	}

	var installed []string
	for _, src := range sources {
		names, err := cat.InstallFromSource(context.Background(), src, addonsDir, progress)
		if err != nil {
			return nil, fmt.Errorf("install %q: %w", src, err)
		}
		installed = append(installed, names...)
	}
	return installed, nil
}

// restoreSavedVars copies an extracted savedvars/ tree into
// wtfRoot/Account/<first account>/SavedVariables, creating the
// directories as needed. With no existing account, "A1" is used.
func restoreSavedVars(src, wtfRoot string) error {
	if src == "" || !utils.IsDir(src) {
		return nil
	}
	account := "A1"
	acctRoot := filepath.Join(wtfRoot, "Account")
	if entries, err := os.ReadDir(acctRoot); err == nil {
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		if len(names) > 0 {
			account = names[0]
		}
	}
	dst := filepath.Join(acctRoot, account, "SavedVariables")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("cannot create %q: %w", dst, err)
	}
	return utils.CopyDir(src, dst)
}

// checkZipPath mirrors the installer's zip-slip guard.
func checkZipPath(name string) error {
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) {
		return fmt.Errorf("archive contains unsafe path %q", name)
	}
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("archive contains path traversal %q", name)
	}
	return nil
}

// extractZipEntry writes one zip entry under dst, replaying byte counts
// through progress. The entry name has already passed checkZipPath.
func extractZipEntry(f *zip.File, dst string, progress func(n int64)) error {
	clean := filepath.Clean(f.Name)
	target := filepath.Join(dst, clean)
	if !strings.HasPrefix(target, filepath.Clean(dst)+string(filepath.Separator)) {
		return fmt.Errorf("archive contains path traversal %q", f.Name)
	}
	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, rc)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if progress != nil {
		progress(n)
	}
	return nil
}

// addZipFile writes one named entry whose content comes from write.
func addZipFile(zw *zip.Writer, name string, write func(io.Writer) error) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	return write(w)
}

// addZipDir walks src and stores every file under prefix/ (recursive).
// Zip entry names always use forward slashes.
func addZipDir(zw *zip.Writer, src, prefix string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		name := prefix
		if rel != "." {
			name = prefix + "/" + rel
		}
		if d.IsDir() {
			_, err := zw.Create(name + "/")
			return err
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks and specials in bundles
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = name
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
}
