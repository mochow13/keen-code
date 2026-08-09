package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeFileAtomic(path string, data []byte) error {
	target, err := resolveWriteTarget(path)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	mode := os.FileMode(0644)
	if fi, err := os.Lstat(target); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to set permissions on temporary file: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write failed: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync failed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close failed: %w", err)
	}

	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	return nil
}

func resolveWriteTarget(path string) (string, error) {
	current := path
	for range 255 {
		fi, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return current, nil
			}
			return "", fmt.Errorf("failed to stat %q: %w", current, err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return current, nil
		}
		target, err := os.Readlink(current)
		if err != nil {
			return "", fmt.Errorf("failed to read symlink %q: %w", current, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		current = target
	}
	return "", fmt.Errorf("too many levels of symbolic links resolving %q", path)
}
