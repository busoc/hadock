package main

import (
	"github.com/midbel/cli"
)

const helpText = `{{.Name}} process VMU packets.

Usage:

  {{.Name}} command [arguments]

The commands are:

{{range .Commands}}{{printf "  %-9s %s" .String .Short}}
{{end}}

Use {{.Name}} [command] -h for more information about its usage.
`

var commands = []*cli.Command{
	{
		Usage: "replay [-r] [-s] [-m] [-t] <host:port> <archive...>",
		Short: "send VMU packets throught the network from a HRDP archive",
		Run:   runReplay,
	},
	{
		Usage: "listen <hdk.toml>",
		Short: "store packets in archive",
		Run:   runListen,
	},
	{
		Usage: "distrib <hdk.toml>",
		Short: "distribute files stored in archive",
		Run:   runDistrib,
	},
	{
		Usage: "monitor <group...>",
		Short: "monitor hadock activities",
		Run:   runMonitor,
	},
	{
		Usage: "dispatch <directory>",
		Short: "",
		Run:   runDispatch,
	},
}

func main() {
	cli.RunAndExit(commands, cli.Usage("hadock", helpText, commands))
}
