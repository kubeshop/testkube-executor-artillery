package main

import (
	"fmt"
	"os"

	"github.com/kubeshop/testkube-executor-artillery/pkg/runner"
	"github.com/kubeshop/testkube/pkg/executor/agent"
)

func main() {
	r, err := runner.NewArtilleryRunner()
	if err != nil {
		panic(fmt.Errorf("could not run artillery tests: %w", err))
	}
	agent.Run(r, os.Args)
}
