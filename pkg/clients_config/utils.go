package clients_config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// getUserHomeDir returns the user's home directory.
func getUserHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %v", err)
	}
	return home, nil
}

// backupFile creates a copy of the target file appending ".bak".
// If the target file doesn't exist, it does nothing and returns nil.
func backupFile(targetPath string) error {
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return nil // Nothing to backup
	}

	backupPath := targetPath + ".bak"
	return copyFile(targetPath, backupPath)
}

// restoreFile copies the backup file (".bak") back to the target file.
// If the backup file doesn't exist, it returns an error.
func restoreFile(targetPath string) error {
	backupPath := targetPath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}

	err := copyFile(backupPath, targetPath)
	if err != nil {
		return err
	}

	// Option: remove backup after restore?
	// We'll keep it just in case, but usually we don't delete immediately.
	return nil
}

// hasBackup checks if the backup file (".bak") exists.
func hasBackup(targetPath string) bool {
	backupPath := targetPath + ".bak"
	_, err := os.Stat(backupPath)
	return err == nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %v", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %v", dst, err)
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy data from %s to %s: %v", src, dst, err)
	}
	return nil
}

// ensureDirExists ensures that the directory for the given path exists.
func ensureDirExists(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}

// getAppDataDir gets the application data directory based on OS.
func getAppDataDir(appName string) (string, error) {
	home, err := getUserHomeDir()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, appName), nil
	}

	// macOS and Linux generally use ~/.config/appName
	return filepath.Join(home, ".config", appName), nil
}
