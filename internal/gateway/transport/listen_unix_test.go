//go:build !windows

package transport

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListenUnixAcceptsConnectionAndCleansSocket(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), "gateway.sock")
	socketDir := filepath.Dir(socketPath)
	listener, err := Listen(socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	acceptDone := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			acceptDone <- acceptErr
			return
		}
		_ = conn.Close()
		acceptDone <- nil
	}()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial unix socket: %v", err)
	}
	_ = conn.Close()

	socketInfo, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket file: %v", err)
	}
	if got := socketInfo.Mode() & os.ModePerm; got != unixSocketFilePerm {
		t.Fatalf("socket file perm = %#o, want %#o", got, unixSocketFilePerm)
	}

	dirInfo, err := os.Stat(socketDir)
	if err != nil {
		t.Fatalf("stat socket dir: %v", err)
	}
	if got := dirInfo.Mode() & os.ModePerm; got != unixSocketDirPerm {
		t.Fatalf("socket dir perm = %#o, want %#o", got, unixSocketDirPerm)
	}

	select {
	case acceptErr := <-acceptDone:
		if acceptErr != nil {
			t.Fatalf("accept connection: %v", acceptErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("accept timed out")
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("socket file should be removed on close, stat err: %v", err)
	}
}
