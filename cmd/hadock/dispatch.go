package main

import (
	"github.com/midbel/cli"
)

func runDispatch(cmd *cli.Command, args []string) error {
	return cmd.Flag.Parse(args)
}
