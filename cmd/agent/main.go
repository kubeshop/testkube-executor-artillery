package main

import (
	"log"
	"os"

	"github.com/kubeshop/testkube-executor-artillery/pkg/runner"
	"github.com/kubeshop/testkube/pkg/executor/agent"
	"github.com/kubeshop/testkube/pkg/ui"
)

func main() {
	r, err := runner.NewArtilleryRunner()
	if err != nil {
		log.Fatalf("%s could not run artillery tests: %w", ui.IconCross, err)
	}
	agent.Run(r, os.Args)
}
