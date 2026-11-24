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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

func validateJiraBulkIssues(requested []string, page *jiraSearchPage) error {
	if len(page.IssueErrors) > 0 {
		return fmt.Errorf("Jira bulk fetch reported %d issue errors; the page is incomplete", len(page.IssueErrors))
	}
	remaining := make(map[string]bool, len(requested))
	for _, id := range requested {
		if remaining[id] {
			return fmt.Errorf("Jira search returned a duplicate issue ID")
		}
		remaining[id] = true
	}
	for _, issue := range page.Issues {
		if !remaining[issue.ID] {
			return fmt.Errorf("Jira bulk fetch returned a duplicate or unexpected issue")
		}
		delete(remaining, issue.ID)
	}
	if len(remaining) > 0 {
		return fmt.Errorf("Jira bulk fetch omitted %d of %d requested issues", len(remaining), len(requested))
	}
	return nil
}

func validateJiraCloudPage(page jiraSearchPage, previousToken string) error {
	if page.NextPageToken != "" && page.NextPageToken == previousToken {
		return fmt.Errorf("Jira pagination token did not advance")
	}
	if page.IsLast != nil && !*page.IsLast && page.NextPageToken == "" {
		return fmt.Errorf("Jira non-final page is missing the next pagination token")
	}
	return nil
}

func advanceJiraServerPage(page jiraSearchPage, startAt, requested int) (int, bool, error) {
	if page.StartAt != nil && *page.StartAt != startAt {
		return startAt, false, fmt.Errorf("Jira returned page offset %d instead of %d", *page.StartAt, startAt)
	}
	next := startAt + len(page.Issues)
	if page.Total != nil {
		if *page.Total < 0 || len(page.Issues) == 0 && next < *page.Total {
			return startAt, false, fmt.Errorf("Jira returned an empty page before all issues were read")
		}
		return next, next >= *page.Total, nil
	}
	return next, len(page.Issues) < requested, nil
}

func (c *JiraConnector) completeComments(ctx context.Context, issue *jiraIssue) error {
	block := &issue.Fields.Comment
	if !c.includeComments || block.Total == nil {
		return nil
	}
	total := *block.Total
	if total < 0 || block.StartAt < 0 || block.StartAt+len(block.Comments) > total {
		return fmt.Errorf("Jira issue %s returned invalid comment pagination", issue.Key)
	}
	if block.StartAt != 0 {
		block.Comments = nil
	}
	seen := map[string]bool{}
	for _, comment := range block.Comments {
		if comment.ID == "" || seen[comment.ID] {
			return fmt.Errorf("Jira issue %s returned missing or duplicate comment IDs", issue.Key)
		}
		seen[comment.ID] = true
	}
	for len(block.Comments) < total {
		start := len(block.Comments)
		query := url.Values{"startAt": {strconv.Itoa(start)}, "maxResults": {strconv.Itoa(jiraMaxFullPageSize)}}
		var page jiraCommentBlock
		if _, err := c.doJSON(ctx, http.MethodGet, "issue/"+url.PathEscape(issue.Key)+"/comment", query, nil, &page); err != nil {
			return fmt.Errorf("read comments for Jira issue %s: %w", issue.Key, err)
		}
		if page.Total == nil || *page.Total != total || page.StartAt != start || len(page.Comments) == 0 || start+len(page.Comments) > total {
			return fmt.Errorf("Jira issue %s comment listing changed or returned an incomplete page", issue.Key)
		}
		for _, comment := range page.Comments {
			if comment.ID == "" || seen[comment.ID] {
				return fmt.Errorf("Jira issue %s returned missing or duplicate comment IDs", issue.Key)
			}
			seen[comment.ID] = true
		}
		block.Comments = append(block.Comments, page.Comments...)
	}
	block.StartAt = 0
	return nil
}
