package ledger

import (
	"fmt"
	"os"
	"path/filepath"
)

// Seed copies a trusted sample directory once. It never replaces an existing
// ledger or repairs a partially populated directory by overwriting its files.
func Seed(path, source string) error {
	if source == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	target := filepath.Dir(path)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("seed destination already exists without a ledger: %s", target)
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(target)
	stage, err := os.MkdirTemp(parent, ".ledger-seed-")
	if err != nil {
		return fmt.Errorf("create seed staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := os.CopyFS(stage, os.DirFS(source)); err != nil {
		return fmt.Errorf("copy sample ledger: %w", err)
	}
	if _, err := os.Stat(filepath.Join(stage, filepath.Base(path))); err != nil {
		return fmt.Errorf("sample root ledger missing: %w", err)
	}
	if err := os.Rename(stage, target); err != nil {
		// A concurrent first start may have initialized the directory already.
		if _, existing := os.Stat(path); existing == nil {
			return nil
		}
		return fmt.Errorf("install sample ledger: %w", err)
	}
	return nil
}
