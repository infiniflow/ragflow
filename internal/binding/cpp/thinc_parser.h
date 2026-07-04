#ifndef THINC_PARSER_H
#define THINC_PARSER_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>
#include <stdbool.h>

// ---------------------------------------------------------------------------
// C API for spaCy dependency parser and POS tagger
// ---------------------------------------------------------------------------

typedef void* ThincParserHandle;
typedef void* ThincTaggerHandle;

// Parser: create/destroy
// model_ner_dir: path to NER model directory (shared tok2vec weights + vocab).
// model_parser_dir: path to parser model directory.
ThincParserHandle ThincParser_Create(const char* model_ner_dir, const char* model_parser_dir);
void ThincParser_Destroy(ThincParserHandle handle);

// Run dependency parser on pre-tokenized text.
// tokens_json: JSON array of token strings, e.g. ["Apple", "was", "founded", ...]
// Returns JSON array of token annotations:
//   [{"text":"Apple","head":2,"dep":"nsubjpass","index":0}, ...]
// where head is the index of the head token (0-based), -1 for root.
// Caller must free with ThincParser_FreeString.
char* ThincParser_Predict(ThincParserHandle handle, const char* tokens_json);
void ThincParser_FreeString(char* ptr);

// Tagger: create/destroy
// model_ner_dir: path to NER model directory (for tok2vec weights)
// model_tagger_dir: path to tagger model directory
ThincTaggerHandle ThincTagger_Create(const char* model_ner_dir, const char* model_tagger_dir);
void ThincTagger_Destroy(ThincTaggerHandle handle);

// Run POS tagger on pre-tokenized text.
// tokens_json: JSON array of token strings.
// Returns JSON array: [{"text":"Apple","tag":"NNP","index":0}, ...]
char* ThincTagger_Predict(ThincTaggerHandle handle, const char* tokens_json);
void ThincTagger_FreeString(char* ptr);

#ifdef __cplusplus
}
#endif

#endif // THINC_PARSER_H
