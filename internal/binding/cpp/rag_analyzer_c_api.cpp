// C API implementation for RAGAnalyzer

#include "rag_analyzer_c_api.h"
#include "rag_analyzer.h"
#include "term.h"
#include <cstring>
#include <string>
#include <vector>

extern "C" {

RAGAnalyzerHandle RAGAnalyzer_Create(const char* path) {
    if (!path) return nullptr;
    try {
        RAGAnalyzer* analyzer = new RAGAnalyzer(std::string(path));
        return static_cast<RAGAnalyzerHandle>(analyzer);
    } catch (...) {
        return nullptr;
    }
}

void RAGAnalyzer_Destroy(RAGAnalyzerHandle handle) {
    if (handle) {
        RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
        delete analyzer;
    }
}

int RAGAnalyzer_Load(RAGAnalyzerHandle handle) {
    if (!handle) return -1;
    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    return analyzer->Load();
}

void RAGAnalyzer_SetFineGrained(RAGAnalyzerHandle handle, bool fine_grained) {
    if (!handle) return;
    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    analyzer->SetFineGrained(fine_grained);
}

void RAGAnalyzer_SetEnablePosition(RAGAnalyzerHandle handle, bool enable_position) {
    if (!handle) return;
    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    analyzer->SetEnablePosition(enable_position);
}

void RAGAnalyzer_SetLanguage(RAGAnalyzerHandle handle, const char* language) {
    if (!handle || !language) return;
    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    analyzer->SetLanguage(std::string(language));
}

int RAGAnalyzer_Analyze(RAGAnalyzerHandle handle, const char* text, RAGTokenCallback callback) {
    if (!handle || !text || !callback) return -1;

    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);

    Term input;
    input.text_ = std::string(text);

    TermList output;
    // Use the analyzer's internal state for fine_grained and enable_position
    int ret = analyzer->Analyze(input, output, analyzer->fine_grained_, analyzer->enable_position_);

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
    if (!handle || !text) return nullptr;

    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);

    std::string result = analyzer->Tokenize(std::string(text));

    // Allocate memory for C string
    char* c_result = static_cast<char*>(malloc(result.size() + 1));
    if (c_result) {
        std::memcpy(c_result, result.c_str(), result.size() + 1);
    }
    return c_result;
}

RAGTokenList* RAGAnalyzer_TokenizeWithPosition(RAGAnalyzerHandle handle, const char* text) {
    if (!handle || !text) return nullptr;

    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);

    auto [tokens, positions] = analyzer->TokenizeWithPosition(std::string(text));
    if (analyzer->fine_grained_) {
        std::string joined_tokens;
        for (size_t i = 0; i < tokens.size(); ++i) {
            if (i > 0) joined_tokens += " ";
            joined_tokens += tokens[i];
        }

        std::vector<std::string> fine_tokens;
        std::vector<std::pair<unsigned, unsigned>> fine_positions;
        analyzer->FineGrainedTokenizeWithPosition(joined_tokens, positions, fine_tokens, fine_positions);
        tokens = std::move(fine_tokens);
        positions = std::move(fine_positions);
    }

    RAGTokenList* token_list = static_cast<RAGTokenList*>(malloc(sizeof(RAGTokenList)));
    if (!token_list) {
        return nullptr;
    }

    token_list->tokens = nullptr;
    token_list->count = static_cast<uint32_t>(tokens.size());
    if (tokens.empty()) {
        return token_list;
    }

    token_list->tokens = static_cast<RAGTokenWithPosition*>(
        malloc(sizeof(RAGTokenWithPosition) * tokens.size())
    );
    if (!token_list->tokens) {
        free(token_list);
        return nullptr;
    }

    for (size_t i = 0; i < tokens.size(); ++i) {
        token_list->tokens[i].text = static_cast<char*>(malloc(tokens[i].size() + 1));
        if (token_list->tokens[i].text) {
            std::memcpy(token_list->tokens[i].text, tokens[i].c_str(), tokens[i].size() + 1);
        }
        token_list->tokens[i].offset = positions[i].first;
        token_list->tokens[i].end_offset = positions[i].second;
    }

    return token_list;
}

void RAGAnalyzer_FreeTokenList(RAGTokenList* token_list) {
    if (!token_list) return;

    if (token_list->tokens) {
        for (uint32_t i = 0; i < token_list->count; ++i) {
            if (token_list->tokens[i].text) {
                free(token_list->tokens[i].text);
            }
        }
        free(token_list->tokens);
    }
    free(token_list);
}

// Helper functions to access token fields
const char* RAGToken_GetText(void* token) {
    if (!token) return nullptr;
    RAGTokenWithPosition* t = static_cast<RAGTokenWithPosition*>(token);
    return t->text;
}

uint32_t RAGToken_GetOffset(void* token) {
    if (!token) return 0;
    RAGTokenWithPosition* t = static_cast<RAGTokenWithPosition*>(token);
    return t->offset;
}

uint32_t RAGToken_GetEndOffset(void* token) {
    if (!token) return 0;
    RAGTokenWithPosition* t = static_cast<RAGTokenWithPosition*>(token);
    return t->end_offset;
}

char* RAGAnalyzer_FineGrainedTokenize(RAGAnalyzerHandle handle, const char* tokens) {
    if (!handle || !tokens) return nullptr;

    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);

    std::vector<std::string> result;
    analyzer->FineGrainedTokenize(std::string(tokens), result);

    // Join results with space
    std::string result_str;
    for (size_t i = 0; i < result.size(); ++i) {
        if (i > 0) result_str += " ";
        result_str += result[i];
    }

    // Allocate memory for C string
    char* c_result = static_cast<char*>(malloc(result_str.size() + 1));
    if (c_result) {
        std::memcpy(c_result, result_str.c_str(), result_str.size() + 1);
    }
    return c_result;
}

int32_t RAGAnalyzer_GetTermFreq(RAGAnalyzerHandle handle, const char* term) {
    if (!handle || !term) return 0;

    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    return analyzer->Freq(term);
}

char* RAGAnalyzer_GetTermTag(RAGAnalyzerHandle handle, const char* term) {
    if (!handle || !term) return nullptr;

    RAGAnalyzer* analyzer = static_cast<RAGAnalyzer*>(handle);
    std::string tag_result = analyzer->Tag(term);

    if (tag_result.empty()) {
        return nullptr;
    }

    // Allocate memory for C string
    char* c_result = static_cast<char*>(malloc(tag_result.size() + 1));
    if (c_result) {
        std::memcpy(c_result, tag_result.c_str(), tag_result.size() + 1);
    }
    return c_result;
}

RAGAnalyzerHandle RAGAnalyzer_Copy(RAGAnalyzerHandle handle) {
    if (!handle) return nullptr;
    try {
        RAGAnalyzer* original = static_cast<RAGAnalyzer*>(handle);
        RAGAnalyzer* copy = new RAGAnalyzer(*original);
        return static_cast<RAGAnalyzerHandle>(copy);
    } catch (...) {
        return nullptr;
    }
}

} // extern "C"
