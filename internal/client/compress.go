package client

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CompressFiles takes a list of file paths and compresses them into a single zip file.
func CompressFiles(filepaths []string, destZip string) error {
	// Create the zip file
	outFile, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	for _, path := range filepaths {
		err := zipPath(zipWriter, path, "")
		if err != nil {
			return err
		}
	}

	return nil
}

// zipPath compresses a single file or directory into the zip writer.
// It keeps the directory structure when adding files from a directory.
func zipPath(zipWriter *zip.Writer, path, base string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	// If it's a directory, walk through it and compress all files inside it
	if info.IsDir() {
		files, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, file := range files {
			newPath := filepath.Join(path, file.Name())
			err := zipPath(zipWriter, newPath, filepath.Join(base, info.Name()))
			if err != nil {
				return err
			}
		}
		return nil
	}

	// It's a file
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create a zip header using the relative path
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.Join(base, info.Name())
	header.Method = zip.Deflate // compression

	// Create writer for this file inside zip
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	// Copy file data into zip
	_, err = io.Copy(writer, file)
	return err
}

// CollectFileTree returns a list of all file paths that will be included in the zip.
// Each path includes the relative directory structure (e.g., "dir/subdir/file.txt").
func CollectFileTree(filepaths []string) ([]string, error) {
	var tree []string
	for _, path := range filepaths {
		if err := collectPath(&tree, path, ""); err != nil {
			return nil, err
		}
	}
	return tree, nil
}

func collectPath(tree *[]string, path, base string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		files, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, file := range files {
			newPath := filepath.Join(path, file.Name())
			if err := collectPath(tree, newPath, filepath.Join(base, info.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	// It's a file - add to tree with its relative path
	*tree = append(*tree, filepath.Join(base, info.Name()))
	return nil
}

// DecompressPath extracts a single file or an entire sub-directory from zipFile
// into destDir. targetPath is the entry path inside the archive using forward
// slashes (e.g. "src/utils/helper.go" or "src/utils").
//
// When targetPath names a directory the extracted tree is rooted at that
// directory name inside destDir, matching the behaviour of most archive tools:
//
//	DecompressPath(z, "src/utils", "./out")  →  ./out/utils/helper.go
func DecompressPath(zipFile, targetPath, destDir string) error {
	reader, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer reader.Close()

	if destDir == "" {
		destDir = "."
	}

	targetPath = filepath.ToSlash(filepath.Clean(targetPath))

	// When extracting a directory we strip everything up to (but not including)
	// the target directory component so the target dir appears in destDir.
	parent := filepath.ToSlash(filepath.Dir(targetPath))
	var stripPrefix string
	if parent != "." {
		stripPrefix = parent + "/"
	}

	matched := false
	for _, f := range reader.File {
		normalized := filepath.ToSlash(filepath.Clean(f.Name))

		switch {
		case normalized == targetPath && !f.FileInfo().IsDir():
			// Exact single-file match.
			matched = true
			if err := os.MkdirAll(destDir, 0755); err != nil {
				return err
			}
			if err := extractZipEntry(f, filepath.Join(destDir, filepath.Base(f.Name))); err != nil {
				return err
			}

		case normalized == targetPath && f.FileInfo().IsDir(),
			strings.HasPrefix(normalized, targetPath+"/"):
			// The entry is the target directory itself or lives inside it.
			matched = true
			relPath := filepath.FromSlash(normalized[len(stripPrefix):])
			dest := filepath.Join(destDir, relPath)
			if f.FileInfo().IsDir() {
				if err := os.MkdirAll(dest, f.Mode()); err != nil {
					return err
				}
			} else {
				if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
					return err
				}
				if err := extractZipEntry(f, dest); err != nil {
					return err
				}
			}
		}
	}

	if !matched {
		return fmt.Errorf("path %q not found in archive", targetPath)
	}
	return nil
}

func extractZipEntry(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		rc.Close()
		return err
	}
	_, copyErr := io.Copy(out, rc)
	rc.Close()
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// Decompress extracts a zip file into the specified destination directory.
// If not specified, it extracts to the current directory.
func Decompress(zipFile, destDir string) error {
	// Open the zip file
	reader, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Use current directory if destDir is not specified
	if destDir == "" {
		destDir = "."
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// Extract each file
	for _, file := range reader.File {
		// Determine the target path
		targetPath := filepath.Join(destDir, file.Name)

		// Create directory if the file is a directory
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, file.Mode()); err != nil {
				return err
			}
			continue
		}

		// Ensure the directory exists for the file
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		// Open the file from the archive
		fileReader, err := file.Open()
		if err != nil {
			return err
		}

		// Create the target file
		targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			fileReader.Close()
			return err
		}

		// Copy the file contents
		_, err = io.Copy(targetFile, fileReader)

		// Close both files
		fileReader.Close()
		cerr := targetFile.Close()

		// Return the copy error if any
		if err != nil {
			return err
		}
		if cerr != nil {
			return cerr
		}
	}

	return nil
}
