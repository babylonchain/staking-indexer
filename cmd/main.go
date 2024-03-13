package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

const (
	homeFlag = "home"
)

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "[staking-indexer] %v\n", err)
	os.Exit(1)
}

func main() {
	app := cli.NewApp()
	app.Name = "indexer"
	app.Usage = "Staking Indexer."
	app.Commands = append(app.Commands, startCommand, initCommand)

	if err := app.Run(os.Args); err != nil {
		fatal(err)
	}
}
