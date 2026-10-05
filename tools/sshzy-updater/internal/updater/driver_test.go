package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/pkg/release"
)

func archiveFixture(t *testing.T, name string, content string, kind byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressor := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressor)
	header := &tar.Header{Name: name, Mode: 0644, Size: int64(len(content)), Typeflag: kind}
	if kind != tar.TypeReg {
		header.Size = 0
	}
	if err := writer.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		writer.Write([]byte(content))
	}
	writer.Close()
	compressor.Close()
	return buffer.Bytes()
}

func TestExtractRejectsTraversalAndLinks(t *testing.T) {
	for _, name := range []string{"../secret", "/etc/config", "assets/../../secret", "assets\\..\\secret"} {
		root := t.TempDir()
		archive := filepath.Join(root, "ui.tar.gz")
		os.WriteFile(archive, archiveFixture(t, name, "payload", tar.TypeReg), 0600)
		if err := extractUI(archive, filepath.Join(root, "release")); err == nil {
			t.Fatalf("accepted unsafe archive path %q", name)
		}
	}
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeFifo} {
		root := t.TempDir()
		archive := filepath.Join(root, "ui.tar.gz")
		os.WriteFile(archive, archiveFixture(t, "assets/link", "", kind), 0600)
		if err := extractUI(archive, filepath.Join(root, "release")); err == nil {
			t.Fatal("accepted special archive entry")
		}
	}
}

func TestReplaceBackendImagePreservesOtherSettings(t *testing.T) {
	input := []byte("services:\n  sub2api:\n    image: local/sub2api-batch:0.2.13-r1\n    ports: ['127.0.0.1:8080:8080']\n    environment:\n      JWT_SECRET: ${JWT_SECRET}\n  redis:\n    image: redis:8-alpine\n")
	next := release.Image + "@sha256:" + strings.Repeat("a", 64)
	output, err := replaceBackendImage(input, next)
	if err != nil || !bytes.Contains(output, []byte(next)) || !bytes.Contains(output, []byte("${JWT_SECRET}")) || !bytes.Contains(output, []byte("127.0.0.1:8080:8080")) {
		t.Fatalf("image replacement failed: %v", err)
	}
	if _, err := replaceBackendImage([]byte("services: {other: {image: postgres}}"), next); err == nil {
		t.Fatal("accepted missing backend service")
	}
}

func TestReplaceBackendImagePreservesFormatting(t *testing.T) {
	previous := "local/sub2api-batch:0.2.13-r2"
	next := release.Image + "@sha256:" + strings.Repeat("b", 64)
	input := []byte("# deployment header\n" +
		"services:\n" +
		"  sub2api:\n" +
		"    image: " + previous + "\n" +
		"    ports: ['127.0.0.1:8080:8080']\n" +
		"    environment:\n" +
		"      JWT_SECRET: ${JWT_SECRET}\n" +
		"  redis:\n" +
		"    image: redis:8-alpine\n" +
		"    # every line needs a trailing backslash\n" +
		"    command: >\n" +
		"        sh -c '\n" +
		"          redis-server \\\n" +
		"          --save 60 1 \\\n" +
		"          --appendonly yes'\n")
	output, err := replaceBackendImage(input, next)
	if err != nil {
		t.Fatalf("replaceBackendImage failed: %v", err)
	}
	expected := bytes.Replace(input, []byte(previous), []byte(next), 1)
	if !bytes.Equal(output, expected) {
		t.Fatalf("replacement changed unrelated formatting:\n--- got ---\n%s\n--- want ---\n%s", output, expected)
	}
	if !bytes.Contains(output, []byte("redis:8-alpine")) || !bytes.Contains(output, []byte("--appendonly yes")) {
		t.Fatal("redis service block was altered")
	}

	quoted := []byte("services:\n  sub2api:\n    image: \"" + previous + "\"\n  redis:\n    image: redis:8-alpine\n")
	output, err = replaceBackendImage(quoted, next)
	if err != nil || !bytes.Contains(output, []byte("\""+next+"\"")) || !bytes.Contains(output, []byte("redis:8-alpine")) {
		t.Fatalf("quoted image replacement failed: %v", err)
	}
}
