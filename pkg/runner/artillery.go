package runner

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kelseyhightower/envconfig"

	"github.com/kubeshop/testkube/pkg/api/v1/testkube"
	"github.com/kubeshop/testkube/pkg/executor"
	"github.com/kubeshop/testkube/pkg/executor/content"
	"github.com/kubeshop/testkube/pkg/executor/output"
	"github.com/kubeshop/testkube/pkg/executor/runner"
	"github.com/kubeshop/testkube/pkg/executor/scraper"
	"github.com/kubeshop/testkube/pkg/executor/secret"
	"github.com/kubeshop/testkube/pkg/ui"
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
	DataDir         string // RUNNER_DATADIR
}

// NewArtilleryRunner ...
func NewArtilleryRunner() *ArtilleryRunner {
	output.PrintEvent(fmt.Sprintf("%s Preparing test runner", ui.IconTruck))
	var params Params

	output.PrintEvent(fmt.Sprintf("%s Reading environment variables...", ui.IconWorld))
	err := envconfig.Process("runner", &params)
	if err != nil {
		panic(err.Error())
	}
	output.PrintEvent(fmt.Sprintf("%s Environment variables read successfully", ui.IconCheckMark))
	printParams(params)

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
	}
}

// ArtilleryRunner ...
type ArtilleryRunner struct {
	Params  Params
	Fetcher content.ContentFetcher
	Scraper scraper.Scraper
}

// Run ...
func (r *ArtilleryRunner) Run(execution testkube.Execution) (result testkube.ExecutionResult, err error) {
	output.PrintEvent(fmt.Sprintf("%s Preparing for test run", ui.IconTruck))
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

	output.PrintEvent(fmt.Sprintf("%s Fetching test content from %s...", ui.IconBox, execution.Content.Type_))
	path, err := r.Fetcher.Fetch(execution.Content)
	if err != nil {
		return result, fmt.Errorf("could not fetch test content: %w", err)
	}
	output.PrintEvent(fmt.Sprintf("%s Test content fetched to path %s", ui.IconCheckMark, path))

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
	output.PrintEvent(fmt.Sprintf("%s executing test\n\t$ artillery %s", ui.IconMicroscope, strings.Join(args, " ")))
	out, rerr := executor.Run(runPath, "artillery", envManager, args...)

	out = envManager.Obfuscate(out)

	var artilleryResult ArtilleryExecutionResult
	artilleryResult, err = r.GetArtilleryExecutionResult(testReportFile, out)
	if err != nil {
		return result.WithErrors(rerr, fmt.Errorf("failed to get test execution results")), err
	}
	if err == nil && rerr == nil {
		output.PrintEvent(fmt.Sprintf("%s Test run succeeded", ui.IconCheckMark))
	} else {
		output.PrintEvent(fmt.Sprintf("%s Test run failed: \n %s \n %s", ui.IconCross, err.Error(), rerr.Error()))
	}

	result = MapTestSummaryToResults(artilleryResult)

	if r.Params.ScrapperEnabled && r.Scraper != nil {
		artifacts := []string{
			testReportFile,
		}
		output.PrintEvent(fmt.Sprintf("%s Scraping artifacts %s", ui.IconCabinet, artifacts))
		err = r.Scraper.Scrape(execution.Id, artifacts)
		if err != nil {
			output.PrintEvent(fmt.Sprintf("%s Failed to scrape artifacts: %s", ui.IconCross, err.Error()))
			return result.WithErrors(fmt.Errorf("scrape artifacts error: %w", err)), nil
		}
		output.PrintEvent(fmt.Sprintf("%s Successfully scraped artifacts", ui.IconCheckMark))
	}

	// return ExecutionResult
	return result.WithErrors(err), nil
}

// GetType returns runner type
func (r *ArtilleryRunner) GetType() runner.Type {
	return runner.TypeMain
}

// printParams shows the read parameters in logs
func printParams(params Params) {
	output.PrintLog(fmt.Sprintf("RUNNER_ENDPOINT=\"%s\"", params.Endpoint))
	printSensitiveParam("RUNNER_ACCESSKEYID", params.AccessKeyID)
	printSensitiveParam("RUNNER_SECRETACCESSKEY", params.SecretAccessKey)
	output.PrintLog(fmt.Sprintf("RUNNER_LOCATION=\"%s\"", params.Location))
	printSensitiveParam("RUNNER_TOKEN", params.Token)
	output.PrintLog(fmt.Sprintf("RUNNER_SSL=%t", params.Ssl))
	output.PrintLog(fmt.Sprintf("RUNNER_SCRAPPERENABLED=\"%t\"", params.ScrapperEnabled))
	output.PrintLog(fmt.Sprintf("RUNNER_GITUSERNAME=\"%s\"", params.GitUsername))
	printSensitiveParam("RUNNER_GITTOKEN", params.GitToken)
	output.PrintLog(fmt.Sprintf("RUNNER_DATADIR=\"%s\"", params.DataDir))
}

// printSensitiveParam shows in logs if a parameter is set or not
func printSensitiveParam(name string, value string) {
	if len(value) == 0 {
		output.PrintLog(fmt.Sprintf("%s=\"\"", name))
	} else {
		output.PrintLog(fmt.Sprintf("%s=\"********\"", name))
	}
}
