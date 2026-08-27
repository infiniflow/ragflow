//go:build cgo_thincner

package extractor

/*
#include <stdlib.h>
#include "../../../binding/cpp/thinc_parser.h"
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"unsafe"
)

// DepToken holds dependency parse info for one token (mirrors Go dep_relation.go)
type DepTokenC struct {
	Text  string `json:"text"`
	Head  int    `json:"head"`
	Dep   string `json:"dep"`
	Index int    `json:"index"`
}

// RunParser runs the C++ dependency parser on tokenized text.
// modelBaseDir: path to model directory containing ner/ and parser/ subdirectories.
// tokensJSON: JSON array of token strings.
// Returns JSON array of DepTokenC.
func RunParser(nerDir, parserDir string, tokensJSON string) (string, error) {
	cNer := C.CString(nerDir)
	cParser := C.CString(parserDir)
	cTokens := C.CString(tokensJSON)
	defer C.free(unsafe.Pointer(cNer))
	defer C.free(unsafe.Pointer(cParser))
	defer C.free(unsafe.Pointer(cTokens))

	handle := C.ThincParser_Create(cNer, cParser)
	if handle == nil {
		return "", fmt.Errorf("failed to create ThincParser handle")
	}
	defer C.ThincParser_Destroy(handle)

	cResult := C.ThincParser_Predict(handle, cTokens)
	if cResult == nil {
		return "", fmt.Errorf("parser prediction failed")
	}
	defer C.ThincParser_FreeString(cResult)

	return C.GoString(cResult), nil
}

// RunTagger runs the C++ POS tagger.
// nerDir: path to NER model directory (for tok2vec weights).
// taggerDir: path to tagger model directory.
func RunTagger(nerDir, taggerDir string, tokensJSON string) (string, error) {
	cNer := C.CString(nerDir)
	cTagger := C.CString(taggerDir)
	cTokens := C.CString(tokensJSON)
	defer C.free(unsafe.Pointer(cNer))
	defer C.free(unsafe.Pointer(cTagger))
	defer C.free(unsafe.Pointer(cTokens))

	handle := C.ThincTagger_Create(cNer, cTagger)
	if handle == nil {
		return "", fmt.Errorf("failed to create ThincTagger handle")
	}
	defer C.ThincTagger_Destroy(handle)

	cResult := C.ThincTagger_Predict(handle, cTokens)
	if cResult == nil {
		return "", fmt.Errorf("tagger prediction failed")
	}
	defer C.ThincTagger_FreeString(cResult)

	return C.GoString(cResult), nil
}

// ParseTokensWithParser runs the C++ parser and returns parsed DepToken slice.
func ParseTokensWithParser(nerDir, parserDir string, tokens []string) ([]DepTokenC, error) {
	tj, _ := json.Marshal(tokens)
	resultJSON, err := RunParser(nerDir, parserDir, string(tj))
	if err != nil {
		return nil, err
	}
	var tokensC []DepTokenC
	if err := json.Unmarshal([]byte(resultJSON), &tokensC); err != nil {
		return nil, fmt.Errorf("parse result: %w", err)
	}
	return tokensC, nil
}
