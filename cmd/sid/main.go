package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "[staking-indexer] %v\n", err)
	os.Exit(1)
}

func main() {
	app := cli.NewApp()
	app.Name = "sid"
	app.Usage = "Staking Indexer Daemon (sid)."
	app.Commands = append(app.Commands, startCommand, initCommand)

	if err := app.Run(os.Args); err != nil {
		fatal(err)
	}
}
