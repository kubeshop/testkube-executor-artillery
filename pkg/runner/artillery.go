package runner

import (
	"fmt"
	"path/filepath"

	"github.com/kubeshop/testkube/pkg/api/v1/testkube"
	"github.com/kubeshop/testkube/pkg/envs"
	"github.com/kubeshop/testkube/pkg/executor"
	"github.com/kubeshop/testkube/pkg/executor/content"
	"github.com/kubeshop/testkube/pkg/executor/output"
	"github.com/kubeshop/testkube/pkg/executor/runner"
	"github.com/kubeshop/testkube/pkg/executor/scraper"
	"github.com/kubeshop/testkube/pkg/executor/secret"
	"github.com/kubeshop/testkube/pkg/ui"
)

// NewArtilleryRunner creates a new Testkube test runner for Artillery tests
func NewArtilleryRunner() (*ArtilleryRunner, error) {
	output.PrintLog(fmt.Sprintf("%s Preparing test runner", ui.IconTruck))
	params, err := envs.LoadTestkubeVariables()
	if err != nil {
		return nil, fmt.Errorf("could not initialize Artillery runner variables: %w", err)
	}

	return &ArtilleryRunner{
		Fetcher: content.NewFetcher(""),
		Params:  params,
		Scraper: scraper.NewMinioScraper(
			params.Endpoint,
			params.AccessKeyID,
			params.SecretAccessKey,
			params.Location,
			params.Token,
			params.Ssl,
		),
	}, err
}

// ArtilleryRunner ...
type ArtilleryRunner struct {
	Params  envs.Params
	Fetcher content.ContentFetcher
	Scraper scraper.Scraper
}

// Run ...
func (r *ArtilleryRunner) Run(execution testkube.Execution) (result testkube.ExecutionResult, err error) {
	output.PrintLog(fmt.Sprintf("%s Preparing for test run", ui.IconTruck))
	// make some validation
	err = r.Validate(execution)
	if err != nil {
		return result, err
	}
	if r.Params.GitUsername != "" || r.Params.GitToken != "" {
		if execution.Content != nil && execution.Content.Repository != nil {
			execution.Content.Repository.Username = r.Params.GitUsername
			execution.Content.Repository.Token = r.Params.GitToken
		}
	}

	path, err := r.Fetcher.Fetch(execution.Content)
	if err != nil {
		return result, fmt.Errorf("could not fetch test content: %w", err)
	}

	testDir, _ := filepath.Split(path)
	args := []string{"run", path}
	envManager := secret.NewEnvManagerWithVars(execution.Variables)
	envManager.GetVars(envManager.Variables)
	for _, v := range envManager.Variables {
		args = append(args, fmt.Sprintf("%s=%s", v.Name, v.Value))
	}
	// artillery test result output file
	testReportFile := filepath.Join(testDir, "test-report.json")

	// append args from execution
	args = append(args, "-o", testReportFile)

	args = append(args, execution.Args...)

	runPath := testDir
	if execution.Content.Repository != nil && execution.Content.Repository.WorkingDir != "" {
		runPath = filepath.Join(r.Params.DataDir, "repo", execution.Content.Repository.WorkingDir)
	}

	// run executor
	out, rerr := executor.Run(runPath, "artillery", envManager, args...)

	out = envManager.Obfuscate(out)

	var artilleryResult ArtilleryExecutionResult
	artilleryResult, err = r.GetArtilleryExecutionResult(testReportFile, out)
	if err != nil {
		return result.WithErrors(rerr, fmt.Errorf("failed to get test execution results")), err
	}

	result = MapTestSummaryToResults(artilleryResult)
	output.PrintLog(fmt.Sprintf("%s Mapped test summary to Execution Results...", ui.IconCheckMark))

	if r.Params.ScrapperEnabled && r.Scraper != nil {
		artifacts := []string{
			testReportFile,
		}
		err = r.Scraper.Scrape(execution.Id, artifacts)
		if err != nil {
			return result.WithErrors(fmt.Errorf("scrape artifacts error: %w", err)), nil
		}
	}

	// return ExecutionResult
	return result.WithErrors(err), nil
}

// GetType returns runner type
func (r *ArtilleryRunner) GetType() runner.Type {
	return runner.TypeMain
}
