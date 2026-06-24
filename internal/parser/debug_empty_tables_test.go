//go:build cgo && integration

package parser

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestDebugEmptyTables(t *testing.T) {
	client := mustConnectOssDeepDoc(t)

	pdfs := []string{
		"13_crosspage_table.pdf",
		"14_text_table_interleaved.pdf",
		"table_rotation_test.pdf",
	}

	for _, name := range pdfs {
		fmt.Printf("\n══════════════════════ %s ══════════════════════\n", name)

		eng := mustOpenEngine(t, name)
		cfg := DefaultParserConfig()
		cfg.TableBuilder = NewOssDeepDocTableBuilder(client)
		p := NewParser(cfg, client)
			result, err := p.Parse(context.Background(), eng)
		eng.Close()
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			continue
		}

		// Print TSR debug data.
		fmt.Printf("  TSRRawCell count: %d\n", len(result.TSRDebug))
		for i, rc := range result.TSRDebug {
			if i < 20 {
				fmt.Printf("    [%d] table=%d page=%d label=%q bbox=(%.0f,%.0f,%.0f,%.0f)\n",
					i, rc.TableIndex, rc.Page, rc.Label, rc.X0, rc.Y0, rc.X1, rc.Y1)
			}
		}
		if len(result.TSRDebug) > 20 {
			fmt.Printf("    ... and %d more\n", len(result.TSRDebug)-20)
		}

		// Table details.
		fmt.Printf("  Tables: %d\n", len(result.Tables))
		for i, tbl := range result.Tables {
			fmt.Printf("  Table[%d]:\n", i)
			fmt.Printf("    Cells: %d\n", len(tbl.Cells))
			fmt.Printf("    Grid: %d rows\n", len(tbl.Grid))
			if len(tbl.Grid) > 0 {
				for ri, row := range tbl.Grid {
					if ri < 5 {
						fmt.Printf("      row[%d]: %d cols\n", ri, len(row))
						for ci, cell := range row {
							if ci < 5 {
								fmt.Printf("        [%d,%d] bbox=(%.0f,%.0f,%.0f,%.0f) label=%q text=%q\n",
									ri, ci, cell.X0, cell.Y0, cell.X1, cell.Y1, cell.Label, cell.Text)
							}
						}
					}
				}
				if len(tbl.Grid) > 5 {
					fmt.Printf("      ... and %d more rows\n", len(tbl.Grid)-5)
				}
			}
			fmt.Printf("    Rows: %d\n", len(tbl.Rows))
			if len(tbl.Rows) > 0 {
				fmt.Printf("    Row sample: %v\n", tbl.Rows[0])
			}
			fmt.Printf("    Caption: %q\n", tbl.Caption)
			// Check if ImageB64 is valid.
			if tbl.ImageB64 != "" {
				fmt.Printf("    ImageB64: %d chars\n", len(tbl.ImageB64))
			}
		}

		// Sections related to tables.
		for _, s := range result.Sections {
			if s.LayoutType == "table" || strings.Contains(s.LayoutType, "table") {
				txt := strings.TrimSpace(s.Text)
				if len(txt) > 100 {
					txt = txt[:100]
				}
				fmt.Printf("  Table section: type=%q text=%q\n", s.LayoutType, txt)
			}
		}
	}
}


