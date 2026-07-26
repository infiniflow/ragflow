package tool

// BatchResult stores per-PDF pipeline stage output.
type BatchResult struct {
	File          string  `json:"file"`
	Pages         int     `json:"pages"`
	Chars         int     `json:"chars"`
	BoxesInitial  int     `json:"boxes_initial"`
	BoxesTextMerg int     `json:"boxes_text_merge"`
	BoxesVertMerg int     `json:"boxes_vertical_merge"`
	Sections      int     `json:"sections"`
	TSTables      int     `json:"tsr_tables,omitempty"`
	TextLen       int     `json:"text_len"`
	TimeS         float64 `json:"time_s"`
	Error         string  `json:"error,omitempty"`
}

// PyResult mirrors Python dump_py_results.py output.
type PyResult struct {
	File           string  `json:"file"`
	Pages          int     `json:"pages"`
	Chars          int     `json:"chars"`
	BoxesInitial   int     `json:"boxes_initial"`
	BoxesTextMerge int     `json:"boxes_text_merge"`
	BoxesVertMerge int     `json:"boxes_vertical_merge"`
	Sections       int     `json:"sections"`
	Tables         int     `json:"tables"`
	TextLen        int     `json:"text_len"`
	IsEnglish      *bool   `json:"is_english"`
	TimeS          float64 `json:"time_s"`
	Error          string  `json:"error,omitempty"`
}

// TableItem stores per-table output.
type TableItem struct {
	ImageB64  string     `json:"image_b64"`
	Rows      [][]string `json:"rows"`
	Cells     []TSRCell  `json:"cells,omitempty"`
	Positions []Position `json:"positions"`
}

// TSRCell mirrors parser.TSRCell for serialization.
type TSRCell struct {
	X0    float64 `json:"x0"`
	Y0    float64 `json:"y0"`
	X1    float64 `json:"x1"`
	Y1    float64 `json:"y1"`
	Text  string  `json:"text"`
	Label string  `json:"label"`
}

// Position stores a bounding box.
type Position struct {
	Left, Right, Top, Bottom float64
}

// RealPDFResult holds per-PDF stats for Go vs Python comparison.
type RealPDFResult struct {
	File     string `json:"file"`
	Pages    int    `json:"pages"`
	Chars    int    `json:"chars"`
	Sections int    `json:"sections"`
	TextLen  int    `json:"text_len"`
	Error    string `json:"error,omitempty"`
}

// TLogger is a minimal interface for logging in comparison functions.
type TLogger interface {
	Logf(format string, args ...any)
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Skipf(format string, args ...any)
}
