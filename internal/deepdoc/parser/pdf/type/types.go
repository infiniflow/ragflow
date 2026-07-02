// Package pdftype provides PDF-specific types and re-exports shared types
// from the doctype package via Go type aliases.  Existing PDF parser code
// that imports this package continues to work without changes.
package pdftype

import doctype "ragflow/internal/deepdoc/parser/type"

// ── Re-export shared types via aliases ─────────────────────────────────────

type PipelineMetrics = doctype.PipelineMetrics
type ParseResult = doctype.ParseResult
type DLAPageRegions = doctype.DLAPageRegions
type TSRRawCell = doctype.TSRRawCell
type TextChar = doctype.TextChar
type TextBox = doctype.TextBox
type Position = doctype.Position
type Section = doctype.Section
type TableItem = doctype.TableItem
type TSRCell = doctype.TSRCell
type DLARegion = doctype.DLARegion
type OCRBox = doctype.OCRBox
type OCRText = doctype.OCRText
type ParserConfig = doctype.ParserConfig
type DocAnalyzer = doctype.DocAnalyzer
type Outline = doctype.Outline
type PDFEngine = doctype.PDFEngine
type Tokenizer = doctype.Tokenizer
type SampleFunc = doctype.SampleFunc
type TableBuilder = doctype.TableBuilder
type Rectangular = doctype.Rectangular

// ── Re-export constants ────────────────────────────────────────────────────

const DlaDPI = doctype.DlaDPI
const DlaScale = doctype.DlaScale

const (
	LayoutTypeText        = doctype.LayoutTypeText
	LayoutTypeTable       = doctype.LayoutTypeTable
	LayoutTypeFigure      = doctype.LayoutTypeFigure
	LayoutTypeEquation    = doctype.LayoutTypeEquation
	LayoutTypeTitle       = doctype.LayoutTypeTitle
	LayoutTypeReference   = doctype.LayoutTypeReference
	LayoutTypeFooter      = doctype.LayoutTypeFooter
	LayoutTypeHeader      = doctype.LayoutTypeHeader
	DLALabelFigureCaption = doctype.DLALabelFigureCaption
	DLALabelTableCaption  = doctype.DLALabelTableCaption
)

// ── Re-export functions and variables ──────────────────────────────────────

var (
	CollectFigures      = doctype.CollectFigures
	DefaultParserConfig = doctype.DefaultParserConfig
	IsCJK               = doctype.IsCJK
)
