package infra

import (
	"os"
	"path/filepath"
)

func SaveImageToTempFile(data []byte, prefix string) (string, error) {
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, prefix+"_image.png")

	f, err := os.Create(tmpFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return "", err
	}

	return tmpFile, nil
}
