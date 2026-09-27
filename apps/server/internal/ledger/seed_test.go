package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeedPreservesExistingData(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "ledger", "main.beancount")
	if err := os.WriteFile(filepath.Join(source, "main.beancount"), []byte("sample"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Seed(target, source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Seed(target, source); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(target)
	if string(content) != "edited" {
		t.Fatal("seed replaced existing data")
	}
}
func TestSeedRejectsPartialDestination(t *testing.T) {
	target := filepath.Join(t.TempDir(), "main.beancount")
	if err := Seed(target, t.TempDir()); err == nil {
		t.Fatal("accepted a partially populated destination")
	}
	if err := Seed(target, ""); err != nil {
		t.Fatal("seeding must be opt-in")
	}
}
