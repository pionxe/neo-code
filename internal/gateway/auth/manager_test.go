package auth

import (
	"encoding/json"
	"errors"
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
	expectedCustomPath, err := filepath.Abs(filepath.Clean(customPath))
	if err != nil {
		t.Fatalf("abs custom path: %v", err)
	}
	if resolvedCustomPath != expectedCustomPath {
		t.Fatalf("resolved custom path = %q, want %q", resolvedCustomPath, expectedCustomPath)
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

func TestNewManagerRecoversInvalidJSONCredential(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(credentialPath, []byte("{invalid-json"), 0o600); err != nil {
		t.Fatalf("write invalid auth file: %v", err)
	}

	manager, err := NewManager(credentialPath)
	if err != nil {
		t.Fatalf("new manager should recover invalid json: %v", err)
	}
	if strings.TrimSpace(manager.Token()) == "" {
		t.Fatal("recovered token should not be empty")
	}
}

func TestNewManagerRejectsSymbolicLinkCredentialPath(t *testing.T) {
	baseDir := t.TempDir()
	targetPath := filepath.Join(baseDir, "real-auth.json")
	if err := os.WriteFile(targetPath, []byte(`{"version":1,"token":"token-a"}`), 0o600); err != nil {
		t.Fatalf("write target auth file: %v", err)
	}

	linkPath := filepath.Join(baseDir, "auth-link.json")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Skipf("symlink unsupported in current environment: %v", err)
	}

	_, err := NewManager(linkPath)
	if err == nil {
		t.Fatal("expected symbolic link credential path to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "symbolic link") {
		t.Fatalf("error = %v, want symbolic link rejection", err)
	}
}

func TestEnsureAuthDirRejectsSymbolicLinkDirectory(t *testing.T) {
	baseDir := t.TempDir()
	realDir := filepath.Join(baseDir, "real")
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("create real dir: %v", err)
	}
	linkDir := filepath.Join(baseDir, "auth-dir-link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink unsupported in current environment: %v", err)
	}

	err := ensureAuthDir(linkDir)
	if err == nil {
		t.Fatal("expected symbolic link directory to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "symbolic link") {
		t.Fatalf("error = %v, want symbolic link rejection", err)
	}
}

func TestWriteCredentialsUsesAtomicReplaceWithoutTempLeak(t *testing.T) {
	authDir := t.TempDir()
	credentialPath := filepath.Join(authDir, "auth.json")

	first, err := buildCredentials(time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatalf("build first credentials: %v", err)
	}
	second, err := buildCredentials(time.Now().UTC())
	if err != nil {
		t.Fatalf("build second credentials: %v", err)
	}

	if err := writeCredentials(credentialPath, first); err != nil {
		t.Fatalf("write first credentials: %v", err)
	}
	if err := writeCredentials(credentialPath, second); err != nil {
		t.Fatalf("write second credentials: %v", err)
	}

	stored, err := readCredentials(credentialPath)
	if err != nil {
		t.Fatalf("read replaced credentials: %v", err)
	}
	if stored.Token != second.Token {
		t.Fatalf("token = %q, want %q", stored.Token, second.Token)
	}

	entries, err := os.ReadDir(authDir)
	if err != nil {
		t.Fatalf("read auth dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".auth.json.tmp-") {
			t.Fatalf("unexpected temp auth file leak: %s", entry.Name())
		}
	}
}

func TestReadCredentialsRejectsHardLinkOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hard-link nlink guard is unix-specific")
	}

	authDir := t.TempDir()
	credentialPath := filepath.Join(authDir, "auth.json")
	linkedPath := filepath.Join(authDir, "auth-linked.json")

	credentials, err := buildCredentials(time.Now().UTC())
	if err != nil {
		t.Fatalf("build credentials: %v", err)
	}
	if err := writeCredentials(credentialPath, credentials); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	if err := os.Link(credentialPath, linkedPath); err != nil {
		t.Fatalf("create hard link: %v", err)
	}

	if _, err := readCredentials(credentialPath); err == nil {
		t.Fatal("expected hard-linked credential file to be rejected")
	} else if !strings.Contains(strings.ToLower(err.Error()), "hard link") {
		t.Fatalf("error = %v, want hard link rejection", err)
	}
}

func TestIsRecoverableCredentialReadErrorBranches(t *testing.T) {
	if isRecoverableCredentialReadError(nil) {
		t.Fatal("nil error should not be recoverable")
	}
	if !isRecoverableCredentialReadError(os.ErrNotExist) {
		t.Fatal("os.ErrNotExist should be recoverable")
	}
	invalidBodyErr := errors.Join(errInvalidCredentialBody, errors.New("decode failed"))
	if !isRecoverableCredentialReadError(invalidBodyErr) {
		t.Fatal("invalid credential body should be recoverable")
	}
	if isRecoverableCredentialReadError(errors.New("random failure")) {
		t.Fatal("random failure should not be recoverable")
	}
}

func TestEnsureSafeCredentialDirectoryMissingPathError(t *testing.T) {
	err := ensureSafeCredentialDirectory(filepath.Join(t.TempDir(), "missing-dir"))
	if err == nil {
		t.Fatal("expected missing directory error")
	}
}

func TestEnsureSafeCredentialFilePathAllowNotExistAndSymlinkBranches(t *testing.T) {
	baseDir := t.TempDir()
	missingPath := filepath.Join(baseDir, "missing-auth.json")
	if err := ensureSafeCredentialFilePath(missingPath, true); err != nil {
		t.Fatalf("allowNotExist should accept missing file: %v", err)
	}
	if err := ensureSafeCredentialFilePath(missingPath, false); err == nil {
		t.Fatal("allowNotExist=false should reject missing file")
	}

	realPath := filepath.Join(baseDir, "real-auth.json")
	if err := os.WriteFile(realPath, []byte(`{"version":1,"token":"token-b"}`), 0o600); err != nil {
		t.Fatalf("write real auth file: %v", err)
	}
	linkPath := filepath.Join(baseDir, "auth-link-2.json")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlink unsupported in current environment: %v", err)
	}

	if err := ensureSafeCredentialFilePath(linkPath, false); err == nil {
		t.Fatal("expected symbolic link file to be rejected")
	}
}
