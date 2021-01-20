package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
)

type CommandOutput struct {
	Output string
	Number int
}

func runCommand(command string, parallel bool, cmdNo int) (*exec.Cmd, chan CommandOutput) {
	outputChannel := make(chan CommandOutput, 50)
	fmt.Println("Starting command #" + strconv.Itoa(cmdNo) + ": " + command)

	var cmd *exec.Cmd
	runFunc := func() {
		cmdSplitted := strings.Split(command, " ")

		errBuf := bytes.Buffer{}

		cmd = exec.Command(filepath.Clean(cmdSplitted[0]), cmdSplitted[1:]...)
		cmd.Stderr = &errBuf

		outPipe, err := cmd.StdoutPipe()

		if err != nil {
			log.Fatal(err)
		}

		startTime := time.Now()

		tookTime := func() string {
			return "Took " + time.Now().Sub(startTime).String()
		}

		if err := cmd.Start(); err != nil {
			fmt.Println(aurora.Red("Could not start command: " + command))
			fmt.Println(aurora.Red("Error: "), aurora.Red(err))
			return
		}

		tookTime()

		r := bufio.NewReader(outPipe)

		for {
			if cmd.ProcessState != nil {
				fmt.Println("Process exited...")
				break
			}

			line, _, err := r.ReadLine()

			if err != nil {
				break
			}

			outputChannel <- CommandOutput{
				Output: string(line),
				Number: cmdNo,
			}

			time.Sleep(1000 * time.Millisecond)
		}

		if cmd.ProcessState.ExitCode() != 0 {
			fmt.Println(aurora.Red("Error in command: "+command), tookTime())
			fmt.Println(aurora.Red(string(errBuf.Bytes())))
		} else {
			fmt.Println("Command "+command+" done.", tookTime())
		}
	}

	if parallel {
		go runFunc()
	} else {
		runFunc()
	}

	return cmd, outputChannel
}
