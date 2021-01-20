package main

import "os"

func isFile(path string) bool {
	stat, err := os.Stat(path)

	if err != nil {
		return false
	}

	return !stat.IsDir()
}

func isFolder(path string) bool {
	stat, err := os.Stat(path)

	if err != nil {
		return false
	}

	return stat.IsDir()
}

func pathExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

func folderExists(path string) bool {
	return isFolder(path)
}

func fileExists(path string) bool {
	return isFile(path)
}
