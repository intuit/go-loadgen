package main

import (
	"github.com/intuit/go-loadgen/cli"
	loadgen "github.com/intuit/go-loadgen/loadgenerator"
)

func main() {

	props := new(loadgen.LoadGenProperties)
	cli.Run(props)
}