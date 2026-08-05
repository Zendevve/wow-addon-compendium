package savedvars

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowfix/wowfix/internal/logger"
)

// newTestManager builds a Manager over a fresh WTF root with the given
// account names, each populated with the given SavedVariables stems.
func newTestManager(t *testing.T, accounts map[string][]string) (*Manager, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "WTF")
	m := New(root, logger.New(50))
	for acct, stems := range accounts {
		dir := filepath.Join(root, "Account", acct, "SavedVariables")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, stem := range stems {
			if err := os.WriteFile(filepath.Join(dir, stem+".lua"), []byte("SV = {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return m, root
}

func TestAccountsSorted(t *testing.T) {
	m, _ := newTestManager(t, map[string][]string{
		"zeta":  {"X"},
		"Alpha": {"X"},
		"beta":  {"X"},
	})
	got := m.Accounts()
	want := []string{"Alpha", "beta", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("accounts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("accounts = %v, want %v", got, want)
		}
	}
}

func TestAccountsEmpty(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), "WTF"), nil)
	if got := m.Accounts(); len(got) != 0 {
		t.Fatalf("expected no accounts, got %v", got)
	}
}

func TestListMultiAccount(t *testing.T) {
	m, _ := newTestManager(t, map[string][]string{
		"A1": {"Questie", "DBM", "dbm-core"},
		"A2": {"WeakAuras"},
	})
	files, err := m.List("A1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"DBM", "Questie", "dbm-core"}
	if len(files) != len(want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("files = %v, want %v", files, want)
		}
	}
	if files[0] != "DBM" || files[1] != "Questie" {
		t.Fatalf("files must be sorted: %v", files)
	}
	// A2 is unaffected.
	files, err = m.List("A2")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "WeakAuras" {
		t.Fatalf("A2 files = %v", files)
	}
	// Missing account yields an empty list, not an error.
	files, err = m.List("nobody")
	if err != nil || len(files) != 0 {
		t.Fatalf("missing account: files=%v err=%v", files, err)
	}
}

func TestListRejectsTraversalAccount(t *testing.T) {
	m, _ := newTestManager(t, map[string][]string{"A1": {"X"}})
	if _, err := m.List(".."); err == nil {
		t.Fatal("account '..' must be rejected")
	}
	if _, err := m.List("..\\..\\outside"); err == nil {
		t.Fatal("traversal account must be rejected")
	}
}

func TestBackupCreatesTimestampedCopy(t *testing.T) {
	m, _ := newTestManager(t, map[string][]string{"A1": {"Questie", "DBM"}})
	dest := filepath.Join(t.TempDir(), "dest")
	path, err := m.Backup("A1", dest)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(dest, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("backup path %q not under dest %q", path, dest)
	}
	for _, stem := range []string{"Questie.lua", "DBM.lua"} {
		if _, err := os.Stat(filepath.Join(path, stem)); err != nil {
			t.Fatalf("backup missing %s: %v", stem, err)
		}
	}
	// A second backup in the same destination must not collide.
	path2, err := m.Backup("A1", dest)
	if err != nil {
		t.Fatal(err)
	}
	if path == path2 {
		t.Fatalf("two backups collided at %s", path)
	}
}

func TestBackupRequiresSavedVariables(t *testing.T) {
	root := filepath.Join(t.TempDir(), "WTF")
	if err := os.MkdirAll(filepath.Join(root, "Account", "A1"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := New(root, nil)
	if _, err := m.Backup("A1", t.TempDir()); err == nil {
		t.Fatal("backup without a SavedVariables dir must error")
	}
}

func TestRestoreReplacesAndSnapshotsFirst(t *testing.T) {
	m, root := newTestManager(t, map[string][]string{"A1": {"Questie"}})
	// The backup destination must live under Root: restore refuses
	// paths outside the WTF directory.
	backupPath, err := m.Backup("A1", filepath.Join(root, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	// Mutate the live state after backing up.
	if err := os.WriteFile(filepath.Join(root, "Account", "A1", "SavedVariables", "Questie.lua"),
		[]byte("SV = {changed = true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Restore("A1", backupPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Account", "A1", "SavedVariables", "Questie.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "changed") {
		t.Fatalf("restore did not bring the backup content back: %q", data)
	}
	// A pre-restore snapshot of the mutated state must exist next to
	// the backup.
	entries, err := os.ReadDir(filepath.Dir(backupPath))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "-prerestore") {
			found = true
			pre, err := os.ReadFile(filepath.Join(filepath.Dir(backupPath), e.Name(), "Questie.lua"))
			if err != nil || !strings.Contains(string(pre), "changed") {
				t.Fatalf("pre-restore snapshot missing the mutated state: %q err=%v", pre, err)
			}
		}
	}
	if !found {
		t.Fatal("no pre-restore snapshot created")
	}
}

func TestRestoreRejectsOutsideRoot(t *testing.T) {
	m, root := newTestManager(t, map[string][]string{"A1": {"X"}})
	outside := filepath.Join(t.TempDir(), "not-under-root")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Restore("A1", outside); err == nil {
		t.Fatal("restore of a path outside Root must be refused")
	}
	// Sanity: the live state is untouched.
	if _, err := os.Stat(filepath.Join(root, "Account", "A1", "SavedVariables", "X.lua")); err != nil {
		t.Fatalf("refused restore must not mutate state: %v", err)
	}
}

func TestResetDeletesExactStemOnly(t *testing.T) {
	m, root := newTestManager(t, map[string][]string{"A1": {"DBM", "DBM-Core", "dbm", "WeakAuras"}})
	if err := m.Reset("A1", "DBM"); err != nil {
		t.Fatal(err)
	}
	sv := filepath.Join(root, "Account", "A1", "SavedVariables")
	if _, err := os.Stat(filepath.Join(sv, "DBM.lua")); !os.IsNotExist(err) {
		t.Fatalf("DBM.lua must be deleted (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(sv, "dbm.lua")); !os.IsNotExist(err) {
		t.Fatalf("dbm.lua must be deleted case-insensitively (err=%v)", err)
	}
	// Exact-stem match only: prefix matches survive.
	for _, keep := range []string{"DBM-Core.lua", "WeakAuras.lua"} {
		if _, err := os.Stat(filepath.Join(sv, keep)); err != nil {
			t.Fatalf("%s must survive: %v", keep, err)
		}
	}
}

func TestResetRefusesTraversal(t *testing.T) {
	m, _ := newTestManager(t, map[string][]string{"A1": {"X"}})
	if err := m.Reset("A1", "../x"); err == nil {
		t.Fatal("traversal addon name must be rejected")
	}
	if err := m.Reset("A1", "..\\..\\x"); err == nil {
		t.Fatal("traversal addon name must be rejected")
	}
	if err := m.Reset("..", "X"); err == nil {
		t.Fatal("traversal account must be rejected")
	}
}

func TestResetUnknownAddonErrors(t *testing.T) {
	m, _ := newTestManager(t, map[string][]string{"A1": {"X"}})
	if err := m.Reset("A1", "nope"); err == nil {
		t.Fatal("resetting an unknown addon must error")
	}
}
