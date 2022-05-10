package runner

import (
	"fmt"
	"path/filepath"

	"github.com/kelseyhightower/envconfig"
	"github.com/kubeshop/testkube/pkg/api/v1/testkube"
	"github.com/kubeshop/testkube/pkg/executor"
	"github.com/kubeshop/testkube/pkg/executor/content"
	"github.com/kubeshop/testkube/pkg/executor/output"
	"github.com/kubeshop/testkube/pkg/executor/scrapper"
)

// Params ...
type Params struct {
	Endpoint        string // RUNNER_ENDPOINT
	AccessKeyID     string // RUNNER_ACCESSKEYID
	SecretAccessKey string // RUNNER_SECRETACCESSKEY
	Location        string // RUNNER_LOCATION
	Token           string // RUNNER_TOKEN
	Ssl             bool   // RUNNER_SSL
	ScrapperEnabled bool   // RUNNER_SCRAPPERENABLED
	GitUsername     string // RUNNER_GITUSERNAME
	GitToken        string // RUNNER_GITTOKEN
}

// NewArtilleryRunner ...
func NewArtilleryRunner() *ArtilleryRunner {
	var params Params
	err := envconfig.Process("runner", &params)
	if err != nil {
		panic(err.Error())
	}
	return &ArtilleryRunner{
		Fetcher: content.NewFetcher(""),
		Params:  params,
		Scrapper: scrapper.NewScrapper(
			params.Endpoint,
			params.AccessKeyID,
			params.SecretAccessKey,
			params.Location,
			params.Token,
			params.Ssl,
		),
	}
}

// ArtilleryRunner ...
type ArtilleryRunner struct {
	Params   Params
	Fetcher  content.ContentFetcher
	Scrapper *scrapper.Scrapper
}

// Run ...
func (r *ArtilleryRunner) Run(execution testkube.Execution) (result testkube.ExecutionResult, err error) {
	// make some validation
	err = r.Validate(execution)
	if err != nil {
		return result, err
	}
	if r.Params.GitUsername != "" && r.Params.GitToken != "" {
		if execution.Content != nil && execution.Content.Repository != nil {
			execution.Content.Repository.Username = r.Params.GitUsername
			execution.Content.Repository.Token = r.Params.GitToken
		}
	}
	path, err := r.Fetcher.Fetch(execution.Content)
	if err != nil {
		return result, err
	}

	output.PrintEvent("created content path", path)

	params := make([]string, 0, len(execution.Params))
	for key, value := range execution.Params {
		params = append(params, fmt.Sprintf("%s=%s", key, value))
	}
	testDir, _ := filepath.Split(path)
	args := []string{"run", path}
	if len(params) != 0 {
		args = append(args, params...)
	}
	// artillery test result output file
	testReportFile := filepath.Join(testDir, "test-report.json")

	// append args from execution
	args = append(args, "-o", testReportFile)

	args = append(args, execution.Args...)
	// run executor here
	out, rerr := executor.Run(testDir, "artillery", args...)

	var artilleryResult ArtilleryExecutionResult
	artilleryResult, err = r.GetArtilleryExecutionResult(testReportFile, out)
	if err != nil {
		return result.WithErrors(rerr, fmt.Errorf("failed to get test execution results")), err
	}

	result = MapTestSummaryToResults(artilleryResult)

	if r.Params.ScrapperEnabled && r.Scrapper != nil {
		artifacts := []string{
			testReportFile,
		}
		err = r.Scrapper.Scrape(execution.Id, artifacts)
		if err != nil {
			return result.WithErrors(fmt.Errorf("scrape artifacts error: %w", err)), nil
		}
	}

	// return ExecutionResult
	return result.WithErrors(err), nil
}
