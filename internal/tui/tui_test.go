package tui

import "testing"

func TestAppTypeAlias(t *testing.T) {
	var _ App = App{}
}

func TestProviderControllerTypeAlias(t *testing.T) {
	var _ ProviderController = ProviderController(nil)
}
