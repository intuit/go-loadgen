package main

import (
	"github.intuit.com/cbhatt1/go-loadgen/cli"
	loadgen "github.intuit.com/cbhatt1/go-loadgen/loadgenerator"
)

func main() {

	props := new(loadgen.LoadGenProperties)
	cli.Run(props)
}


