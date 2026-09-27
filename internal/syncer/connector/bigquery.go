//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	gcpbigquery "cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const (
	defaultBigQueryBatchSize  = 2
	defaultBigQueryPageSize   = 1000
	defaultBigQueryMaxBytes   = int64(1024 * 1024 * 1024)
	bigQueryCursorTypeKey     = "__ragflow_bq_cursor_type__"
	bigQueryIdentifierPattern = `^[A-Za-z_][A-Za-z0-9_]*$`
	bigQueryProjectIDPattern  = `^[A-Za-z][A-Za-z0-9_-]*$`
)

var bigQueryIdentifierRE = regexp.MustCompile(bigQueryIdentifierPattern)
var bigQueryProjectIDRE = regexp.MustCompile(bigQueryProjectIDPattern)

// BigQueryConnector imports rows from Google BigQuery tables or custom queries.
//
// It mirrors the Python BigQueryConnector: selected content columns form the
// document body, metadata columns are copied into document metadata, an
// optional id column gives stable document ids, and an optional timestamp
// column drives cursor-based incremental sync and pruning.
type BigQueryConnector struct {
	projectID          string
	datasetID          string
	tableID            string
	location           string
	query              string
	contentColumns     []string
	metadataColumns    []string
	idColumn           string
	timestampColumn    string
	batchSize          int
	pageSize           int
	maximumBytesBilled int64
	jobTimeout         time.Duration
	useQueryCache      bool
	credentialsJSON    []byte
	newClient          func(context.Context) (bigQueryClient, error)
}

type bigQueryClient interface {
	Query(query string) bigQueryQuery
	Close() error
}

type bigQueryQuery interface {
	Run(ctx context.Context) (bigQueryJob, error)
	Read(ctx context.Context) (bigQueryRowIterator, error)
	SetDryRun(bool)
	SetDisableQueryCache(bool)
	SetMaxBytesBilled(int64)
	SetJobTimeout(time.Duration)
	SetParameters([]gcpbigquery.QueryParameter)
}

type bigQueryJob interface {
	Status(ctx context.Context) (bigQueryJobStatus, error)
	Wait(ctx context.Context) (bigQueryJobStatus, error)
	Read(ctx context.Context) (bigQueryRowIterator, error)
}

type bigQueryJobStatus interface {
	Done() bool
	Err() error
	Statistics() bigQueryJobStatistics
}

type bigQueryJobStatistics interface {
	QuerySchema() []bigQueryField
	TotalBytesProcessed() int64
}

type bigQueryRowIterator interface {
	Next(dst any) error
	Schema() []bigQueryField
	SetPageSize(int)
}

type bigQueryField struct {
	Name string
	Type string
}

type gcpBigQueryClient struct {
	client *gcpbigquery.Client
}

func (c *gcpBigQueryClient) Query(query string) bigQueryQuery {
	return &gcpBigQueryQuery{q: c.client.Query(query)}
}

func (c *gcpBigQueryClient) Close() error { return c.client.Close() }

type gcpBigQueryQuery struct {
	q *gcpbigquery.Query
}

func (q *gcpBigQueryQuery) Run(ctx context.Context) (bigQueryJob, error) {
	job, err := q.q.Run(ctx)
	if err != nil {
		return nil, err
	}
	return &gcpBigQueryJob{j: job}, nil
}

func (q *gcpBigQueryQuery) Read(ctx context.Context) (bigQueryRowIterator, error) {
	it, err := q.q.Read(ctx)
	if err != nil {
		return nil, err
	}
	return &gcpBigQueryRowIterator{it: it}, nil
}

func (q *gcpBigQueryQuery) SetDryRun(value bool)              { q.q.DryRun = value }
func (q *gcpBigQueryQuery) SetDisableQueryCache(value bool)   { q.q.DisableQueryCache = value }
func (q *gcpBigQueryQuery) SetMaxBytesBilled(value int64)     { q.q.MaxBytesBilled = value }
func (q *gcpBigQueryQuery) SetJobTimeout(value time.Duration) { q.q.JobTimeout = value }
func (q *gcpBigQueryQuery) SetParameters(parameters []gcpbigquery.QueryParameter) {
	q.q.Parameters = append([]gcpbigquery.QueryParameter(nil), parameters...)
}

type gcpBigQueryJob struct{ j *gcpbigquery.Job }

func (j *gcpBigQueryJob) Status(ctx context.Context) (bigQueryJobStatus, error) {
	status, err := j.j.Status(ctx)
	if err != nil {
		return nil, err
	}
	return &gcpBigQueryJobStatus{status: status}, nil
}

func (j *gcpBigQueryJob) Wait(ctx context.Context) (bigQueryJobStatus, error) {
	status, err := j.j.Wait(ctx)
	if err != nil {
		return nil, err
	}
	return &gcpBigQueryJobStatus{status: status}, nil
}

func (j *gcpBigQueryJob) Read(ctx context.Context) (bigQueryRowIterator, error) {
	it, err := j.j.Read(ctx)
	if err != nil {
		return nil, err
	}
	return &gcpBigQueryRowIterator{it: it}, nil
}

type gcpBigQueryJobStatus struct{ status *gcpbigquery.JobStatus }

func (s *gcpBigQueryJobStatus) Done() bool { return s.status.Done() }
func (s *gcpBigQueryJobStatus) Err() error { return s.status.Err() }
func (s *gcpBigQueryJobStatus) Statistics() bigQueryJobStatistics {
	return &gcpBigQueryJobStatistics{stats: s.status.Statistics}
}

type gcpBigQueryJobStatistics struct{ stats *gcpbigquery.JobStatistics }

func (s *gcpBigQueryJobStatistics) QuerySchema() []bigQueryField {
	if s.stats == nil || s.stats.Details == nil {
		return nil
	}
	queryStats, ok := s.stats.Details.(*gcpbigquery.QueryStatistics)
	if !ok {
		return nil
	}
	return toBigQueryFields(queryStats.Schema)
}

func (s *gcpBigQueryJobStatistics) TotalBytesProcessed() int64 {
	if s.stats == nil {
		return 0
	}
	return s.stats.TotalBytesProcessed
}

type gcpBigQueryRowIterator struct{ it *gcpbigquery.RowIterator }

func (i *gcpBigQueryRowIterator) Next(dst any) error {
	err := i.it.Next(dst)
	if errors.Is(err, iterator.Done) {
		return io.EOF
	}
	return err
}

func (i *gcpBigQueryRowIterator) Schema() []bigQueryField { return toBigQueryFields(i.it.Schema) }

func (i *gcpBigQueryRowIterator) SetPageSize(size int) {
	if size <= 0 || i.it == nil || i.it.PageInfo() == nil {
		return
	}
	i.it.PageInfo().MaxSize = size
}

// NewBigQueryConnector creates a BigQuery connector from Python-compatible config.
func NewBigQueryConnector(config map[string]any) (*BigQueryConnector, error) {
	credentials := configAnyMap(config["credentials"])
	rawCredential := credentials["service_account_json"]
	var credentialBytes []byte
	switch value := rawCredential.(type) {
	case string:
		credentialBytes = []byte(strings.TrimSpace(value))
	case map[string]any:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, &ConnectorValidationError{Message: fmt.Sprintf("BigQuery: service_account_json is not valid JSON: %v", err)}
		}
		credentialBytes = data
	default:
		if rawCredential != nil {
			return nil, &ConnectorValidationError{Message: "BigQuery: service_account_json must be a JSON string or object"}
		}
	}

	connector := &BigQueryConnector{
		projectID:          strings.TrimSpace(stringConfig(config["project_id"])),
		datasetID:          strings.TrimSpace(stringConfig(config["dataset_id"])),
		tableID:            strings.TrimSpace(stringConfig(config["table_id"])),
		location:           strings.TrimSpace(stringConfig(config["location"])),
		query:              sanitizeBigQueryQuery(stringConfig(config["query"])),
		idColumn:           strings.TrimSpace(stringConfig(config["id_column"])),
		timestampColumn:    strings.TrimSpace(stringConfig(config["timestamp_column"])),
		batchSize:          configInt(config["batch_size"], defaultBigQueryBatchSize),
		pageSize:           configInt(config["page_size"], defaultBigQueryPageSize),
		maximumBytesBilled: configInt64(config["maximum_bytes_billed"], defaultBigQueryMaxBytes),
		useQueryCache:      configBoolDefault(config["use_query_cache"], true),
		credentialsJSON:    credentialBytes,
	}
	if connector.maximumBytesBilled <= 0 {
		connector.maximumBytesBilled = defaultBigQueryMaxBytes
	}
	if value := config["job_timeout_ms"]; value != nil {
		connector.jobTimeout = time.Duration(configInt(value, 0)) * time.Millisecond
	}
	connector.contentColumns = connector.splitColumns(config["content_columns"])
	connector.metadataColumns = connector.splitColumns(config["metadata_columns"])
	if err := connector.validateIdentifiers(); err != nil {
		return nil, err
	}
	return connector, nil
}

// Validate validates BigQuery settings and credentials.
func (c *BigQueryConnector) Validate(ctx context.Context) error {
	if err := c.validateStatic(); err != nil {
		return err
	}
	client, err := c.openClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	connectivity := c.configuredQuery(client, "SELECT 1")
	if err := waitBigQueryJob(ctx, connectivity); err != nil {
		return &ConnectorValidationError{Message: fmt.Sprintf("BigQuery validation failed: %v", err)}
	}

	dryRun := c.configuredQuery(client, c.wrapQuery(c.buildBaseQuery(), "*"))
	dryRun.SetDryRun(true)
	dryRun.SetDisableQueryCache(true)
	job, err := dryRun.Run(ctx)
	if err != nil {
		return &ConnectorValidationError{Message: fmt.Sprintf("BigQuery validation failed: %v", err)}
	}
	status, err := job.Wait(ctx)
	if err != nil {
		return &ConnectorValidationError{Message: fmt.Sprintf("BigQuery validation failed: %v", err)}
	}
	if !status.Done() {
		return &ConnectorValidationError{Message: "BigQuery validation dry-run did not complete"}
	}
	if err := status.Err(); err != nil {
		return &ConnectorValidationError{Message: fmt.Sprintf("BigQuery validation failed: %v", err)}
	}
	schema := status.Statistics().QuerySchema()
	if len(schema) == 0 {
		return &ConnectorValidationError{Message: "BigQuery validation dry-run returned no schema"}
	}
	return c.validateSchema(schema)
}

// ValidateConnectorSetting validates an unsaved BigQuery config.
func (c *BigQueryConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	candidate, err := NewBigQueryConnector(request)
	if err != nil {
		return err
	}
	return candidate.Validate(ctx)
}

// OpenSync opens one BigQuery sync session.
func (c *BigQueryConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	client, err := c.openClient(ctx)
	if err != nil {
		return nil, err
	}
	queryText, parameters := c.buildSyncQuery(request)
	query := c.configuredQuery(client, queryText)
	query.SetParameters(parameters)
	iterator, err := query.Read(ctx)
	if err != nil {
		client.Close()
		return nil, err
	}
	iterator.SetPageSize(c.effectivePageSize())
	session := &bigQuerySyncSession{
		connector: c,
		client:    client,
		rows:      iterator,
		batchSize: c.effectiveBatchSize(),
	}
	if err := session.applyResume(request.Resume); err != nil {
		client.Close()
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete BigQuery prune snapshot session.
func (c *BigQueryConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	client, err := c.openClient(ctx)
	if err != nil {
		return nil, err
	}
	query := c.configuredQuery(client, c.buildSlimQuery(c.buildBaseQuery()))
	iterator, err := query.Read(ctx)
	if err != nil {
		client.Close()
		return nil, err
	}
	iterator.SetPageSize(c.effectivePageSize())
	return &bigQueryPruneSession{
		connector: c,
		client:    client,
		rows:      iterator,
		batchSize: c.effectiveBatchSize(),
	}, nil
}

func (c *BigQueryConnector) openClient(ctx context.Context) (bigQueryClient, error) {
	if c.newClient != nil {
		return c.newClient(ctx)
	}
	if len(c.credentialsJSON) == 0 {
		return nil, &ConnectorMissingCredentialError{Message: "BigQuery: missing service_account_json"}
	}
	if !json.Valid(c.credentialsJSON) {
		return nil, &ConnectorValidationError{Message: "BigQuery: service_account_json is not valid JSON"}
	}
	options := []option.ClientOption{option.WithCredentialsJSON(c.credentialsJSON)}
	client, err := gcpbigquery.NewClient(ctx, c.projectID, options...)
	if err != nil {
		return nil, fmt.Errorf("BigQuery: failed to create client: %w", err)
	}
	if c.location != "" {
		client.Location = c.location
	}
	return &gcpBigQueryClient{client: client}, nil
}

func (c *BigQueryConnector) configuredQuery(client bigQueryClient, query string) bigQueryQuery {
	q := client.Query(query)
	q.SetMaxBytesBilled(c.maximumBytesBilled)
	q.SetJobTimeout(c.jobTimeout)
	q.SetDisableQueryCache(!c.useQueryCache)
	return q
}

func (c *BigQueryConnector) validateStatic() error {
	if c == nil {
		return &ConnectorValidationError{Message: "BigQuery connector is nil"}
	}
	if c.projectID == "" {
		return &ConnectorValidationError{Message: "BigQuery project_id is required."}
	}
	if len(c.contentColumns) == 0 {
		return &ConnectorValidationError{Message: "At least one content column must be specified."}
	}
	if c.query == "" && (c.datasetID == "" || c.tableID == "") {
		return &ConnectorValidationError{Message: "BigQuery requires either a custom query or both dataset_id and table_id."}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "BigQuery batch_size must be a positive integer"}
	}
	if c.pageSize <= 0 {
		return &ConnectorValidationError{Message: "BigQuery page_size must be a positive integer"}
	}
	if len(c.credentialsJSON) == 0 {
		return &ConnectorMissingCredentialError{Message: "BigQuery: missing service_account_json"}
	}
	return nil
}

func (c *BigQueryConnector) validateIdentifiers() error {
	if c.projectID != "" && !bigQueryProjectIDRE.MatchString(c.projectID) {
		return &ConnectorValidationError{Message: fmt.Sprintf("Invalid BigQuery identifier for %q", "project_id")}
	}
	for _, column := range c.contentColumns {
		if err := validateBigQueryIdentifier(column, fmt.Sprintf("content_columns[%q]", column)); err != nil {
			return err
		}
	}
	for _, column := range c.metadataColumns {
		if err := validateBigQueryIdentifier(column, fmt.Sprintf("metadata_columns[%q]", column)); err != nil {
			return err
		}
	}
	for name, column := range map[string]string{"dataset_id": c.datasetID, "table_id": c.tableID} {
		if column != "" {
			if err := validateBigQueryIdentifier(column, name); err != nil {
				return err
			}
		}
	}
	if c.idColumn != "" {
		if err := validateBigQueryIdentifier(c.idColumn, "id_column"); err != nil {
			return err
		}
	}
	if c.timestampColumn != "" {
		if err := validateBigQueryIdentifier(c.timestampColumn, "timestamp_column"); err != nil {
			return err
		}
	}
	return nil
}

func validateBigQueryIdentifier(value, name string) error {
	if value == "" {
		return nil
	}
	if !bigQueryIdentifierRE.MatchString(value) {
		return &ConnectorValidationError{Message: fmt.Sprintf("Invalid BigQuery identifier for %q", name)}
	}
	return nil
}

func (c *BigQueryConnector) effectiveBatchSize() int {
	if c.batchSize > 0 {
		return c.batchSize
	}
	return defaultBigQueryBatchSize
}

func (c *BigQueryConnector) effectivePageSize() int {
	if c.pageSize > 0 {
		return c.pageSize
	}
	return defaultBigQueryPageSize
}

func (c *BigQueryConnector) buildBaseQuery() string {
	if c.query != "" {
		return c.query
	}
	return fmt.Sprintf("SELECT * FROM `%s.%s.%s`", c.projectID, c.datasetID, c.tableID)
}

func (c *BigQueryConnector) usesTableMode() bool { return c.query == "" }

func (c *BigQueryConnector) wrapQuery(base, selectClause string) string {
	return c.wrapFilteredQuery(base, selectClause, "")
}

func (c *BigQueryConnector) wrapFilteredQuery(base, selectClause, where string) string {
	query := fmt.Sprintf("SELECT %s FROM (%s) AS ragflow_src", selectClause, base)
	if where != "" {
		query += " WHERE " + where
	}
	return query + c.stableOrderClause()
}

func (c *BigQueryConnector) stableOrderClause() string {
	columns := make([]string, 0, 2)
	if c.idColumn != "" {
		columns = append(columns, c.idColumn)
	}
	if c.timestampColumn != "" && c.timestampColumn != c.idColumn {
		columns = append(columns, c.timestampColumn)
	}
	if len(columns) == 0 {
		return ""
	}
	order := make([]string, 0, len(columns))
	for _, column := range columns {
		order = append(order, fmt.Sprintf("ragflow_src.%s ASC", column))
	}
	return " ORDER BY " + strings.Join(order, ", ")
}

func (c *BigQueryConnector) validateSchema(schema []bigQueryField) error {
	names := map[string]struct{}{}
	for _, field := range schema {
		names[field.Name] = struct{}{}
	}
	required := map[string]struct{}{}
	for _, column := range c.contentColumns {
		required[column] = struct{}{}
	}
	optional := map[string]struct{}{}
	for _, column := range c.metadataColumns {
		optional[column] = struct{}{}
	}
	if c.idColumn != "" {
		optional[c.idColumn] = struct{}{}
	}
	if c.timestampColumn != "" {
		optional[c.timestampColumn] = struct{}{}
	}
	var missing []string
	for column := range required {
		if _, ok := names[column]; !ok {
			missing = append(missing, column)
		}
	}
	for column := range optional {
		if _, ok := names[column]; !ok {
			missing = append(missing, column)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return &ConnectorValidationError{Message: fmt.Sprintf("BigQuery configured columns not found in schema: %s", strings.Join(missing, ", "))}
	}
	if c.timestampColumn != "" {
		if _, ok := names[c.timestampColumn]; !ok {
			return &ConnectorValidationError{Message: fmt.Sprintf("BigQuery timestamp column '%s' was not found in the schema.", c.timestampColumn)}
		}
	}
	return nil
}

// buildSyncQuery returns the wrapped query and cursor parameters.
func (c *BigQueryConnector) buildSyncQuery(request SyncRequest) (string, []gcpbigquery.QueryParameter) {
	base := c.buildBaseQuery()
	if c.timestampColumn == "" || request.FromBeginning {
		return c.wrapQuery(base, "*"), nil
	}
	conditions := []string{}
	parameters := []gcpbigquery.QueryParameter{}
	if request.WindowStart != nil {
		conditions = append(conditions, fmt.Sprintf("ragflow_src.%s >= @start_cursor", c.timestampColumn))
		parameters = append(parameters, gcpbigquery.QueryParameter{Name: "start_cursor", Value: request.WindowStart.UTC()})
	}
	if !request.WindowEnd.IsZero() {
		conditions = append(conditions, fmt.Sprintf("ragflow_src.%s <= @end_cursor", c.timestampColumn))
		parameters = append(parameters, gcpbigquery.QueryParameter{Name: "end_cursor", Value: request.WindowEnd.UTC()})
	}
	if len(conditions) == 0 {
		return c.wrapQuery(base, "*"), nil
	}
	return c.wrapFilteredQuery(base, "*", strings.Join(conditions, " AND ")), parameters
}

func (c *BigQueryConnector) buildSlimQuery(base string) string {
	columns := c.contentColumns
	if c.idColumn != "" {
		columns = []string{c.idColumn}
	}
	if len(columns) == 0 {
		return c.wrapQuery(base, "*")
	}
	selects := make([]string, 0, len(columns))
	for _, column := range columns {
		selects = append(selects, fmt.Sprintf("ragflow_src.%s", column))
	}
	return c.wrapQuery(base, strings.Join(selects, ", "))
}

func (c *BigQueryConnector) splitColumns(value any) []string {
	switch typed := value.(type) {
	case string:
		parts := strings.Split(typed, ",")
		columns := make([]string, 0, len(parts))
		for _, part := range parts {
			if column := strings.TrimSpace(part); column != "" {
				columns = append(columns, column)
			}
		}
		return columns
	case []any:
		columns := make([]string, 0, len(typed))
		for _, item := range typed {
			if column := strings.TrimSpace(stringConfig(item)); column != "" {
				columns = append(columns, column)
			}
		}
		return columns
	}
	return nil
}

func (c *BigQueryConnector) contentColumnsForRow(row map[string]any, schema []bigQueryField) []string {
	if len(c.contentColumns) > 0 {
		return c.contentColumns
	}
	excluded := map[string]bool{}
	if c.idColumn != "" {
		excluded[c.idColumn] = true
	}
	if c.timestampColumn != "" {
		excluded[c.timestampColumn] = true
	}
	columns := make([]string, 0, len(schema))
	for _, field := range schema {
		if _, ok := row[field.Name]; ok && !excluded[field.Name] {
			columns = append(columns, field.Name)
		}
	}
	return columns
}

func (c *BigQueryConnector) buildContent(row map[string]any, columns []string) string {
	parts := []string{}
	for _, column := range columns {
		value, ok := row[column]
		if !ok || value == nil {
			continue
		}
		rendered := renderBigQueryContentValue(value)
		if rendered == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("【%s】:\n%s", column, rendered))
	}
	return strings.Join(parts, "\n\n")
}

func (c *BigQueryConnector) buildDocumentID(row map[string]any) string {
	prefix := c.idPrefix()
	if c.idColumn != "" {
		if value, ok := row[c.idColumn]; ok && value != nil {
			return prefix + ":" + bigQueryValueString(value)
		}
	}
	content := c.buildContent(row, c.contentColumns)
	sum := md5.Sum([]byte(content))
	return prefix + ":" + hex.EncodeToString(sum[:])
}

func (c *BigQueryConnector) idPrefix() string {
	if c.usesTableMode() {
		return fmt.Sprintf("bigquery:%s:%s.%s", c.projectID, c.datasetID, c.tableID)
	}
	return fmt.Sprintf("bigquery:%s:query", c.projectID)
}

func (c *BigQueryConnector) rowToSourceDocument(row map[string]any, schema []bigQueryField) (SourceDocument, bool) {
	contentColumns := c.contentColumnsForRow(row, schema)
	content := c.buildContent(row, contentColumns)

	metadata := map[string]any{}
	for _, column := range c.metadataColumns {
		value, ok := row[column]
		if !ok || value == nil {
			continue
		}
		metadata[column] = renderBigQueryMetadataValue(value)
	}

	updatedAt := time.Now().UTC()
	if c.timestampColumn != "" {
		if value, ok := row[c.timestampColumn]; ok && value != nil {
			updatedAt = parseBigQueryUpdatedAt(value)
		}
	}

	semanticID := "bigquery_record"
	if len(contentColumns) > 0 {
		if value, ok := row[contentColumns[0]]; ok && value != nil {
			semanticID = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(bigQueryValueString(value), "\n", " "), "\r", " "))
			if semanticID == "" {
				semanticID = "bigquery_record"
			} else if len(semanticID) > 100 {
				semanticID = semanticID[:100]
			}
		}
	}

	sourceID := c.buildDocumentID(row)
	blob := []byte(content)
	return SourceDocument{
		SourceID:           sourceID,
		SemanticIdentifier: semanticID,
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint: stableFingerprint(map[string]any{
			"id":       sourceID,
			"content":  content,
			"metadata": metadata,
		}),
	}, true
}

func renderBigQueryContentValue(value any) string {
	switch typed := value.(type) {
	case []byte:
		return ""
	case time.Time:
		return typed.Format(time.RFC3339)
	case civil.Date:
		return typed.String()
	case civil.Time:
		return typed.String()
	case civil.DateTime:
		return typed.String()
	case *big.Rat:
		return typed.RatString()
	case map[string]any:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	case []any:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	default:
		return fmt.Sprint(value)
	}
}

func renderBigQueryMetadataValue(value any) string {
	switch typed := value.(type) {
	case []byte:
		return base64.StdEncoding.EncodeToString(typed)
	case time.Time:
		return typed.Format(time.RFC3339)
	case civil.Date:
		return typed.String()
	case civil.Time:
		return typed.String()
	case civil.DateTime:
		return typed.String()
	case *big.Rat:
		return typed.RatString()
	case map[string]any:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	case []any:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	default:
		return fmt.Sprint(value)
	}
}

func parseBigQueryUpdatedAt(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC()
	case civil.DateTime:
		return typed.In(time.UTC)
	case civil.Date:
		return time.Date(typed.Year, typed.Month, typed.Day, 0, 0, 0, 0, time.UTC)
	}
	return time.Now().UTC()
}

func bigQueryValueString(value any) string {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format(time.RFC3339)
	case civil.Date:
		return typed.String()
	case civil.Time:
		return typed.String()
	case civil.DateTime:
		return typed.String()
	case *big.Rat:
		return typed.RatString()
	case map[string]any:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	case []any:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	default:
		return fmt.Sprint(value)
	}
}

func waitBigQueryJob(ctx context.Context, query bigQueryQuery) error {
	job, err := query.Run(ctx)
	if err != nil {
		return err
	}
	status, err := job.Wait(ctx)
	if err != nil {
		return err
	}
	if !status.Done() {
		return fmt.Errorf("BigQuery query did not complete")
	}
	if err := status.Err(); err != nil {
		return err
	}
	return nil
}

func toBigQueryFields(schema gcpbigquery.Schema) []bigQueryField {
	if len(schema) == 0 {
		return nil
	}
	fields := make([]bigQueryField, 0, len(schema))
	for _, field := range schema {
		if field == nil {
			continue
		}
		fields = append(fields, bigQueryField{Name: field.Name, Type: string(field.Type)})
	}
	return fields
}

func sanitizeBigQueryQuery(raw string) string {
	query := strings.TrimSpace(raw)
	if query == "" {
		return ""
	}
	if strings.HasPrefix(query, "```") {
		query = strings.TrimPrefix(query, "```")
		if strings.HasSuffix(query, "```") {
			query = strings.TrimSuffix(query, "```")
		}
		if head, tail, ok := strings.Cut(query, "\n"); ok {
			head = strings.TrimSpace(head)
			if head == "sql" || head == "google-sql" || head == "bigquery" {
				head = ""
			}
			query = strings.TrimSpace(head) + "\n" + strings.TrimSpace(tail)
		}
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(query), ";"))
}

// serializeBigQueryCursor mirrors Python's cursor serialization used for
// connector config persistence. Only temporal values need a marker wrapper.
func serializeBigQueryCursor(value any) any {
	switch typed := value.(type) {
	case time.Time:
		return map[string]any{bigQueryCursorTypeKey: "datetime", "value": typed.Format(time.RFC3339)}
	case civil.Date:
		return map[string]any{bigQueryCursorTypeKey: "date", "value": typed.String()}
	case civil.Time:
		return map[string]any{bigQueryCursorTypeKey: "time", "value": typed.String()}
	case civil.DateTime:
		return map[string]any{bigQueryCursorTypeKey: "datetime", "value": typed.String()}
	case *big.Rat:
		return map[string]any{bigQueryCursorTypeKey: "decimal", "value": typed.RatString()}
	default:
		return value
	}
}

func deserializeBigQueryCursor(value any) any {
	typed, ok := value.(map[string]any)
	if !ok {
		return value
	}
	raw, ok := typed[bigQueryCursorTypeKey].(string)
	if !ok {
		return value
	}
	switch raw {
	case "datetime":
		if parsed, err := time.Parse(time.RFC3339, fmt.Sprint(typed["value"])); err == nil {
			return parsed
		}
		if parsed, err := time.Parse("2006-01-02 15:04:05", fmt.Sprint(typed["value"])); err == nil {
			return parsed
		}
	case "date":
		if parsed, err := time.Parse("2006-01-02", fmt.Sprint(typed["value"])); err == nil {
			return parsed
		}
	case "time":
		if parsed, err := civil.ParseTime(fmt.Sprint(typed["value"])); err == nil {
			return parsed
		}
	case "decimal":
		if parsed, ok := new(big.Rat).SetString(fmt.Sprint(typed["value"])); ok {
			return parsed
		}
	}
	return value
}

func configInt64(value any, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

type bigQuerySyncSession struct {
	connector *BigQueryConnector
	client    bigQueryClient
	rows      bigQueryRowIterator
	batchSize int
	resume    *SyncCheckpoint
	closed    bool
}

func (s *bigQuerySyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.closed {
		return SyncBatch{}, io.EOF
	}
	documents := make([]SourceDocument, 0, s.batchSize)
	for len(documents) < s.batchSize {
		var row map[string]any
		err := s.rows.Next(&row)
		if errors.Is(err, io.EOF) {
			if len(documents) == 0 {
				if s.resume != nil {
					return SyncBatch{}, fmt.Errorf("bigquery resume anchor %q was not found in the current result: %w", s.resume.SourceID, ErrSyncResumeInvalid)
				}
				return SyncBatch{}, io.EOF
			}
			break
		}
		if err != nil {
			return SyncBatch{}, err
		}
		doc, ok := s.connector.rowToSourceDocument(row, s.rows.Schema())
		if !ok {
			continue
		}
		if !s.includeResumed(doc) {
			if s.resume != nil && doc.SourceID == s.resume.SourceID {
				// The anchor was found; continue emitting rows after it.
			}
			continue
		}
		documents = append(documents, doc)
	}
	if len(documents) == 0 {
		return SyncBatch{}, io.EOF
	}
	return SyncBatch{Documents: documents, Checkpoint: bigQuerySyncCheckpoint(documents[len(documents)-1])}, nil
}

func (s *bigQuerySyncSession) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.client.Close()
}

func (s *bigQuerySyncSession) includeResumed(doc SourceDocument) bool {
	if s.resume == nil || s.resume.SourceID == "" {
		return true
	}
	if doc.SourceID == s.resume.SourceID {
		s.resume = nil
		return false
	}
	return false
}

func (s *bigQuerySyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("bigquery sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if s.connector.stableOrderClause() == "" {
		return fmt.Errorf("bigquery sync resume requires a stable ordering: %w", ErrSyncResumeInvalid)
	}
	s.resume = &SyncCheckpoint{SourceID: sourceID}
	return nil
}

func bigQuerySyncCheckpoint(document SourceDocument) *SyncCheckpoint {
	updatedAt := document.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    document.SourceID,
		SourceID:  document.SourceID,
		UpdatedAt: &updatedAt,
	}
}

type bigQueryPruneSession struct {
	connector *BigQueryConnector
	client    bigQueryClient
	rows      bigQueryRowIterator
	batchSize int
	closed    bool
}

func (s *bigQueryPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.closed {
		return PruneBatch{}, io.EOF
	}
	documents := make([]SlimDocument, 0, s.batchSize)
	for len(documents) < s.batchSize {
		var row map[string]any
		err := s.rows.Next(&row)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return PruneBatch{}, err
		}
		documents = append(documents, SlimDocument{SourceID: s.connector.buildDocumentID(row)})
	}
	if len(documents) == 0 {
		return PruneBatch{}, io.EOF
	}
	return PruneBatch{Documents: documents}, nil
}

func (s *bigQueryPruneSession) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.client.Close()
}
