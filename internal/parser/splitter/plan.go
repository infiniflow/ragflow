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

package splitter

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"

	doctype "ragflow/internal/deepdoc/parser/type"
)

// GetPDFPageCount returns the total page count of the given PDF data.
// It returns 0 when the data is empty or invalid.
func GetPDFPageCount(pdfData []byte) int {
	if len(pdfData) == 0 {
		return 0
	}
	r, err := pdf.NewReader(bytes.NewReader(pdfData), int64(len(pdfData)))
	if err != nil {
		return 0
	}
	if n := r.NumPage(); n > 0 && n <= 1_000_000 {
		return n
	}
	return 0
}

var sanitizeTitleRe = regexp.MustCompile(`[\\/*?:"<>|\r\n\t]+`)

func sanitizeTitle(title string) string {
	cleaned := sanitizeTitleRe.ReplaceAllString(title, " ")
	cleaned = strings.TrimSpace(cleaned)
	if len(cleaned) > 40 {
		cleaned = strings.TrimSpace(cleaned[:40])
	}
	return cleaned
}

type rawSection struct {
	start int // 0-indexed inclusive
	end   int // 0-indexed inclusive
	title string
}

// PlanAdaptiveChapters produces an ordered list of non-overlapping, contiguous
// chapter slices covering pages [0, totalPages-1].
//
// Rules:
// 1. If totalPages <= opts.MaxPages or totalPages < opts.ThresholdPages, returns a single chapter covering all pages.
// 2. If outlines exist, uses hierarchical TOC nodes (Level 0 / Level 1) to determine natural chapter boundaries.
// 3. For any section > opts.MaxPages, drills down into sub-headings or windows by opts.MaxPages.
// 4. For small sections, coalesces with adjacent sections up to opts.MaxPages.
// 5. If no outlines exist, falls back to regular windows of opts.MaxPages.
// 6. Guarantees 100% continuous coverage of [0, totalPages-1] with zero gaps and zero duplicates.
func PlanAdaptiveChapters(totalPages int, outlines []doctype.Outline, opts SplitOptions) []ChapterPlan {
	if totalPages <= 0 {
		return nil
	}
	opts = opts.Normalize()

	if totalPages < opts.ThresholdPages || totalPages <= opts.MaxPages {
		return []ChapterPlan{
			{
				Index:     0,
				Title:     "Full Document",
				StartPage: 0,
				EndPage:   totalPages - 1,
				PageCount: totalPages,
			},
		}
	}

	validOutlines := filterAndSortOutlines(outlines, totalPages)
	if len(validOutlines) == 0 {
		return planFixedWindows(totalPages, opts.MaxPages)
	}

	// Determine minimum level in the outline tree.
	minLevel := math.MaxInt
	for _, o := range validOutlines {
		if o.Level < minLevel {
			minLevel = o.Level
		}
	}

	var l1 []doctype.Outline
	for _, o := range validOutlines {
		if o.Level == minLevel {
			l1 = append(l1, o)
		}
	}
	if len(l1) == 0 && minLevel+1 < 10 {
		for _, o := range validOutlines {
			if o.Level == minLevel+1 {
				l1 = append(l1, o)
			}
		}
	}
	if len(l1) == 0 {
		return planFixedWindows(totalPages, opts.MaxPages)
	}

	var rawSections []rawSection
	for i, item := range l1 {
		sp := item.PageNumber - 1
		if i == 0 {
			// Include pre-chapter pages (cover, front matter) in the first chapter.
			sp = 0
		}
		ep := totalPages - 1
		if i+1 < len(l1) {
			nextPage := l1[i+1].PageNumber - 1
			if nextPage > sp {
				ep = nextPage - 1
			} else {
				ep = sp
			}
		}
		if ep < sp {
			ep = sp
		}
		rawSections = append(rawSections, rawSection{
			start: sp,
			end:   ep,
			title: sanitizeTitle(item.Title),
		})
	}

	var slices []rawSection
	var curStart, curEnd *int
	var curTitles []string

	flush := func() {
		if curStart != nil && curEnd != nil {
			title := strings.Join(curTitles, " + ")
			if title == "" {
				title = fmt.Sprintf("Pages %d-%d", *curStart+1, *curEnd+1)
			}
			slices = append(slices, rawSection{
				start: *curStart,
				end:   *curEnd,
				title: title,
			})
			curStart = nil
			curEnd = nil
			curTitles = nil
		}
	}

	for _, sec := range rawSections {
		sp := sec.start
		ep := sec.end
		title := sec.title
		cnt := ep - sp + 1

		if cnt > opts.MaxPages {
			flush()

			// Check for sub-headings within [sp+1, ep+1]
			var subItems []doctype.Outline
			for _, o := range validOutlines {
				if o.Level == minLevel+1 && (sp+1) <= o.PageNumber && o.PageNumber <= (ep+1) {
					subItems = append(subItems, o)
				}
			}

			if len(subItems) > 0 {
				var subStart, subEnd *int
				var subTitles []string
				flushSub := func() {
					if subStart != nil && subEnd != nil {
						sTitle := strings.Join(subTitles, " + ")
						if sTitle == "" {
							sTitle = fmt.Sprintf("Pages %d-%d", *subStart+1, *subEnd+1)
						}
						slices = append(slices, rawSection{
							start: *subStart,
							end:   *subEnd,
							title: sTitle,
						})
						subStart = nil
						subEnd = nil
						subTitles = nil
					}
				}

				for sIdx, sItem := range subItems {
					subSp := sItem.PageNumber - 1
					if sIdx == 0 && subSp > sp {
						subSp = sp
					}
					subEp := ep
					if sIdx+1 < len(subItems) {
						nextSub := subItems[sIdx+1].PageNumber - 1
						if nextSub > subSp {
							subEp = nextSub - 1
						} else {
							subEp = subSp
						}
					}
					if subEp < subSp {
						subEp = subSp
					}
					subTitle := sanitizeTitle(sItem.Title)
					if title != "" && !strings.Contains(subTitle, title) {
						subTitle = title + ": " + subTitle
					}
					subCnt := subEp - subSp + 1

					if subCnt > opts.MaxPages {
						flushSub()
						for w := subSp; w <= subEp; w += opts.MaxPages {
							wEnd := w + opts.MaxPages - 1
							if wEnd > subEp {
								wEnd = subEp
							}
							slices = append(slices, rawSection{
								start: w,
								end:   wEnd,
								title: fmt.Sprintf("%s (P%d-%d)", subTitle, w+1, wEnd+1),
							})
						}
					} else {
						if subStart == nil {
							startVal := subSp
							endVal := subEp
							subStart = &startVal
							subEnd = &endVal
							subTitles = []string{subTitle}
						} else {
							subCurLen := *subEnd - *subStart + 1
							subCombinedLen := subEp - *subStart + 1
							shouldCoalesce := subCombinedLen <= opts.MaxPages && (subCurLen < opts.MinPages || subCnt < opts.MinPages || subCombinedLen <= opts.MinPages)
							if shouldCoalesce {
								*subEnd = subEp
								subTitles = append(subTitles, subTitle)
							} else {
								flushSub()
								startVal := subSp
								endVal := subEp
								subStart = &startVal
								subEnd = &endVal
								subTitles = []string{subTitle}
							}
						}
					}
				}
				flushSub()
				continue
			}

			// No sub-headings: partition section into regular windows
			for w := sp; w <= ep; w += opts.MaxPages {
				wEnd := w + opts.MaxPages - 1
				if wEnd > ep {
					wEnd = ep
				}
				slices = append(slices, rawSection{
					start: w,
					end:   wEnd,
					title: fmt.Sprintf("%s (P%d-%d)", title, w+1, wEnd+1),
				})
			}
			continue
		}

		// cnt <= opts.MaxPages
		if curStart == nil {
			startVal := sp
			endVal := ep
			curStart = &startVal
			curEnd = &endVal
			curTitles = []string{title}
		} else {
			curLen := *curEnd - *curStart + 1
			combinedLen := ep - *curStart + 1
			shouldCoalesce := combinedLen <= opts.MaxPages && (curLen < opts.MinPages || cnt < opts.MinPages || combinedLen <= opts.MinPages)
			if shouldCoalesce {
				*curEnd = ep
				curTitles = append(curTitles, title)
			} else {
				flush()
				startVal := sp
				endVal := ep
				curStart = &startVal
				curEnd = &endVal
				curTitles = []string{title}
			}
		}
	}
	flush()

	// Post-processing: ensure continuous gap-free coverage from 0 to totalPages-1
	if len(slices) == 0 {
		return planFixedWindows(totalPages, opts.MaxPages)
	}

	var plans []ChapterPlan
	curr := 0
	for i, s := range slices {
		start := s.start
		end := s.end
		if i == 0 {
			start = 0
		} else if start != curr {
			start = curr
		}
		if end < start {
			end = start
		}
		if i == len(slices)-1 {
			end = totalPages - 1
		}
		title := s.title
		if title == "" {
			title = fmt.Sprintf("Part %02d", i+1)
		}
		plans = append(plans, ChapterPlan{
			Index:     i,
			Title:     title,
			StartPage: start,
			EndPage:   end,
			PageCount: end - start + 1,
		})
		curr = end + 1
	}

	// Final check: if curr < totalPages, extend final plan
	if curr < totalPages && len(plans) > 0 {
		last := &plans[len(plans)-1]
		last.EndPage = totalPages - 1
		last.PageCount = last.EndPage - last.StartPage + 1
	}

	return plans
}

func planFixedWindows(totalPages, windowSize int) []ChapterPlan {
	var plans []ChapterPlan
	idx := 0
	for start := 0; start < totalPages; start += windowSize {
		end := start + windowSize - 1
		if end >= totalPages {
			end = totalPages - 1
		}
		plans = append(plans, ChapterPlan{
			Index:     idx,
			Title:     fmt.Sprintf("Part %02d (Pages %d-%d)", idx+1, start+1, end+1),
			StartPage: start,
			EndPage:   end,
			PageCount: end - start + 1,
		})
		idx++
	}
	return plans
}

func filterAndSortOutlines(outlines []doctype.Outline, totalPages int) []doctype.Outline {
	var valid []doctype.Outline
	for _, o := range outlines {
		if o.PageNumber > 0 && o.PageNumber <= totalPages {
			valid = append(valid, o)
		}
	}
	sort.SliceStable(valid, func(i, j int) bool {
		if valid[i].PageNumber != valid[j].PageNumber {
			return valid[i].PageNumber < valid[j].PageNumber
		}
		return valid[i].Level < valid[j].Level
	})
	return valid
}
