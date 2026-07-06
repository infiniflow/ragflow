// Debug version of C API with memory tracking
// Compile with: -DMEMORY_DEBUG to enable tracking

#include "rag_analyzer_c_api.h"
#include "rag_analyzer.h"
#include "term.h"
#include <cstring>
#include <string>
#include <vector>
#include <cstdio>

#ifdef MEMORY_DEBUG
#include <map>
#include <mutex>

static std::mutex g_memory_mutex;
static std::map<void*, size_t> g_allocations;
static size_t g_total_allocated = 0;
static size_t g_total_freed = 0;

void* debug_malloc(size_t size, const char* file, int line) {
    void* ptr = malloc(size);
    std::lock_guard<std::mutex> lock(g_memory_mutex);
    g_allocations[ptr] = size;
    g_total_allocated += size;
    fprintf(stderr, "[MEM_DEBUG] ALLOC: %p (%zu bytes) at %s:%d\n", ptr, size, file, line);
    return ptr;
}

void debug_free(void* ptr, const char* file, int line) {
    if (!ptr) return;
    {
        std::lock_guard<std::mutex> lock(g_memory_mutex);
        auto it = g_allocations.find(ptr);
        if (it != g_allocations.end()) {
            g_total_freed += it->second;
            g_allocations.erase(it);
        }
    }
    fprintf(stderr, "[MEM_DEBUG] FREE:  %p at %s:%d\n", ptr, file, line);
    free(ptr);
}

void print_memory_stats() {
    std::lock_guard<std::mutex> lock(g_memory_mutex);
    fprintf(stderr, "\n[MEM_DEBUG] ===== Memory Statistics =====\n");
    fprintf(stderr, "[MEM_DEBUG] Total allocated: %zu bytes\n", g_total_allocated);
    fprintf(stderr, "[MEM_DEBUG] Total freed:     %zu bytes\n", g_total_freed);
    fprintf(stderr, "[MEM_DEBUG] Current usage:   %zu bytes\n", g_total_allocated - g_total_freed);
    fprintf(stderr, "[MEM_DEBUG] Active allocations: %zu\n", g_allocations.size());
    if (!g_allocations.empty()) {
        fprintf(stderr, "[MEM_DEBUG] Active blocks:\n");
        for (const auto& [ptr, size] : g_allocations) {
            fprintf(stderr, "[MEM_DEBUG]   %p: %zu bytes\n", ptr, size);
        }
    }
    fprintf(stderr, "[MEM_DEBUG] ============================\n\n");
}

#define DEBUG_MALLOC(size) debug_malloc(size, __FILE__, __LINE__)
#define DEBUG_FREE(ptr) debug_free(ptr, __FILE__, __LINE__)

#else

#define DEBUG_MALLOC(size) malloc(size)
#define DEBUG_FREE(ptr) free(ptr)
void print_memory_stats() {}

#endif

extern "C" {

RAGAnalyzerHandle RAGAnalyzer_Create(const char* path) {
    if (!path) return nullptr;
    try {
        RAGAnalyzer* analyzer = new RAGAnalyzer(std::string(path));
        fprintf(stderr, "[C_API] Created analyzer: %p\n", (void*)analyzer);
        return static_cast<RAGAnalyzerHandle>(analyzer);
    } catch (...) {
        fprintf(stderr, "[C_API] Failed to create analyzer\n");
        return nullptr;
    }
}

void RAGAnalyzer_Destroy(RAGAnalyzerHandle handle) {
    if (handle) {
        fprintf(stderr, "[C_API] Destroying analyzer: %p\n", handle);
        RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
        delete analyzer;
    }
}

int RAGAnalyzer_Load(RAGAnalyzerHandle handle) {
    if (!handle) return -1;
    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    int ret = analyzer->Load();
    fprintf(stderr, "[C_API] Load result: %d\n", ret);
    return ret;
}

void RAGAnalyzer_SetFineGrained(RAGAnalyzerHandle handle, bool fine_grained) {
    if (!handle) return;
    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    analyzer->SetFineGrained(fine_grained);
    fprintf(stderr, "[C_API] SetFineGrained: %d\n", fine_grained);
}

void RAGAnalyzer_SetEnablePosition(RAGAnalyzerHandle handle, bool enable_position) {
    if (!handle) return;
    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    analyzer->SetEnablePosition(enable_position);
    fprintf(stderr, "[C_API] SetEnablePosition: %d\n", enable_position);
}

int RAGAnalyzer_Analyze(RAGAnalyzerHandle handle, const char* text, RAGTokenCallback callback) {
    if (!handle || !text || !callback) return -1;
    
    fprintf(stderr, "[C_API] Analyze called with text length: %zu\n", strlen(text));
    
    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    
    Term input;
    input.text_ = std::string(text);
    
    TermList output;
    int ret = analyzer->Analyze(input, output);
    
    fprintf(stderr, "[C_API] Analyze returned: %d, tokens: %zu\n", ret, output.size());
    
    if (ret != 0) {
        return ret;
    }
    
    // Call callback for each token
    for (const auto& term : output) {
        callback(term.text_.c_str(), term.text_.length(), term.word_offset_, term.end_offset_);
    }
    
    return 0;
}

char* RAGAnalyzer_Tokenize(RAGAnalyzerHandle handle, const char* text) {
    if (!handle || !text) {
        fprintf(stderr, "[C_API] Tokenize called with null handle or text\n");
        return nullptr;
    }
    
    fprintf(stderr, "[C_API] Tokenize called with text length: %zu\n", strlen(text));
    
    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    
    std::string result = analyzer->Tokenize(std::string(text));
    
    // Allocate memory for C string
    char* c_result = static_cast<char*>(DEBUG_MALLOC(result.size() + 1));
    if (c_result) {
        std::memcpy(c_result, result.c_str(), result.size() + 1);
        fprintf(stderr, "[C_API] Tokenize allocated result: %p\n", (void*)c_result);
    }
    return c_result;
}

// Debug function to print memory stats
void RAGAnalyzer_PrintMemoryStats() {
    print_memory_stats();
}

} // extern "C"
