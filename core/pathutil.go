package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindModuleRoot walks parent directories starting from start and returns the directory containing go.mod.
func FindModuleRoot(start string) (string, error) {
	if start == "" {
		return "", fmt.Errorf("empty start path")
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	dir := abs
	for {
		modPath := filepath.Join(dir, "go.mod")
		if _, statErr := os.Stat(modPath); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found above %s", start)
}
