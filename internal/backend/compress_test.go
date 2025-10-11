package backend

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Helper function to create test files
func createTestFiles(t *testing.T, dir string) []string {
	t.Helper()

	testFiles := []struct {
		name    string
		content string
	}{
		{"file1.txt", "This is the content of file 1"},
		{"file2.txt", "This is the content of file 2"},
	}

	var filePaths []string
	for _, testFile := range testFiles {
		filePath := filepath.Join(dir, testFile.name)
		filePaths = append(filePaths, filePath)
		if err := os.WriteFile(filePath, []byte(testFile.content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", testFile.name, err)
		}
	}
	return filePaths
}

// Test function for compression
func testCompress(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "compress-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePaths := createTestFiles(t, tempDir)

	zipFile := filepath.Join(tempDir, "test.zip")
	if err := CompressFiles(filePaths, zipFile); err != nil {
		t.Fatalf("CompressFiles failed: %v", err)
	}

	if _, err := os.Stat(zipFile); os.IsNotExist(err) {
		t.Fatalf("Zip file was not created: %s", zipFile)
	}

	reader, err := zip.OpenReader(zipFile)
	if err != nil {
		t.Fatalf("Failed to open zip file: %v", err)
	}
	defer reader.Close()

	expectedFiles := map[string]string{
		"file1.txt": "This is the content of file 1",
		"file2.txt": "This is the content of file 2",
	}

	for _, zipFile := range reader.File {
		filename := filepath.Base(zipFile.Name)
		expectedContent, exists := expectedFiles[filename]
		if !exists {
			t.Errorf("Unexpected file in zip: %s", filename)
			continue
		}

		fileReader, err := zipFile.Open()
		if err != nil {
			t.Errorf("Failed to open %s in zip: %v", filename, err)
			continue
		}

		content, err := io.ReadAll(fileReader)
		fileReader.Close()
		if err != nil {
			t.Errorf("Failed to read %s from zip: %v", filename, err)
			continue
		}

		if string(content) != expectedContent {
			t.Errorf("Content mismatch for %s: expected %q, got %q", filename, expectedContent, string(content))
		}

		delete(expectedFiles, filename)
	}

	if len(expectedFiles) > 0 {
		for filename := range expectedFiles {
			t.Errorf("Expected file %s not found in zip", filename)
		}
	}
}

// Test function for decompression
func testDecompress(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "decompress-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePaths := createTestFiles(t, tempDir)

	zipFile := filepath.Join(tempDir, "test.zip")
	if err := CompressFiles(filePaths, zipFile); err != nil {
		t.Fatalf("Failed to create test zip file: %v", err)
	}

	extractDir := filepath.Join(tempDir, "extracted")
	if err := os.Mkdir(extractDir, 0755); err != nil {
		t.Fatalf("Failed to create extract directory: %v", err)
	}

	if err := Decompress(zipFile, extractDir); err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	for _, testFile := range []struct {
		name    string
		content string
	}{
		{"file1.txt", "This is the content of file 1"},
		{"file2.txt", "This is the content of file 2"},
	} {
		extractedPath := filepath.Join(extractDir, testFile.name)
		extractedContent, err := os.ReadFile(extractedPath)
		if err != nil {
			t.Errorf("Failed to read extracted file %s: %v", testFile.name, err)
			continue
		}

		if string(extractedContent) != testFile.content {
			t.Errorf("Content mismatch for %s: expected %q, got %q", testFile.name, testFile.content, string(extractedContent))
		}
	}
}

// TestCompressAndDecompress runs both the compression and decompression tests
func TestCompressAndDecompress(t *testing.T) {
	t.Run("Compression", testCompress)
	t.Run("Decompression", testDecompress)
}
