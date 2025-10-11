package backend

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
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
