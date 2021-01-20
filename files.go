package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const fastPathCapacity = 10

func fileWatcher(config Config, extConfig ExtensionConfig) {
	newFilesChan := make(chan CheckQueueItem, 5000)
	pathDatas := make(CheckQueueItemQueue, 0, 5000)
	fastPathDatas := make(CheckQueueItemQueue, 0, fastPathCapacity)

	lastMod := time.Time{}

	deepCheckTimer := time.NewTicker(time.Duration(extConfig.CheckInterval*4) * time.Millisecond)
	newFilesCheckTimer := time.NewTicker(time.Duration(config.NewFileCheckInterval) * time.Millisecond)

	addToFastPathDatas := func(item *CheckQueueItem) {
		newFastPathDatas := make(CheckQueueItemQueue, 0, fastPathCapacity)
		for _, path := range fastPathDatas {
			if path.Path != item.Path {
				newFastPathDatas = append(newFastPathDatas, path)
			}
		}

		if len(newFastPathDatas) == cap(newFastPathDatas) {
			newFastPathDatas = append(CheckQueueItemQueue{item}, newFastPathDatas[0:cap(fastPathDatas)-1]...)
		} else {
			newFastPathDatas = append(newFastPathDatas, item)
		}

		fastPathDatas = newFastPathDatas
	}

	newFilesCheck := func(pathDatas CheckQueueItemQueue) {
		for _, watchFolder := range config.WatchFolders {
			filepath.Walk(watchFolder, func(path string, info os.FileInfo, err error) error {
				if info.IsDir() {
					path := filepath.Clean(path)
					for _, dir := range config.ExcludeDirs {
						dir = filepath.Clean(dir)
						if dir == path {
							return filepath.SkipDir
						}
					}
					return nil
				}

				for _, p := range pathDatas {
					if p.Path == path {
						return nil
					}
				}

				ext := strings.ToLower(filepath.Ext(path))
				if ext != extConfig.Ext {
					return nil
				}

				newFilesChan <- CheckQueueItem{
					LastMod:   info.ModTime(),
					Command:   extConfig.Command,
					Extension: ext,
					Path:      path,
					Restart:   extConfig.Restart,
				}

				return nil
			})
		}
	}

	go newFilesCheck(pathDatas)

	deepCheck := false

	for {
		select {
		case newFile := <-newFilesChan:
			// fmt.Println("New file found: " + newFile.Path)
			pathDatas = append(pathDatas, &newFile)
		case <-newFilesCheckTimer.C:
			fmt.Println("Checking new files")
			go newFilesCheck(pathDatas)
			fmt.Println("New files checked.")
		case <-deepCheckTimer.C:
			deepCheck = true
		default:
			usedDatas := fastPathDatas

			if deepCheck {
				usedDatas = pathDatas
			}

			for _, pathData := range usedDatas {
				data, err := os.Stat(pathData.Path)

				if err != nil {
					continue
				}

				addToFastPathDatas(pathData)

				if data.ModTime() != pathData.LastMod {
					checkQueue <- pathData
					pathData.LastMod = data.ModTime()
				}

				if lastMod.Before(data.ModTime()) {
					lastMod = data.ModTime()
				}
			}

			if deepCheck {
				// Cleanup deleted files
				for k, pathData := range pathDatas {
					if !pathExists(pathData.Path) {
						pathDatas = append(pathDatas[:k], pathDatas[k+1:]...)
						break
					}
				}

				sort.Sort(fastPathDatas)
			}

			deepCheck = false

			time.Sleep(time.Duration(extConfig.CheckInterval) * time.Millisecond)
		}
	}
}
