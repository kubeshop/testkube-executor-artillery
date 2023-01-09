package runner

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/kelseyhightower/envconfig"

	"github.com/kubeshop/testkube/pkg/api/v1/testkube"
	"github.com/kubeshop/testkube/pkg/executor"
	"github.com/kubeshop/testkube/pkg/executor/content"
	"github.com/kubeshop/testkube/pkg/executor/output"
	"github.com/kubeshop/testkube/pkg/executor/runner"
	"github.com/kubeshop/testkube/pkg/executor/scraper"
	"github.com/kubeshop/testkube/pkg/executor/secret"
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
	var params Params
	err := envconfig.Process("runner", &params)
	if err != nil {
		panic(err.Error())
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
		return result, err
	}

	output.PrintEvent("created content path", path)

	testDir, _ := filepath.Split(path)
	args := []string{"run", path}
	envManager := secret.NewEnvManagerWithVars(execution.Variables)
	envManager.GetVars(envManager.Variables)

	if len(envManager.Variables) != 0 {
		envFile, err := CreateEnvFile(envManager.Variables)
		if err != nil {
			return result, err
		}
		defer envFile.Close()
		defer os.Remove(envFile.Name())
		args = append(args, "--dotenv", envFile.Name())
		output.PrintEvent("created dotenv file", envFile.Name())
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

	// run executor here
	out, rerr := executor.Run(runPath, "artillery", envManager, args...)

	out = envManager.Obfuscate(out)

	var artilleryResult ArtilleryExecutionResult
	artilleryResult, err = r.GetArtilleryExecutionResult(testReportFile, out)
	if err != nil {
		return result.WithErrors(rerr, fmt.Errorf("failed to get test execution results")), err
	}

	result = MapTestSummaryToResults(artilleryResult)

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

func CreateEnvFile(vars map[string]testkube.Variable) (*os.File, error) {
	envVars := []byte{}
	for _, v := range vars {
		envVars = append(envVars, []byte(fmt.Sprintf("%s=%s\n", v.Name, v.Value))...)
	}
	envFile, err := ioutil.TempFile("/tmp", "")
	if err != nil {
		return nil, fmt.Errorf("could not create dotenv file: %w", err)
	}

	if _, err := envFile.Write(envVars); err != nil {
		return nil, fmt.Errorf("could not write dotenv file: %w", err)
	}

	return envFile, nil
}
