//go:build !windows && !darwin

package infra

import (
	"errors"

	clipboardtext "github.com/atotto/clipboard"
)

var errClipboardImageUnsupported = errors.New("clipboard image is not supported on this platform")

func CopyText(text string) error {
	return clipboardtext.WriteAll(text)
}

func ReadClipboardText() (string, error) {
	return clipboardtext.ReadAll()
}

func ReadClipboardImage() ([]byte, error) {
	return nil, errClipboardImageUnsupported
}
