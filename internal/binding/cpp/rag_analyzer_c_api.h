// C API wrapper for RAGAnalyzer
// This file provides C-compatible interface for CGO to call

#ifndef RAG_ANALYZER_C_API_H
#define RAG_ANALYZER_C_API_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>
#include <stdbool.h>

// Opaque pointer to RAGAnalyzer
typedef void* RAGAnalyzerHandle;

// Callback function type for receiving tokens
typedef void (*RAGTokenCallback)(
    const char* text,
    uint32_t len,
    uint32_t offset,
    uint32_t end_offset
);

// Create a new RAGAnalyzer instance
// path: path to dictionary files
// Returns: handle to the analyzer, or NULL on failure
RAGAnalyzerHandle RAGAnalyzer_Create(const char* path);

// Destroy a RAGAnalyzer instance
void RAGAnalyzer_Destroy(RAGAnalyzerHandle handle);

// Load the analyzer (must be called before Analyze)
// Returns: 0 on success, negative value on failure
int RAGAnalyzer_Load(RAGAnalyzerHandle handle);

// Set fine-grained mode
void RAGAnalyzer_SetFineGrained(RAGAnalyzerHandle handle, bool fine_grained);

// Set enable position tracking
void RAGAnalyzer_SetEnablePosition(RAGAnalyzerHandle handle, bool enable_position);

// Set the language for stemming (e.g. "English", "Dutch", "German")
void RAGAnalyzer_SetLanguage(RAGAnalyzerHandle handle, const char* language);

// Analyze text and call callback for each token
// Returns: 0 on success, negative value on failure
int RAGAnalyzer_Analyze(
    RAGAnalyzerHandle handle,
    const char* text,
    RAGTokenCallback callback
);

// Simple analyze that returns tokens as a single space-separated string
// Caller is responsible for freeing the returned string
// Returns: dynamically allocated string (must call free()), or NULL on failure
char* RAGAnalyzer_Tokenize(RAGAnalyzerHandle handle, const char* text);

// Structure for a token with position information
typedef struct {
    char* text;           // Token text (must be freed with free())
    uint32_t offset;      // Byte offset of the token in the original text
    uint32_t end_offset;  // Byte end offset of the token
} RAGTokenWithPosition;

// Helper functions to access token fields (for CGO)
const char* RAGToken_GetText(void* token);
uint32_t RAGToken_GetOffset(void* token);
uint32_t RAGToken_GetEndOffset(void* token);

// Structure for a list of tokens with positions
typedef struct {
    RAGTokenWithPosition* tokens;  // Array of tokens (must be freed with RAGAnalyzer_FreeTokenList)
    uint32_t count;                // Number of tokens in the list
} RAGTokenList;

// Tokenize with position information
// Caller is responsible for freeing the returned token list with RAGAnalyzer_FreeTokenList
// Returns: dynamically allocated token list (must call RAGAnalyzer_FreeTokenList), or NULL on failure
RAGTokenList* RAGAnalyzer_TokenizeWithPosition(RAGAnalyzerHandle handle, const char* text);

// Free a token list allocated by RAGAnalyzer_TokenizeWithPosition
void RAGAnalyzer_FreeTokenList(RAGTokenList* token_list);

// Fine-grained tokenize: takes space-separated tokens and returns fine-grained tokens as space-separated string
// Caller is responsible for freeing the returned string
// Returns: dynamically allocated string (must call free()), or NULL on failure
char* RAGAnalyzer_FineGrainedTokenize(RAGAnalyzerHandle handle, const char* tokens);

// Get the frequency of a term (matching Python rag_tokenizer.freq)
// Returns: frequency value, or 0 if term not found
int32_t RAGAnalyzer_GetTermFreq(RAGAnalyzerHandle handle, const char* term);

// Get the POS tag of a term (matching Python rag_tokenizer.tag)
// Caller is responsible for freeing the returned string
// Returns: dynamically allocated string (must call free()), or NULL if term not found or no tag
char* RAGAnalyzer_GetTermTag(RAGAnalyzerHandle handle, const char* term);

// Copy an existing RAGAnalyzer instance to create a new independent instance
// This is useful for creating per-request analyzer instances in multi-threaded environments
// The new instance shares the tokenizer dictionaries and owns its OpenCC converter state.
// Returns: handle to the new analyzer instance, or NULL on failure
RAGAnalyzerHandle RAGAnalyzer_Copy(RAGAnalyzerHandle handle);

// ---------------------------------------------------------------------------
// Named Entity Recognition (spaCy model inference)
// ---------------------------------------------------------------------------

// Create a ThincNER inference handle.
// model_ner_dir:  path to the spaCy model's ner/ component directory
// model_vocab_dir: path to the spaCy model's vocab/ directory (optional, can be NULL)
// Returns: handle, or NULL on failure
RAGAnalyzerHandle ThincNER_Create(const char* model_ner_dir, const char* model_vocab_dir);

// Destroy a ThincNER handle.
void ThincNER_Destroy(RAGAnalyzerHandle handle);

// Run NER on pre-tokenized text.
// tokens_json: JSON array e.g. ["Apple","Inc.","was","founded","by","Steve","Jobs","."]
// Returns JSON array of entities, caller must free with ThincNER_FreeString.
char* ThincNER_Predict(RAGAnalyzerHandle handle, const char* tokens_json);

// Tokenize text using spaCy-compatible rules.
// Returns JSON array of token strings, caller must free with ThincNER_FreeString.
char* ThincNER_Tokenize(const char* text, const char* lang);

// Free a string returned by ThincNER_Predict or ThincNER_Tokenize.
void ThincNER_FreeString(char* ptr);

// ---------------------------------------------------------------------------
// SentencePiece Unigram vocabulary (XLM-R family)
//
// The lookup structure is the Darts-clone double-array already used by
// DartsTrie, because a per-node map trie costs ~96 MB for the 250k-piece XLM-R
// vocabulary while the double-array costs single-digit MB. Viterbi runs on this
// side: every step needs an O(1) trie transition, so a cgo round trip per byte
// would dominate the counting itself.
// ---------------------------------------------------------------------------

// Opaque pointer to an SPMVocab
typedef void* SPMVocabHandle;

// Build a vocabulary from the pieces that participate in segmentation.
//   keys/key_lens/ids: num_keys entries; ids[i] indexes into scores.
//   scores: per-piece Unigram score, indexed by the piece index.
//   unk_score/unk_id: the <unk> piece's score and index (-1 when absent).
//   byte_by_value: 256 entries mapping a byte to its "<0xXX>" piece index
//                  (-1 when absent); only read when byte_fallback is true.
// Returns: handle, or NULL on failure
SPMVocabHandle SPM_Vocab_Create(
    const char* const* keys,
    const uint32_t* key_lens,
    const int32_t* ids,
    uint32_t num_keys,
    const float* scores,
    uint32_t num_scores,
    float unk_score,
    int32_t unk_id,
    const int32_t* byte_by_value,
    bool byte_fallback);

// Destroy a vocabulary created by SPM_Vocab_Create
void SPM_Vocab_Destroy(SPMVocabHandle handle);

// Run Unigram Viterbi over text.
//   out_ids/out_ids_capacity: when out_ids is non-NULL it must hold `len`
//     entries (a token never consumes fewer than one byte); the chosen piece ids
//     are written there.
// Returns: 0 on success, negative on failure; out_count receives the token count
int SPM_Vocab_Viterbi(
    SPMVocabHandle handle,
    const char* text,
    uint32_t len,
    int32_t* out_ids,
    uint32_t out_ids_capacity,
    int32_t* out_count);

// Bytes held by the double-array (diagnostics and tests)
uint64_t SPM_Vocab_ArrayBytes(SPMVocabHandle handle);

// Bytes held by the per-piece score table (diagnostics and tests)
uint64_t SPM_Vocab_ScoresBytes(SPMVocabHandle handle);

#ifdef __cplusplus
}
#endif

#endif // RAG_ANALYZER_C_API_H
