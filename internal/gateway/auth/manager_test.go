package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewManagerCreatesCredentialFile(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "auth.json")

	manager, err := NewManager(credentialPath)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if manager.Token() == "" {
		t.Fatal("token should not be empty")
	}

	info, err := os.Stat(credentialPath)
	if err != nil {
		t.Fatalf("stat auth file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != authFilePerm {
		t.Fatalf("file perm = %o, want %o", info.Mode().Perm(), authFilePerm)
	}
}

func TestNewManagerReusesValidCredential(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	first, err := NewManager(credentialPath)
	if err != nil {
		t.Fatalf("first manager: %v", err)
	}

	second, err := NewManager(credentialPath)
	if err != nil {
		t.Fatalf("second manager: %v", err)
	}
	if second.Token() != first.Token() {
		t.Fatalf("token mismatch: %q != %q", second.Token(), first.Token())
	}
}

func TestNewManagerRecoversInvalidCredential(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(credentialPath, []byte(`{"version":1,"token":""}`), 0o600); err != nil {
		t.Fatalf("write invalid auth file: %v", err)
	}

	manager, err := NewManager(credentialPath)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if manager.Token() == "" {
		t.Fatal("recovered token should not be empty")
	}
}

func TestLoadTokenFromFile(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	manager, err := NewManager(credentialPath)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	token, err := LoadTokenFromFile(credentialPath)
	if err != nil {
		t.Fatalf("load token: %v", err)
	}
	if token != manager.Token() {
		t.Fatalf("token = %q, want %q", token, manager.Token())
	}
}

func TestLoadTokenFromFileInvalidJSON(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(credentialPath, []byte("{bad-json"), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	if _, err := LoadTokenFromFile(credentialPath); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestValidateToken(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	manager, err := NewManager(credentialPath)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if !manager.ValidateToken(manager.Token()) {
		t.Fatal("expected valid token")
	}
	if manager.ValidateToken("wrong-token") {
		t.Fatal("expected invalid token")
	}
}

func TestCredentialFileSchema(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	_, err := NewManager(credentialPath)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	raw, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}
	var credentials map[string]any
	if err := json.Unmarshal(raw, &credentials); err != nil {
		t.Fatalf("decode auth file: %v", err)
	}
	for _, key := range []string{"version", "token", "created_at", "updated_at"} {
		if _, exists := credentials[key]; !exists {
			t.Fatalf("missing key %q", key)
		}
	}
}

func TestManagerNilReceiverHelpers(t *testing.T) {
	var manager *Manager
	if manager.Path() != "" {
		t.Fatalf("nil manager path = %q, want empty", manager.Path())
	}
	if manager.Token() != "" {
		t.Fatalf("nil manager token = %q, want empty", manager.Token())
	}
	if manager.ValidateToken("any") {
		t.Fatal("nil manager should reject all tokens")
	}
}

func TestResolveAuthPathAndEnsureDirError(t *testing.T) {
	customPath := filepath.Join(t.TempDir(), "custom-auth.json")
	resolvedCustomPath, err := resolveAuthPath(customPath)
	if err != nil {
		t.Fatalf("resolve custom path: %v", err)
	}
	if resolvedCustomPath != filepath.Clean(customPath) {
		t.Fatalf("resolved custom path = %q, want %q", resolvedCustomPath, filepath.Clean(customPath))
	}

	baseDir := t.TempDir()
	notDirectoryPath := filepath.Join(baseDir, "not-dir")
	if err := os.WriteFile(notDirectoryPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write not-dir file: %v", err)
	}
	if _, err := NewManager(filepath.Join(notDirectoryPath, "auth.json")); err == nil {
		t.Fatal("expected create dir error")
	}
}

func TestLoadTokenFromFileEmptyToken(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(credentialPath, []byte(`{"version":1,"token":""}`), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	_, err := LoadTokenFromFile(credentialPath)
	if err == nil || !strings.Contains(err.Error(), "token is empty") {
		t.Fatalf("expected empty token error, got %v", err)
	}
}

func TestBuildCredentialsAndValidation(t *testing.T) {
	credentials, err := buildCredentials(time.Now().UTC())
	if err != nil {
		t.Fatalf("build credentials: %v", err)
	}
	if credentials.Token == "" {
		t.Fatal("token should not be empty")
	}
	if !isValidCredentials(credentials) {
		t.Fatal("generated credentials should be valid")
	}
	if isValidCredentials(Credentials{Version: 0, Token: "abc"}) {
		t.Fatal("version below schema should be invalid")
	}
	if isValidCredentials(Credentials{Version: 1, Token: "   "}) {
		t.Fatal("blank token should be invalid")
	}
}

func TestDefaultAuthPathAndLoadOrCreateNilManager(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	defaultPath, err := DefaultAuthPath()
	if err != nil {
		t.Fatalf("default auth path: %v", err)
	}
	expectedPath := filepath.Join(tempHome, DefaultAuthRelativePath)
	if defaultPath != expectedPath {
		t.Fatalf("default auth path = %q, want %q", defaultPath, expectedPath)
	}

	manager, err := NewManager("")
	if err != nil {
		t.Fatalf("new manager with default path: %v", err)
	}
	if manager.Path() != expectedPath {
		t.Fatalf("manager path = %q, want %q", manager.Path(), expectedPath)
	}

	token, err := LoadTokenFromFile("")
	if err != nil {
		t.Fatalf("load token from default path: %v", err)
	}
	if token != manager.Token() {
		t.Fatalf("token = %q, want %q", token, manager.Token())
	}

	var nilManager *Manager
	if err := nilManager.loadOrCreate(); err == nil {
		t.Fatal("expected nil manager loadOrCreate error")
	}
}

func TestManagerGenerateTokenAndWriteCredentialsErrorBranches(t *testing.T) {
	t.Run("write credentials fails on directory path", func(t *testing.T) {
		dir := t.TempDir()
		err := writeCredentials(dir, Credentials{Version: 1, Token: "token"})
		if err == nil || !strings.Contains(err.Error(), "write auth file") {
			t.Fatalf("expected write auth file error, got %v", err)
		}
	})
}
