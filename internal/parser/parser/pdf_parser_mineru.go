package parser

import (
	"context"
	"fmt"
	"ragflow/internal/common"
	"strings"
	"time"

	models "ragflow/internal/entity/models"
)

const minerUPollTimeout = 30 * time.Second
const minerUPollInterval = 200 * time.Millisecond

var validMinerUBackends = map[string]struct{}{
	"pipeline":           {},
	"vlm-engine":         {},
	"hybrid-engine":      {},
	"vlm-http-client":    {},
	"hybrid-http-client": {},
}

func minerUBackendRequiresServerURL(backend string) bool {
	return backend == "vlm-http-client" || backend == "hybrid-http-client"
}

func resolveMinerUBackend(parser *PDFParser) string {
	backend := strings.TrimSpace(parser.MinerUBackend)
	if backend == "" {
		backend = strings.TrimSpace(common.GetEnv(common.EnvMineruBackend))
	}
	if backend == "" {
		backend = "pipeline"
	}
	return backend
}

func resolveMinerUServerURL(parser *PDFParser) string {
	serverURL := strings.TrimSpace(parser.MinerUServerURL)
	if serverURL == "" {
		serverURL = strings.TrimSpace(common.GetEnv(common.EnvMineruServerURL))
	}
	return strings.TrimRight(serverURL, "/")
}

func validateMinerUConfig(backend, serverURL string) error {
	if _, ok := validMinerUBackends[backend]; !ok {
		return fmt.Errorf(
			"parser: MinerU invalid backend %q (valid: pipeline, vlm-engine, hybrid-engine, vlm-http-client, hybrid-http-client)",
			backend,
		)
	}
	if minerUBackendRequiresServerURL(backend) && serverURL == "" {
		return fmt.Errorf("parser: MinerU requires mineru_server_url or MINERU_SERVER_URL for backend %q", backend)
	}
	return nil
}

func parsePDFWithMinerU(ctx context.Context, filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	apiServer := strings.TrimSpace(parser.MinerUAPIServer)
	if apiServer == "" {
		apiServer = strings.TrimSpace(common.GetEnv(common.EnvMineruAPIServer))
	}
	if apiServer == "" {
		return ParseResult{Err: fmt.Errorf("parser: MinerU requires mineru_apiserver or MINERU_APISERVER")}
	}
	apiKey := parser.MinerUAPIKey
	if strings.TrimSpace(apiKey) == "" {
		apiKey = strings.TrimSpace(common.GetEnv(common.EnvMineruAPIKey))
	}
	backend := resolveMinerUBackend(parser)
	serverURL := resolveMinerUServerURL(parser)
	if err := validateMinerUConfig(backend, serverURL); err != nil {
		return ParseResult{Err: err}
	}
	timeout := parser.MinerUPollTimeout
	if timeout <= 0 {
		timeout = minerUPollTimeout
	}

	driver := models.NewMinerLocalUModel(
		map[string]string{"default": apiServer},
		models.URLSuffix{DocumentParse: "file_parse", Task: "tasks"},
	)
	apiConfig := &models.APIConfig{
		BaseURL: &apiServer,
	}
	if apiKey != "" {
		apiConfig.ApiKey = &apiKey
	}

	parseFileConfig := &models.ParseFileConfig{ServerURL: serverURL}
	task, err := driver.ParseFile(ctx, &backend, data, nil, apiConfig, parseFileConfig, nil)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: MinerU submit: %w", err)}
	}
	content, err := pollMinerUTask(ctx, driver, task.TaskID, apiConfig, timeout)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: MinerU result: %w", err)}
	}
	pageCount := 0
	if strings.TrimSpace(content) != "" {
		pageCount = 1
	}
	return parseMinerUMarkdownResult(ctx, filename, content, parser.OutputFormat, pageCount)
}

func pollMinerUTask(ctx context.Context, driver *models.MinerULocalModel, taskID string, apiConfig *models.APIConfig, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = minerUPollTimeout
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		task, err := driver.ShowTask(ctx, taskID, apiConfig)
		if err == nil {
			for _, segment := range task.Segments {
				if strings.TrimSpace(segment.Content) != "" {
					return segment.Content, nil
				}
			}
			lastErr = fmt.Errorf("empty MinerU task content")
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = fmt.Errorf("timed out waiting for MinerU task %s", taskID)
			}
			return "", lastErr
		}
		time.Sleep(minerUPollInterval)
	}
}

func parseMinerUMarkdownResult(ctx context.Context, filename, markdown, outputFormat string, pageCount int) ParseResult {
	fileMeta := pdfFileMeta(filename, pageCount)
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "", "json":
		mp, _ := NewMarkdownParser(GoMarkdown)
		res := mp.ParseWithResult(ctx, filename, []byte(markdown))
		if res.Err != nil {
			return res
		}
		res.File = fileMeta
		return res
	case "markdown":
		return ParseResult{
			OutputFormat: "markdown",
			File:         fileMeta,
			Markdown:     markdown,
		}
	default:
		return ParseResult{Err: fmt.Errorf("parser: unsupported PDF output_format %q", outputFormat)}
	}
}
