package service

import (
	"encoding/hex"

	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/utility"

	"github.com/zeebo/xxh3"
)

// selectUploadParser resolves the parser_id for a document by delegating to
// the builtin pipeline registry, which owns the file-type-to-parser mapping.
func selectUploadParser(docType utility.FileType, filename, defaultParser string) string {
	registry, err := pipelinepkg.DefaultRegistry()
	if err != nil || registry == nil {
		return defaultParser
	}
	return registry.DefaultParserID(string(docType), filename, defaultParser)
}

// contentHashHex mirrors Python xxhash.xxh128(blob).hexdigest().
func contentHashHex(blob []byte) string {
	sum := xxh3.Hash128(blob).Bytes()
	return hex.EncodeToString(sum[:])
}
