package cmd

import (
	"fmt"
	"os"

	"github.com/shadyabhi/mantee/app"
)

// Execute is the main entry point for the CLI
func Execute() {
	var keyword string

	// Parse CLI arguments
	if len(os.Args) >= 2 {
		keyword = os.Args[1]
	}

	// Run the application
	if err := app.Run(keyword); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
