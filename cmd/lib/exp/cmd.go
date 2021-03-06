package exp

import (
	"gopkg.in/urfave/cli.v1"
)

// SetUp sets up SensorBee's experimenting command.
func SetUp() cli.Command {
	cmd := cli.Command{
		Name:  "exp",
		Usage: "experiment BQL statements",
		Subcommands: []cli.Command{
			setUpRun(),
			setUpClean(),
			setUpFile(),
			// TODO: hash <node>: get hash of the latest cache of the node
		},
	}
	return cmd
}
