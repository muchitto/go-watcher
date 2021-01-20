package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/profile"
)

const (
	DefaultExtensionCheckInterval = 500
	ConfigDirName                 = ".watcher"
)

type CheckQueueItem struct {
	Path      string
	Extension string
	Command   string
	Restart   bool
	LastMod   time.Time
}

type CheckQueueItemQueue []*CheckQueueItem

func (c CheckQueueItemQueue) Len() int {
	return len(c)
}

func (c CheckQueueItemQueue) Less(i, j int) bool {
	return c[i].LastMod.Before(c[j].LastMod)
}

func (c CheckQueueItemQueue) Swap(i, j int) {
	f := c[i]
	c[i] = c[j]
	c[j] = f
}

type ExtensionConfig struct {
	Ext           string
	Command       string
	CheckInterval int
	Restart       bool
}

type RunParallelConfig struct {
	WaitBeforeBegin bool
	Command         string
	Name            string
}

type Config struct {
	Extensions           []ExtensionConfig
	RunParallel          []RunParallelConfig
	ExcludeDirs          []string
	WatchFolders         []string
	NewFileCheckInterval int
	FileCleanupInterval  int
}

var checkQueue = make(chan *CheckQueueItem, 5)

var profileCode = flag.String("profile", "", "write profile to file")

var watcherStateChannel = make(chan WatcherState)

type WatcherState int

const (
	WatcherNormal WatcherState = iota
	WatcherQuit
)

func main() {
	flag.Parse()

	switch *profileCode {
	case "cpu":
		defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
	case "mem":
		defer profile.Start(profile.MemProfile, profile.ProfilePath(".")).Stop()
	}

	cacheDir, err := os.UserCacheDir()

	if err != nil {
		log.Fatal(err)
	}

	cacheDir = filepath.Join(cacheDir, ConfigDirName)

	if err := os.MkdirAll(cacheDir, 755); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Initializing the watcher...")

	if !fileExists(".watch.json") {
		log.Fatal("Could not find .watch.json file")
	}

	data, err := ioutil.ReadFile(".watch.json")

	if err != nil {
		log.Fatal(err)
	}

	config := Config{
		WatchFolders:         []string{"./"},
		NewFileCheckInterval: 10 * 1000,
		FileCleanupInterval:  10 * 1000,
	}

	if err := json.Unmarshal(data, &config); err != nil {
		log.Fatal(err)
	}

	for _, watchFolder := range config.WatchFolders {
		if !pathExists(watchFolder) {
			log.Fatal("Watch folder " + watchFolder + " doesn't exist!")
		}
	}

	commandOutputChannels := []chan CommandOutput{}

	if len(config.RunParallel) > 0 {
		fmt.Println("Starting parallel commands...")

		for _, cmd := range config.RunParallel {
			_, c := runCommand(cmd.Command, !cmd.WaitBeforeBegin, len(commandOutputChannels)+1)

			commandOutputChannels = append(commandOutputChannels, c)
		}
	}

	smallestInterval := math.MaxInt32

	for _, ext := range config.Extensions {
		if ext.CheckInterval == 0 {
			ext.CheckInterval = DefaultExtensionCheckInterval
		}

		if smallestInterval > ext.CheckInterval {
			smallestInterval = ext.CheckInterval
		}
	}

	fmt.Println("Starting to watch...")

	startedCommands := map[string]*exec.Cmd{}

	for _, ext := range config.Extensions {
		go fileWatcher(config, ext)
	}

	killStartedCommands := func() {
		for _, cmd := range startedCommands {
			if cmd != nil {
				cmd.Process.Kill()
			}
		}
	}

	defer killStartedCommands()

	watcherState := WatcherNormal

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT)
	go func() {
		for sig := range c {
			if sig == syscall.SIGINT {
				killStartedCommands()
				watcherStateChannel <- WatcherQuit
			}
		}
	}()

	for watcherState != WatcherQuit {
		select {
		case state := <-watcherStateChannel:
			watcherState = state
		case path := <-checkQueue:
			fmt.Println("File changed: " + path.Path)

			cmdStr := path.Command

			cmdStr = strings.ReplaceAll(cmdStr, "$FILE$", path.Path)

			if cmd, ok := startedCommands[path.Command]; path.Restart && ok {
				fmt.Println("Restarting with command: " + cmdStr)

				if cmd != nil {
					cmd.Process.Kill()
				} else {
					delete(startedCommands, path.Command)
				}
			} else {
				fmt.Println("Running command: " + cmdStr)
			}

			cmd, c := runCommand(cmdStr, false, len(commandOutputChannels)+1)

			commandOutputChannels = append(commandOutputChannels, c)

			startedCommands[cmdStr] = cmd
		default:

			for _, c := range commandOutputChannels {
				select {
				case output := <-c:
					fmt.Printf("#%d %s\n", output.Number, output.Output)
				default:
				}
			}

			time.Sleep(time.Duration(smallestInterval) * time.Millisecond)
		}
	}
}
