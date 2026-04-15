package infra

import (
	"os"
)

func SaveImageToTempFile(data []byte, prefix string) (string, error) {
	pattern := "image-*.png"
	if cleaned := sanitizeTempPrefix(prefix); cleaned != "" {
		pattern = cleaned + "-*.png"
	}
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	tmpFile := f.Name()

	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpFile)
		return "", err
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(tmpFile)
		return "", err
	}

	return tmpFile, nil
}

// sanitizeTempPrefix 过滤临时文件名前缀中的不安全字符，避免路径注入与非法命名。
func sanitizeTempPrefix(prefix string) string {
	if prefix == "" {
		return ""
	}

	buf := make([]rune, 0, len(prefix))
	for _, r := range prefix {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			buf = append(buf, r)
		}
	}
	return string(buf)
}
