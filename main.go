package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Tech-Arch1tect/berth-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		var commandErr *cmd.CommandExitError
		if errors.As(err, &commandErr) {
			if commandErr.Interrupted {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			os.Exit(commandErr.Code)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		var operationErr *cmd.OperationExitError
		if errors.As(err, &operationErr) {
			os.Exit(operationErr.Code)
		}
		os.Exit(1)
	}
}
