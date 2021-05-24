package main

import (
	"os"

	"github.com/nclv/golang-blockchain/cli"
)

func main() {
	defer os.Exit(0)

	cli := cli.CommandLine{}
	cli.Run()
}
