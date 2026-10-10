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

func parsePDFWithMinerU(ctx context.Context, filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	providerCfg := models.MinerUProviderConfigFromAPIKey(parser.MinerUAPIKey)

	apiServer := strings.TrimSpace(parser.MinerUAPIServer)
	if apiServer == "" {
		apiServer = providerCfg.APIServer
	}
	if apiServer == "" {
		apiServer = strings.TrimSpace(common.GetEnv(common.EnvMineruAPIServer))
	}
	if apiServer == "" {
		return ParseResult{Err: fmt.Errorf("parser: MinerU requires mineru_apiserver or MINERU_APISERVER")}
	}

	apiKey := providerCfg.AccessToken
	if apiKey == "" && !providerCfg.IsProviderJSON {
		apiKey = strings.TrimSpace(parser.MinerUAPIKey)
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(common.GetEnv(common.EnvMineruAPIKey))
	}

	backend := models.ResolveMinerUBackend(parser.MinerUBackend, parser.MinerUAPIKey)
	serverURL := models.ResolveMinerUServerURL(parser.MinerUServerURL, parser.MinerUAPIKey)
	if _, ok := models.ValidMinerUBackends[backend]; !ok {
		return ParseResult{Err: models.ValidateMinerUConfig(backend, serverURL)}
	}
	timeout := parser.MinerUPollTimeout
	if timeout <= 0 {
		timeout = minerUPollTimeout
	}

	v1, err := models.MinerUSupportsV1(ctx, apiServer, apiKey)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: MinerU probe: %w", err)}
	}
	if err := models.ValidateMinerUConfigForAPI(backend, serverURL, v1); err != nil {
		return ParseResult{Err: err}
	}
	if v1 {
		result, err := models.ParseMinerUV1(ctx, apiServer, apiKey, filename, data, backend, timeout)
		if err != nil {
			return ParseResult{Err: fmt.Errorf("parser: MinerU V1: %w", err)}
		}
		content := ""
		if result != nil {
			content = result.Markdown
			if strings.TrimSpace(content) == "" && len(result.Zip) > 0 {
				content, err = models.MinerUMarkdownFromZip(result.Zip)
				if err != nil {
					return ParseResult{Err: fmt.Errorf("parser: MinerU V1 extract: %w", err)}
				}
			}
		}
		pageCount := 0
		if strings.TrimSpace(content) != "" {
			pageCount = 1
		}
		return parseMinerUMarkdownResult(ctx, filename, content, parser.OutputFormat, pageCount)
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
