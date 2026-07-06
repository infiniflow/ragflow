package parser

import (
	"fmt"
	"os"
	"strings"
	"time"

	models "ragflow/internal/entity/models"
)

const minerUPollTimeout = 30 * time.Second
const minerUPollInterval = 200 * time.Millisecond

func parsePDFWithMinerU(filename string, data []byte, parser *PDFParser) ParseResult {
	if len(data) == 0 {
		return emptyPDFResult(filename)
	}
	apiServer := strings.TrimSpace(parser.MinerUAPIServer)
	if apiServer == "" {
		apiServer = strings.TrimSpace(os.Getenv("MINERU_APISERVER"))
	}
	if apiServer == "" {
		return ParseResult{Err: fmt.Errorf("parser: MinerU requires mineru_apiserver or MINERU_APISERVER")}
	}
	apiKey := parser.MinerUAPIKey
	if strings.TrimSpace(apiKey) == "" {
		apiKey = strings.TrimSpace(os.Getenv("MINERU_API_KEY"))
	}
	backend := strings.TrimSpace(parser.MinerUBackend)
	if backend == "" {
		backend = strings.TrimSpace(os.Getenv("MINERU_BACKEND"))
	}
	if backend == "" {
		backend = "pipeline"
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

	task, err := driver.ParseFile(&backend, data, nil, apiConfig, &models.ParseFileConfig{})
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: MinerU submit: %w", err)}
	}
	content, err := pollMinerUTask(driver, task.TaskID, apiConfig, timeout)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("parser: MinerU result: %w", err)}
	}
	pageCount := 0
	if strings.TrimSpace(content) != "" {
		pageCount = 1
	}
	return parseMinerUMarkdownResult(filename, content, parser.OutputFormat, pageCount)
}

func pollMinerUTask(driver *models.MinerULocalModel, taskID string, apiConfig *models.APIConfig, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = minerUPollTimeout
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		task, err := driver.ShowTask(taskID, apiConfig)
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

func parseMinerUMarkdownResult(filename, markdown, outputFormat string, pageCount int) ParseResult {
	fileMeta := pdfFileMeta(filename, pageCount)
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "", "json":
		mp, err := NewMarkdownParser(GoMarkdown)
		if err != nil {
			return ParseResult{Err: err}
		}
		res := mp.ParseWithResult(filename, []byte(markdown))
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
