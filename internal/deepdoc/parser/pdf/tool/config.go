package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Count         int
	Single        string
	SkipOCR       bool // DLA+TSR but no image OCR
	CompareOnly   bool
	CompareFilter string
	CSVOutput     string
	GoTextDir     string
	PyTextDir     string
	TablesDir     string
	GoSuffix      string
}

func LoadConfig() Config {
	goVariant := "ocr"
	pyVariant := "ocr"
	td := filepath.Join("testdata")
	return Config{
		Count:         envInt("BATCH_COUNT", 0),
		Single:        os.Getenv("BATCH_SINGLE"),
		SkipOCR:       os.Getenv("BATCH_SKIP_OCR") == "1",
		CompareOnly:   os.Getenv("BATCH_COMPARE_ONLY") == "1",
		CompareFilter: os.Getenv("BATCH_COMPARE_FILTER"),
		CSVOutput:     envStr("BATCH_COMPARE_CSV", filepath.Join(td, "output", fmt.Sprintf("compare_%s.csv", time.Now().Format("20060102_150405")))),
		GoTextDir:     filepath.Join(td, "output", "go", goVariant, "text"),
		PyTextDir:     filepath.Join(td, "output", "py", pyVariant, "text"),
		TablesDir:     filepath.Join(td, "output", "go", goVariant, "tables"),
		GoSuffix:      goVariant,
	}
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envStr(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

// FileExists returns true if the path exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
