#ifndef THINC_NER_H
#define THINC_NER_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>
#include <stdbool.h>

// ---------------------------------------------------------------------------
// C API for spaCy model inference (en_core_web_sm / zh_core_web_sm)
//
// Loads model.ckpt + model.bin directly from a spaCy model directory.
// ---------------------------------------------------------------------------

typedef void* ThincNERHandle;

// Create / destroy an inference handle for a single spaCy model.
// model_dir: path to the model component directory, e.g.
//   "models/en_core_web_sm-3.7.1/ner/"
//   "models/zh_core_web_sm-3.7.1/ner/"
// Returns NULL on failure.
ThincNERHandle ThincNER_Create(const char* model_ner_dir, const char* model_vocab_dir);
void           ThincNER_Destroy(ThincNERHandle handle);

// Run NER on pre-tokenized text.
// tokens_json: JSON array of token strings, e.g. ["Apple", "Inc.", "was", ...]
// Returns JSON array of entities:
//   [{"text":"Apple Inc.","label":"ORG","start":0,"end":10,"confidence":0.85}, ...]
// Caller must free with ThincNER_FreeString.
char* ThincNER_Predict(ThincNERHandle handle, const char* tokens_json);

// Free a string returned by ThincNER_Predict.
void ThincNER_FreeString(char* ptr);

// Utility: tokenize text using spaCy-compatible rules.
// Returns JSON array of token strings.
char* ThincNER_Tokenize(const char* text, const char* lang);

#ifdef __cplusplus
}
#endif

#endif // THINC_NER_H
