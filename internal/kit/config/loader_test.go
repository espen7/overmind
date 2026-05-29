package config

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestLoadWithoutConfigDoesNotWriteStdout(t *testing.T) {
	tempDir := t.TempDir()

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	defer readPipe.Close()

	os.Stdout = writePipe
	defer func() {
		os.Stdout = originalStdout
	}()

	_, loadErr := Load(tempDir)

	_ = writePipe.Close()
	if loadErr != nil {
		t.Fatalf("load config: %v", loadErr)
	}

	var output bytes.Buffer
	if _, err := io.Copy(&output, readPipe); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	if output.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", output.String())
	}
}
