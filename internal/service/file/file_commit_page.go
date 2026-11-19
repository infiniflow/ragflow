//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package file

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/storage"

	"go.uber.org/zap"
)

// PageContentAt is one wiki page as it stood at an anchor: which page, and the
// body it held then.
//
// The body is read from object storage on the way out rather than returned as a
// location, because the only caller replays it through the document engine and
// has no business knowing where the history keeps its bytes.
type PageContentAt struct {
	PageType string
	Slug     string
	Title    string
	Content  string
	// Page is the whole page as the commit stored it: content and the fields a
	// restore has to write back with it (summary, topic, entities, links, source
	// ids). A commit written before snapshots existed carries only content, which
	// is still a usable page - the rest is simply absent.
	Page map[string]any
}

// wikiPageCommitRow is one page's latest commit at or before the anchor, as the
// join between the commit (which holds the time and the title) and its item
// (which holds the page identity and the body pointer) returns it.
type wikiPageCommitRow struct {
	FileID               string
	PageTypeKwd          sql.NullString
	SlugKwd              sql.NullString
	Title                sql.NullString
	Operation            sql.NullString
	ContentAfterLocation sql.NullString
}

// PageContentsAt returns the body each wiki page of the dataset held at
// atMillis, one entry per page.
//
// It answers the question a restore asks: "what did the compiled pages say at
// the moment this version was taken". A page edited after the anchor resolves to
// the last edit at or before it; a page created after the anchor has no commit
// there and is therefore absent - the caller puts back what existed, and a page
// the version never saw is not something the version can describe.
//
// It is PageContentsUntil without an upper bound; a caller restoring a version
// should pass the anchor of the version that follows it, because a run writes its
// version row before it finishes writing what the version describes (see there).
//
// A page whose content was never stored (the commit recorded the diff but the
// object write failed, which the writer tolerates) is skipped with a warning
// rather than failing the whole replay: the rest of the pages can still come
// back, and refusing them all would make one bad commit restore-proof.
//
// The anchor compares the millisecond column, not the date: an edit and the
// version that records it can share a second, and the date column cannot order
// them (see entity.DBTimeMillis).
func (s *FileCommitService) PageContentsAt(ctx context.Context, datasetID string, atMillis int64) ([]PageContentAt, error) {
	return s.PageContentsUntil(ctx, datasetID, atMillis, 0)
}

// PageContentsUntil is PageContentsAt bounded above by untilMillis: the anchor of
// the version that follows the one being restored, or 0 when it is the head.
//
// The upper bound is what makes a page produced by the version's own run come
// back. A run records its version row when the pipeline reports the parse, and the
// pages that parse compiled are merged a moment later - so their first commit
// lands just after the anchor, and read strictly the page looks like it appeared
// afterwards. The interval a version describes is bounded by the next version:
// anything committed in it that the next version does not name belongs to the run
// this version closes.
//
// Two shapes are told apart there. A page whose earliest commit beyond the anchor
// is an edit existed before it, so the body it held at the anchor is that edit's
// before-side - recovered from its diff, unbounded, because nothing touched the
// page in between so the before-side is the anchor state however late the edit
// is. A page whose earliest commit is an addition was created by it: inside the
// window the creation belongs to this version's run and the added body is what the
// version holds; outside it the page belongs to a later version and is left alone.
func (s *FileCommitService) PageContentsUntil(ctx context.Context, datasetID string, atMillis, untilMillis int64) ([]PageContentAt, error) {
	if datasetID == "" {
		return nil, nil
	}
	var rows []wikiPageCommitRow
	if err := dao.DB.WithContext(ctx).
		Table("file_commit_item AS item").
		Select("item.file_id, item.page_type_kwd, item.slug_kwd, commit_row.title, item.operation, item.content_after_location").
		Joins("JOIN file_commit AS commit_row ON commit_row.id = item.commit_id").
		Where("commit_row.folder_id = ? AND commit_row.create_time IS NOT NULL AND commit_row.create_time <= ?", datasetID, atMillis).
		Order("commit_row.create_time DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	seen := make(map[string]struct{}, len(rows))
	pages := make([]PageContentAt, 0, len(rows))
	for _, row := range rows {
		// The rows arrive newest first, so the first row of a page is the one the
		// page stood at when the version was taken; the older ones describe states
		// before it.
		if _, ok := seen[row.FileID]; ok {
			continue
		}
		seen[row.FileID] = struct{}{}
		// A page whose latest commit at the anchor is a deletion did not exist at
		// that moment: the version describes a dataset without it, and the caller
		// removes it rather than writing a body back.
		if row.Operation.Valid && row.Operation.String == "delete" {
			continue
		}

		pageType, slug, ok := splitWikiPageFileID(datasetID, row)
		if !ok {
			common.Warn("Wiki history: cannot tell which page a commit belongs to",
				zap.String("dataset_id", datasetID),
				zap.String("file_id", row.FileID))
			continue
		}
		if !row.ContentAfterLocation.Valid || row.ContentAfterLocation.String == "" {
			common.Warn("Wiki history: commit holds no stored body",
				zap.String("dataset_id", datasetID),
				zap.String("file_id", row.FileID))
			continue
		}
		if storageImpl == nil {
			common.Warn("Wiki history: object storage is not configured")
			continue
		}
		content, err := storageImpl.Get(ctx, datasetID, row.ContentAfterLocation.String)
		if err != nil {
			common.Warn("Wiki history: failed to read a stored body",
				zap.String("dataset_id", datasetID),
				zap.String("location", row.ContentAfterLocation.String),
				zap.Error(err))
			continue
		}
		page := decodePageSnapshot(content)
		title := row.Title.String
		if stored := metadataStringOf(page, "title_kwd"); stored != "" {
			title = stored
		}
		pages = append(pages, PageContentAt{
			PageType: pageType,
			Slug:     slug,
			Title:    title,
			Content:  metadataStringOf(page, "content_with_weight"),
			Page:     page,
		})
	}
	// Pages with no commit at or before the anchor, whose history starts after
	// it: the edit that begins their history began from exactly the state the
	// version describes, and the addition that begins it is what the version's own
	// run wrote when it lands inside the version's window.
	pages = append(pages, s.pageContentsBeforeFirstCommit(ctx, datasetID, atMillis, untilMillis, seen, storageImpl)...)
	return pages, nil
}

// pageContentsBeforeFirstCommit recovers the pages whose history starts after the
// anchor: the first commit a page has beyond it is what moved the page away from
// the state the version describes.
//
// An edit there was applied to the anchor state, so undoing its diff on the body
// it produced yields the body the version held. An addition there created the
// page, and whether that counts as the version's own page depends on where the
// addition falls: inside the version's window it was written by the run this
// version closes, outside it the page belongs to a later version.
//
// Nothing is invented. A page whose first commit is an addition outside the window
// did not exist at the anchor and is left out, and an edit whose diff does not fit
// the body it stored is skipped rather than applied blind: putting back a body
// nobody recorded is worse than leaving the page as it is.
func (s *FileCommitService) pageContentsBeforeFirstCommit(ctx context.Context, datasetID string, atMillis, untilMillis int64, seen map[string]struct{}, storageImpl storage.Storage) []PageContentAt {
	type firstEditRow struct {
		FileID               string
		PageTypeKwd          sql.NullString
		SlugKwd              sql.NullString
		Title                sql.NullString
		Operation            sql.NullString
		ContentAfterLocation sql.NullString
		Diff                 sql.NullString
		CreateTime           sql.NullInt64
	}
	var rows []firstEditRow
	if err := dao.DB.WithContext(ctx).
		Table("file_commit_item AS item").
		Select("item.file_id, item.page_type_kwd, item.slug_kwd, commit_row.title, item.operation, item.content_after_location, item.diff, commit_row.create_time").
		Joins("JOIN file_commit AS commit_row ON commit_row.id = item.commit_id").
		Where("commit_row.folder_id = ? AND commit_row.create_time IS NOT NULL AND commit_row.create_time > ?", datasetID, atMillis).
		Order("commit_row.create_time ASC").
		Scan(&rows).Error; err != nil {
		common.Warn("Wiki history: failed to read the edits after the anchor",
			zap.String("dataset_id", datasetID),
			zap.Error(err))
		return nil
	}
	if len(rows) == 0 {
		return nil
	}

	pages := make([]PageContentAt, 0, len(rows))
	handled := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		// The rows arrive oldest first: the first row of a page is the earliest
		// commit after the anchor, and the one that began from the version's state.
		if _, ok := seen[row.FileID]; ok {
			continue
		}
		if _, ok := handled[row.FileID]; ok {
			continue
		}
		handled[row.FileID] = struct{}{}
		added := row.Operation.Valid && row.Operation.String == "add"
		if added {
			// The page was created by this commit. It is the version's page only
			// when the creation belongs to the run this version closes.
			if untilMillis <= 0 || !row.CreateTime.Valid || row.CreateTime.Int64 >= untilMillis {
				continue
			}
		} else if !row.Diff.Valid || row.Diff.String == "" {
			// An edit with no diff cannot be undone; an addition needs none.
			continue
		}
		pageType, slug, ok := splitWikiPageFileID(datasetID, wikiPageCommitRow{
			FileID:      row.FileID,
			PageTypeKwd: row.PageTypeKwd,
			SlugKwd:     row.SlugKwd,
		})
		if !ok {
			continue
		}
		var page map[string]any
		after := ""
		if row.ContentAfterLocation.Valid && row.ContentAfterLocation.String != "" {
			if storageImpl == nil {
				common.Warn("Wiki history: object storage is not configured")
				continue
			}
			content, err := storageImpl.Get(ctx, datasetID, row.ContentAfterLocation.String)
			if err != nil {
				common.Warn("Wiki history: failed to read the body an edit produced",
					zap.String("dataset_id", datasetID),
					zap.String("location", row.ContentAfterLocation.String),
					zap.Error(err))
				continue
			}
			page = decodePageSnapshot(content)
			after = metadataStringOf(page, "content_with_weight")
		}
		// An addition's own body is the anchor state; an edit's before-side is.
		content := after
		if !added {
			content, ok = reverseApplyDiff(after, row.Diff.String)
			if !ok {
				common.Warn("Wiki history: cannot recover the body an edit started from",
					zap.String("dataset_id", datasetID),
					zap.String("file_id", row.FileID))
				continue
			}
		}
		if content == "" {
			continue
		}
		if page == nil {
			page = make(map[string]any, 2)
		}
		page["content_with_weight"] = content
		title := row.Title.String
		if stored := metadataStringOf(page, "title_kwd"); stored != "" {
			title = stored
		}
		pages = append(pages, PageContentAt{
			PageType: pageType,
			Slug:     slug,
			Title:    title,
			Content:  content,
			Page:     page,
		})
	}
	return pages
}

// reverseApplyDiff undoes a unified diff on the text it was applied to, giving
// back the text the edit started from.
//
// The after-side body is stored with every commit, so the before-side does not
// have to be: the diff says which lines were removed, and undoing the applied
// side reconstructs them. Every applied line is checked against the stored body
// on the way, so a diff that does not belong to it is refused rather than
// silently producing a mixture of both.
func reverseApplyDiff(newText, diff string) (string, bool) {
	if diff == "" {
		return "", false
	}
	newLines := strings.Split(newText, "\n")
	lines := strings.Split(diff, "\n")
	out := make([]string, 0, len(newLines))
	pos, hunks := 0, 0
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], "@@ ") {
			i++
			continue
		}
		start, ok := diffHunkNewStart(lines[i])
		if !ok || start < pos || start > len(newLines) {
			return "", false
		}
		// The lines the hunk does not cover are unchanged on both sides.
		out = append(out, newLines[pos:start]...)
		pos = start
		i++
		for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") {
			line := lines[i]
			i++
			// A blank element is the split's trailing artefact: a real empty line
			// in a hunk carries its prefix (" ").
			if line == "" {
				continue
			}
			switch line[0] {
			case ' ':
				if pos >= len(newLines) || newLines[pos] != line[1:] {
					return "", false
				}
				out = append(out, line[1:])
				pos++
			case '-':
				out = append(out, line[1:])
			case '+':
				if pos >= len(newLines) || newLines[pos] != line[1:] {
					return "", false
				}
				pos++
			default:
				return "", false
			}
		}
		hunks++
	}
	if hunks == 0 {
		return "", false
	}
	out = append(out, newLines[pos:]...)
	return strings.Join(out, "\n"), true
}

// diffHunkNewStart reads the first new-side line index (0-based) out of a
// "@@ -a,b +c,d @@" header.
func diffHunkNewStart(header string) (int, bool) {
	_, after, ok := strings.Cut(header, "+")
	if !ok {
		return 0, false
	}
	number := after
	if end := strings.IndexAny(number, ", "); end >= 0 {
		number = number[:end]
	}
	start, err := strconv.Atoi(number)
	if err != nil || start < 1 {
		return 0, false
	}
	return start - 1, true
}

// PageKeysWithHistory returns the identity ("<page_type>/<slug>") of every wiki
// page this dataset has ever recorded a commit for.
//
// It is the guard a strict page-set restore needs: a page with no history at all
// predates the history being kept, so a restore cannot say whether the version
// held it. Those pages are left alone - deleting something a version never
// described, because the record never mentioned it, would turn missing history
// into data loss.
func (s *FileCommitService) PageKeysWithHistory(ctx context.Context, datasetID string) (map[string]struct{}, error) {
	keys := make(map[string]struct{})
	if datasetID == "" {
		return keys, nil
	}
	var rows []wikiPageCommitRow
	if err := dao.DB.WithContext(ctx).
		Table("file_commit_item AS item").
		Select("item.file_id, item.page_type_kwd, item.slug_kwd").
		Joins("JOIN file_commit AS commit_row ON commit_row.id = item.commit_id").
		Where("commit_row.folder_id = ?", datasetID).
		Group("item.file_id, item.page_type_kwd, item.slug_kwd").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		pageType, slug, ok := splitWikiPageFileID(datasetID, row)
		if !ok {
			continue
		}
		keys[PageKey(pageType, slug)] = struct{}{}
	}
	return keys, nil
}

// PageKey is the stable identity of a wiki page across the services that name
// one: the page API addresses "<page_type>/<slug>", and the history keys on the
// same pair.
func PageKey(pageType, slug string) string {
	return pageType + "/" + slug
}

// PageDiffOfCommit returns the unified diff a specific page commit recorded.
//
// The diff is the part of a page edit a reader can judge: the commit's stored
// snapshot says what the page became, and the diff says what moved. It lives on
// the commit item, written by the same RecordPageEdit that made the commit, so
// reading it back costs one lookup by id.
//
// A commit the recorder skipped (no change) or one written before diffs were
// stored answers with an empty string, not an error: "no diff recorded" is a
// state the caller renders, not a failure.
func (s *FileCommitService) PageDiffOfCommit(ctx context.Context, commitID string) (string, error) {
	if commitID == "" || dao.DB == nil {
		return "", nil
	}
	items, err := s.commitItemDAO.ListByCommitID(ctx, dao.DB, commitID)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Diff != nil && *item.Diff != "" {
			return *item.Diff, nil
		}
	}
	return "", nil
}

// PageDiffAt returns the diff the page's newest commit at or before atMillis
// recorded: the edit the version at that moment was written for.
//
// It is the fallback for a change entry that names no commit - recorded before
// the entry carried one, or on a deployment whose page history was unavailable
// when the edit happened. The page identity is the one the page API addresses,
// and the anchor compares the millisecond column for the reason PageContentsAt
// does.
func (s *FileCommitService) PageDiffAt(ctx context.Context, datasetID string, atMillis int64, pageType, slug string) (string, error) {
	if datasetID == "" || pageType == "" || slug == "" || dao.DB == nil {
		return "", nil
	}
	var rows []struct {
		Diff sql.NullString
	}
	if err := dao.DB.WithContext(ctx).
		Table("file_commit_item AS item").
		Select("item.diff").
		Joins("JOIN file_commit AS commit_row ON commit_row.id = item.commit_id").
		Where("commit_row.folder_id = ? AND item.file_id = ? AND commit_row.create_time IS NOT NULL AND commit_row.create_time <= ?",
			datasetID, wikiFileID(datasetID, pageType, slug), atMillis).
		Order("commit_row.create_time DESC").
		Limit(1).
		Scan(&rows).Error; err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0].Diff.String, nil
}

// metadataStringOf reads one stored page field as text: a snapshot round-trips
// through JSON, so a value can come back as a string, a number or a list.
func metadataStringOf(page map[string]any, key string) string {
	switch value := page[key].(type) {
	case string:
		return value
	case []byte:
		return string(value)
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

// splitWikiPageFileID recovers the page identity from a commit item: the page
// columns when the row carries them, otherwise the file id the writer composed
// (<dataset>/<page_type>/<slug>). A row that yields neither is reported as
// unreadable instead of being guessed at - replaying a page under the wrong slug
// would overwrite a page with another page's body.
func splitWikiPageFileID(datasetID string, row wikiPageCommitRow) (string, string, bool) {
	if row.PageTypeKwd.Valid && row.SlugKwd.Valid && row.SlugKwd.String != "" {
		pageType := row.PageTypeKwd.String
		slug := row.SlugKwd.String
		// slug_kwd is stored as "page_type/slug"; a bare slug is what the page
		// API addresses, so the prefix is dropped when it matches.
		slug = strings.TrimPrefix(slug, pageType+"/")
		return pageType, slug, true
	}
	// The file id only describes a page of this dataset when it is scoped to it;
	// a row from elsewhere must not be replayed into one of our pages.
	prefix := datasetID + "/"
	if !strings.HasPrefix(row.FileID, prefix) {
		return "", "", false
	}
	pageType, slug, found := strings.Cut(strings.TrimPrefix(row.FileID, prefix), "/")
	if !found || pageType == "" || slug == "" {
		return "", "", false
	}
	return pageType, slug, true
}
