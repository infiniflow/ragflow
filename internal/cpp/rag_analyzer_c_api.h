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
// The new instance shares the loaded dictionaries with the original but has independent internal state
// Returns: handle to the new analyzer instance, or NULL on failure
RAGAnalyzerHandle RAGAnalyzer_Copy(RAGAnalyzerHandle handle);

#ifdef __cplusplus
}
#endif

#endif // RAG_ANALYZER_C_API_H
