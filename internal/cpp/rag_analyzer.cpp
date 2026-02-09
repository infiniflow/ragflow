// Copyright(C) 2024 InfiniFlow, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#define PCRE2_CODE_UNIT_WIDTH 8

#include "opencc/openccxx.h"
#include "pcre2.h"

#include "string_utils.h"
#include "rag_analyzer.h"
#include "re2/re2.h"

#include <cassert>
#include <cstdint>
#include <filesystem>
#include <iostream>
#include <cmath>
#include <fstream>
// import :term;
// import :stemmer;
// import :analyzer;
// import :darts_trie;
// import :wordnet_lemmatizer;
// import :stemmer;
// import :term;
//
// import std.compat;

namespace fs = std::filesystem;

static const std::string DICT_PATH = "rag/huqie.txt";
static const std::string POS_DEF_PATH = "rag/pos-id.def";
static const std::string TRIE_PATH = "rag/huqie.trie";
static const std::string WORDNET_PATH = "wordnet";

static const std::string OPENCC_PATH = "opencc";

static const std::string REGEX_SPLIT_CHAR =
    R"#(([ ,\.<>/?;'\[\]\`!@#$%^&*$$\{\}\|_+=《》，。？、；‘’：“”【】~！￥%……（）——-]+|[a-zA-Z\.-]+|[0-9,\.-]+))#";

static const std::string NLTK_TOKENIZE_PATTERN =
    R"((?:\-{2,}|\.{2,}|(?:\.\s){2,}\.)|(?=[^\(\"\`{\[:;&\#\*@\)}\]\-,])\S+?(?=\s|$|(?:[)\";}\]\*:@\'\({\[\?!])|(?:\-{2,}|\.{2,}|(?:\.\s){2,}\.)|,(?=$|\s|(?:[)\";}\]\*:@\'\({\[\?!])|(?:\-{2,}|\.{2,}|(?:\.\s){2,}\.)))|\S)";

static constexpr std::size_t MAX_SENTENCE_LEN = 100;

static inline int32_t Encode(int32_t freq, int32_t idx) {
    uint32_t encoded_value = 0;
    if (freq < 0) {
        encoded_value |= static_cast<uint32_t>(-freq);
        encoded_value |= (1U << 23);
    } else {
        encoded_value = static_cast<uint32_t>(freq & 0x7FFFFF);
    }

    encoded_value |= static_cast<uint32_t>(idx) << 24;
    return static_cast<int32_t>(encoded_value);
}

static inline int32_t DecodeFreq(int32_t value) {
    uint32_t v1 = static_cast<uint32_t>(value) & 0xFFFFFF;
    if (v1 & (1 << 23)) {
        v1 &= 0x7FFFFF;
        return -static_cast<int32_t>(v1);
    } else {
        v1 = static_cast<int32_t>(v1);
    }
    return v1;
}

static inline int32_t DecodePOSIndex(int32_t value) {
    // POS index is stored in the high 8 bits (bits 24-31)
    return static_cast<int32_t>(static_cast<uint32_t>(value) >> 24);
}

void Split(const std::string &input, const std::string &split_pattern, std::vector<std::string> &result, bool keep_delim = false) {
    re2::RE2 pattern(split_pattern);
    re2::StringPiece leftover(input.data());
    re2::StringPiece last_end = leftover;
    re2::StringPiece extracted_delim_token;

    while (RE2::FindAndConsume(&leftover, pattern, &extracted_delim_token)) {
        std::string_view token(last_end.data(), extracted_delim_token.data() - last_end.data());
        if (!token.empty()) {
            result.emplace_back(token.data(), token.size());
        }
        if (keep_delim)
            result.emplace_back(extracted_delim_token.data(), extracted_delim_token.size());
        last_end = leftover;
    }

    if (!leftover.empty()) {
        result.emplace_back(leftover.data(), leftover.size());
    }
}

void Split(const std::string &input, const re2::RE2 &pattern, std::vector<std::string> &result, bool keep_delim = false) {
    re2::StringPiece leftover(input.data());
    re2::StringPiece last_end = leftover;
    re2::StringPiece extracted_delim_token;

    while (RE2::FindAndConsume(&leftover, pattern, &extracted_delim_token)) {
        std::string_view token(last_end.data(), extracted_delim_token.data() - last_end.data());
        if (!token.empty()) {
            result.emplace_back(token.data(), token.size());
        }
        if (keep_delim)
            result.emplace_back(extracted_delim_token.data(), extracted_delim_token.size());
        last_end = leftover;
    }

    if (!leftover.empty()) {
        result.emplace_back(leftover.data(), leftover.size());
    }
}

std::string Replace(const re2::RE2 &re, const std::string &replacement, const std::string &input) {
    std::string output = input;
    re2::RE2::GlobalReplace(&output, re, replacement);
    return output;
}

template <typename T>
std::string Join(const std::vector<T> &tokens, int start, int end, const std::string &delim = " ") {
    std::ostringstream oss;
    for (int i = start; i < end; ++i) {
        if (i > start)
            oss << delim;
        oss << tokens[i];
    }
    return std::move(oss).str();
}

template <typename T>
std::string Join(const std::vector<T> &tokens, int start, const std::string &delim = " ") {
    return Join(tokens, start, tokens.size(), delim);
}

std::string Join(const TermList &tokens, int start, int end, const std::string &delim = " ") {
    std::ostringstream oss;
    for (int i = start; i < end; ++i) {
        if (i > start)
            oss << delim;
        oss << tokens[i].text_;
    }
    return std::move(oss).str();
}

bool IsChinese(const std::string &str) {
    for (std::size_t i = 0; i < str.length(); ++i) {
        unsigned char c = str[i];
        if (c >= 0xE4 && c <= 0xE9) {
            if (i + 2 < str.length()) {
                unsigned char c2 = str[i + 1];
                unsigned char c3 = str[i + 2];
                if ((c2 >= 0x80 && c2 <= 0xBF) && (c3 >= 0x80 && c3 <= 0xBF)) {
                    return true;
                }
            }
        }
    }
    return false;
}

bool IsAlphabet(const std::string &str) {
    for (std::size_t i = 0; i < str.length(); ++i) {
        unsigned char c = str[i];
        if (c > 0x7F) {
            return false;
        }
    }
    return true;
}

bool IsKorean(const std::string &str) {
    for (std::size_t i = 0; i < str.length(); ++i) {
        unsigned char c = str[i];
        if (c == 0xE1) {
            if (i + 2 < str.length()) {
                unsigned char c2 = str[i + 1];
                unsigned char c3 = str[i + 2];
                if ((c2 == 0x84 || c2 == 0x85 || c2 == 0x86 || c2 == 0x87) && (c3 >= 0x80 && c3 <= 0xBF)) {
                    return true;
                }
            }
        }
    }
    return false;
}

bool IsJapanese(const std::string &str) {
    for (std::size_t i = 0; i < str.length(); ++i) {
        unsigned char c = str[i];
        if (c == 0xE3) {
            if (i + 2 < str.length()) {
                unsigned char c2 = str[i + 1];
                unsigned char c3 = str[i + 2];
                if ((c2 == 0x81 || c2 == 0x82 || c2 == 0x83) && (c3 >= 0x81 && c3 <= 0xBF)) {
                    return true;
                }
            }
        }
    }
    return false;
}

bool IsCJK(const std::string &str) {
    for (std::size_t i = 0; i < str.length(); ++i) {
        unsigned char c = str[i];

        // Check Chinese
        if (c >= 0xE4 && c <= 0xE9) {
            if (i + 2 < str.length()) {
                unsigned char c2 = str[i + 1];
                unsigned char c3 = str[i + 2];
                if ((c2 >= 0x80 && c2 <= 0xBF) && (c3 >= 0x80 && c3 <= 0xBF)) {
                    return true;
                }
            }
        }

        // Check Japanese
        if (c == 0xE3) {
            if (i + 2 < str.length()) {
                unsigned char c2 = str[i + 1];
                unsigned char c3 = str[i + 2];
                if ((c2 == 0x81 || c2 == 0x82 || c2 == 0x83) && (c3 >= 0x81 && c3 <= 0xBF)) {
                    return true;
                }
            }
        }

        // Check Korean
        if (c == 0xE1) {
            if (i + 2 < str.length()) {
                unsigned char c2 = str[i + 1];
                unsigned char c3 = str[i + 2];
                if ((c2 == 0x84 || c2 == 0x85 || c2 == 0x86 || c2 == 0x87) && (c3 >= 0x80 && c3 <= 0xBF)) {
                    return true;
                }
            }
        }
    }
    return false;
}

class RegexTokenizer {
public:
    RegexTokenizer() {
        int errorcode = 0;
        PCRE2_SIZE erroffset = 0;

        re_ = pcre2_compile((PCRE2_SPTR)(NLTK_TOKENIZE_PATTERN.c_str()),
                            PCRE2_ZERO_TERMINATED,
                            PCRE2_MULTILINE | PCRE2_UTF,
                            &errorcode,
                            &erroffset,
                            nullptr);
    }

    ~RegexTokenizer() {
        pcre2_code_free(re_);
    }

    void RegexTokenize(const std::string &input, TermList &tokens) {
        PCRE2_SPTR subject = (PCRE2_SPTR)input.c_str();
        PCRE2_SIZE subject_length = input.length();

        pcre2_match_data_8 *match_data = pcre2_match_data_create_8(1024, nullptr);

        PCRE2_SIZE start_offset = 0;

        while (start_offset < subject_length) {
            int res = pcre2_match(re_, subject, subject_length, start_offset, 0, match_data, nullptr);

            if (res < 0) {
                if (res == PCRE2_ERROR_NOMATCH) {
                    break; // No more matches
                } else {
                    std::cerr << "Matching error code: " << res << std::endl;
                    break; // Other error
                }
            }

            // Extract matched substring
            PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(match_data);
            for (int i = 0; i < res; ++i) {
                PCRE2_SIZE start = ovector[2 * i];
                PCRE2_SIZE end = ovector[2 * i + 1];
                tokens.Add(input.c_str() + start, end - start, start, end);
            }

            // Update the start offset for the next search
            start_offset = ovector[1]; // Move to the end of the last match
        }

        // Free memory
        pcre2_match_data_free(match_data);
    }

private:
    pcre2_code_8 *re_{nullptr};
};

class MacIntyreContractions {
public:
    // List of contractions adapted from Robert MacIntyre's tokenizer.
    std::vector<std::string> CONTRACTIONS2 = {R"((?i)\b(can)(?#X)(not)\b)",
                                              R"((?i)\b(d)(?#X)('ye)\b)",
                                              R"((?i)\b(gim)(?#X)(me)\b)",
                                              R"((?i)\b(gon)(?#X)(na)\b)",
                                              R"((?i)\b(got)(?#X)(ta)\b)",
                                              R"((?i)\b(lem)(?#X)(me)\b)",
                                              R"((?i)\b(more)(?#X)('n)\b)",
                                              R"((?i)\b(wan)(?#X)(na)(?=\s))"};
    std::vector<std::string> CONTRACTIONS3 = {R"((?i) ('t)(?#X)(is)\b)", R"((?i) ('t)(?#X)(was)\b)"};
    std::vector<std::string> CONTRACTIONS4 = {R"((?i)\b(whad)(dd)(ya)\b)", R"((?i)\b(wha)(t)(cha)\b)"};
};

// Structure to hold precompiled regex patterns
struct CompiledRegex {
    pcre2_code *re{nullptr};
    std::string substitution;

    CompiledRegex(pcre2_code *r, std::string sub) : re(r), substitution(std::move(sub)) {
    }

    CompiledRegex(const CompiledRegex &) = delete;
    CompiledRegex &operator=(const CompiledRegex &) = delete;
    CompiledRegex(CompiledRegex &&other) noexcept : re(other.re), substitution(std::move(other.substitution)) { other.re = nullptr; }

    CompiledRegex &operator=(CompiledRegex &&other) noexcept {
        if (this != &other) {
            if (re)
                pcre2_code_free(re);
            re = other.re;
            substitution = std::move(other.substitution);
            other.re = nullptr;
        }
        return *this;
    }

    ~CompiledRegex() {
        if (re) {
            pcre2_code_free(re);
        }
    }
};

class NLTKWordTokenizer {
    MacIntyreContractions contractions_;

    // Static singleton instance
    static std::unique_ptr<NLTKWordTokenizer> instance_;
    static std::once_flag init_flag_;

public:
    // Static method to get the singleton instance
    static NLTKWordTokenizer &GetInstance() {
        std::call_once(init_flag_, []() { instance_ = std::make_unique<NLTKWordTokenizer>(); });
        return *instance_;
    }

    // Starting quotes.
    std::vector<std::pair<std::string, std::string>> STARTING_QUOTES = {
        {std::string(R"(([«“‘„]|[`]+))"), std::string(R"( $1 )")},
        {std::string(R"(^\")"), std::string(R"(``)")},
        {std::string(R"((``))"), std::string(R"( $1 )")},
        {std::string(R"(([ \(\[{<])(\"|\'{2}))"), std::string(R"($1 `` )")},
        {std::string(R"((?i)(\')(?!re|ve|ll|m|t|s|d|n)(\w)\b)"), std::string(R"($1 $2)")}};

    // Ending quotes.
    std::vector<std::pair<std::string, std::string>> ENDING_QUOTES = {
        {std::string(R"(([»”’]))"), std::string(R"( $1 )")},
        {std::string(R"('')"), std::string(R"( '' )")},
        {std::string(R"(")"), std::string(R"( '' )")},
        {std::string(R"(\s+)"), std::string(R"( )")},
        {std::string(R"(([^' ])('[sS]|'[mM]|'[dD]|') )"), std::string(R"($1 $2 )")},
        {std::string(R"(([^' ])('ll|'LL|'re|'RE|'ve|'VE|n't|N'T) )"), std::string(R"($1 $2 )")}};

    // Punctuation.
    std::vector<std::pair<std::string, std::string>> PUNCTUATION = {
        {std::string(R"(([^\.])(\.)([\]\)}>"\'»”’ ]*)\s*$)"), std::string(R"($1 $2 $3 )")},
        {std::string(R"(([:,])([^\d]))"), std::string(R"( $1 $2)")},
        {std::string(R"(([:,])$)"), std::string(R"($1 )")},
        {std::string(R"(\.{2,})"), std::string(R"($0 )")},
        {std::string(R"([;@#$%&])"), std::string(R"($0 )")},
        {std::string(R"(([^\.])(\.)([\]\)}>"\']*)\s*$)"), std::string(R"($1 $2 $3 )")},
        {std::string(R"([?!])"), std::string(R"($0 )")},
        {std::string(R"(([^'])' )"), std::string(R"($1 ' )")},
        {std::string(R"([*])"), std::string(R"($0 )")}};

    // Pads parentheses
    std::pair<std::string, std::string> PARENS_BRACKETS = {std::string(R"([\]\[\(\)\{\}\<\>])"), std::string(R"( $0 )")};

    std::vector<std::pair<std::string, std::string>> CONVERT_PARENTHESES = {{std::string(R"(\()"), std::string("-LRB-")},
                                                                            {std::string(R"(\))"), std::string("-RRB-")},
                                                                            {std::string(R"(\[)"), std::string("-LSB-")},
                                                                            {std::string(R"(\])"), std::string("-RSB-")},
                                                                            {std::string(R"(\{)"), std::string("-LCB-")},
                                                                            {std::string(R"(\})"), std::string("-RCB-")}};

    std::pair<std::string, std::string> DOUBLE_DASHES = {std::string(R"(--)"), std::string(R"( -- )")};

    // Cache for compiled regex patterns
    std::vector<CompiledRegex> compiled_starting_quotes_;
    std::vector<CompiledRegex> compiled_ending_quotes_;
    std::vector<CompiledRegex> compiled_punctuation_;
    CompiledRegex compiled_parens_brackets_;
    std::vector<CompiledRegex> compiled_convert_parentheses_;
    CompiledRegex compiled_double_dashes_;
    std::vector<CompiledRegex> compiled_contractions2_;
    std::vector<CompiledRegex> compiled_contractions3_;

    // Constructor that precompiles all regex patterns
    NLTKWordTokenizer() : compiled_parens_brackets_(nullptr, ""), compiled_double_dashes_(nullptr, "") { CompileRegexPatterns(); }

    void Tokenize(const std::string &text, std::vector<std::string> &tokens, bool convert_parentheses = false) {
        std::string result = text;

        for (const auto &compiled : compiled_starting_quotes_) {
            result = ApplyRegex(result, compiled);
        }
        for (const auto &compiled : compiled_punctuation_) {
            result = ApplyRegex(result, compiled);
        }

        // Handles parentheses.
        result = ApplyRegex(result, compiled_parens_brackets_);

        // Optionally convert parentheses
        if (convert_parentheses) {
            for (const auto &compiled : compiled_convert_parentheses_) {
                result = ApplyRegex(result, compiled);
            }
        }

        // Handles double dash.
        result = ApplyRegex(result, compiled_double_dashes_);

        // Add extra space to make things easier
        result = " " + result + " ";

        for (const auto &compiled : compiled_ending_quotes_) {
            result = ApplyRegex(result, compiled);
        }

        for (const auto &compiled : compiled_contractions2_) {
            result = ApplyRegex(result, compiled);
        }

        for (const auto &compiled : compiled_contractions3_) {
            result = ApplyRegex(result, compiled);
        }

        // Split the result into tokens
        size_t start = 0;
        size_t end = result.find(' ');
        while (end != std::string::npos) {
            if (end != start) {
                std::string token = result.substr(start, end - start);
                // Handle underscore tokens properly
                if (token == "_") {
                    // Single underscore token
                    tokens.push_back("_");
                } else if (token.find('_') != std::string::npos) {
                    // Split tokens containing underscores and keep underscores as separate tokens
                    std::stringstream ss(token);
                    std::string sub_token;
                    bool first = true;
                    while (std::getline(ss, sub_token, '_')) {
                        if (!first) {
                            tokens.push_back("_");
                        }
                        if (!sub_token.empty()) {
                            tokens.push_back(sub_token);
                        }
                        first = false;
                    }
                    // Handle case where token ends with underscore
                    if (token.back() == '_') {
                        tokens.push_back("_");
                    }
                } else {
                    tokens.push_back(token);
                }
            }
            start = end + 1;
            end = result.find(' ', start);
        }
        if (start != result.length()) {
            std::string token = result.substr(start);
            // Handle underscore tokens properly
            if (token == "_") {
                // Single underscore token
                tokens.push_back("_");
            } else if (token.find('_') != std::string::npos) {
                // Split tokens containing underscores and keep underscores as separate tokens
                std::stringstream ss(token);
                std::string sub_token;
                bool first = true;
                while (std::getline(ss, sub_token, '_')) {
                    if (!first) {
                        tokens.push_back("_");
                    }
                    if (!sub_token.empty()) {
                        tokens.push_back(sub_token);
                    }
                    first = false;
                }
                // Handle case where token ends with underscore
                if (token.back() == '_') {
                    tokens.push_back("_");
                }
            } else {
                tokens.push_back(token);
            }
        }
    }

private:
    void CompileRegexPatterns() {
        compiled_starting_quotes_.reserve(STARTING_QUOTES.size());
        for (const auto &[pattern, substitution] : STARTING_QUOTES) {
            compiled_starting_quotes_.emplace_back(CompilePattern(pattern), substitution);
        }

        compiled_ending_quotes_.reserve(ENDING_QUOTES.size());
        for (const auto &[pattern, substitution] : ENDING_QUOTES) {
            compiled_ending_quotes_.emplace_back(CompilePattern(pattern), substitution);
        }

        compiled_punctuation_.reserve(PUNCTUATION.size());
        for (const auto &[pattern, substitution] : PUNCTUATION) {
            compiled_punctuation_.emplace_back(CompilePattern(pattern), substitution);
        }

        compiled_parens_brackets_ = CompiledRegex(CompilePattern(PARENS_BRACKETS.first), PARENS_BRACKETS.second);

        compiled_convert_parentheses_.reserve(CONVERT_PARENTHESES.size());
        for (const auto &[pattern, substitution] : CONVERT_PARENTHESES) {
            compiled_convert_parentheses_.emplace_back(CompilePattern(pattern), substitution);
        }

        compiled_double_dashes_ = CompiledRegex(CompilePattern(DOUBLE_DASHES.first), DOUBLE_DASHES.second);

        compiled_contractions2_.reserve(contractions_.CONTRACTIONS2.size());
        for (const auto &pattern : contractions_.CONTRACTIONS2) {
            compiled_contractions2_.emplace_back(CompilePattern(pattern), R"( $1 $2 )");
        }

        compiled_contractions3_.reserve(contractions_.CONTRACTIONS3.size());
        for (const auto &pattern : contractions_.CONTRACTIONS3) {
            compiled_contractions3_.emplace_back(CompilePattern(pattern), R"( $1 $2 )");
        }
    }

    pcre2_code *CompilePattern(const std::string &pattern) {
        int errorcode = 0;
        PCRE2_SIZE erroffset = 0;
        pcre2_code *re = pcre2_compile(reinterpret_cast<PCRE2_SPTR>(pattern.c_str()),
                                       PCRE2_ZERO_TERMINATED,
                                       PCRE2_MULTILINE | PCRE2_UTF,
                                       &errorcode,
                                       &erroffset,
                                       nullptr);

        if (re == nullptr) {
            PCRE2_UCHAR buffer[256];
            pcre2_get_error_message(errorcode, buffer, sizeof(buffer));
            std::cerr << "PCRE2 compilation failed at offset " << erroffset << ": " << buffer << std::endl;
            return nullptr;
        }
        return re;
    }

    std::string ApplyRegex(const std::string &text, const CompiledRegex &compiled) {
        if (compiled.re == nullptr) {
            return text;
        }

        PCRE2_SPTR pcre2_subject = reinterpret_cast<PCRE2_SPTR>(text.c_str());
        PCRE2_SPTR pcre2_replacement = reinterpret_cast<PCRE2_SPTR>(compiled.substitution.c_str());

        size_t outlength = text.length() * 2 < 1024 ? 1024 : text.length() * 2;
        auto buffer = std::make_unique<PCRE2_UCHAR[]>(outlength);
        int rc = pcre2_substitute(compiled.re,
                                  pcre2_subject,
                                  text.length(),
                                  0,
                                  PCRE2_SUBSTITUTE_GLOBAL,
                                  nullptr,
                                  nullptr,
                                  pcre2_replacement,
                                  PCRE2_ZERO_TERMINATED,
                                  buffer.get(),
                                  &outlength);

        if (rc < 0) {
            return text;
        }

        return std::string(reinterpret_cast<char *>(buffer.get()), outlength);
    }
};

// Static member definitions for NLTKWordTokenizer singleton
std::unique_ptr<NLTKWordTokenizer> NLTKWordTokenizer::instance_ = nullptr;
std::once_flag NLTKWordTokenizer::init_flag_;

void SentenceSplitter(const std::string &text, std::vector<std::string> &result) {
    int error_code;
    PCRE2_SIZE error_offset;
    const char *pattern = R"( *[\.\?!]['"\)\]]* *)";

    pcre2_code *re = pcre2_compile((PCRE2_SPTR)pattern, PCRE2_ZERO_TERMINATED, PCRE2_MULTILINE | PCRE2_UTF, &error_code, &error_offset, nullptr);

    if (re == nullptr) {
        PCRE2_UCHAR buffer[256];
        pcre2_get_error_message(error_code, buffer, sizeof(buffer));
        std::cerr << "PCRE2 compilation failed at offset " << error_offset << ": " << buffer << std::endl;
        return;
    }

    pcre2_match_data *match_data = pcre2_match_data_create_from_pattern(re, nullptr);

    PCRE2_SIZE start_offset = 0;
    while (start_offset < text.size()) {
        int rc = pcre2_match(re, (PCRE2_SPTR)text.c_str(), text.size(), start_offset, 0, match_data, nullptr);

        if (rc < 0) {
            result.push_back(text.substr(start_offset));
            break;
        }

        PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(match_data);
        PCRE2_SIZE match_start = ovector[0];
        PCRE2_SIZE match_end = ovector[1];

        if (match_start > start_offset) {
            result.push_back(text.substr(start_offset, match_end - start_offset));
        }

        start_offset = match_end;
    }

    pcre2_match_data_free(match_data);
    pcre2_code_free(re);
}

RAGAnalyzer::RAGAnalyzer(const std::string &path)
    : dict_path_(path), stemmer_(std::make_unique<Stemmer>()), lowercase_string_buffer_(term_string_buffer_limit_) {
    InitStemmer(STEM_LANG_ENGLISH);
}

RAGAnalyzer::RAGAnalyzer(const RAGAnalyzer &other)
    : own_dict_(false), trie_(other.trie_), pos_table_(other.pos_table_), wordnet_lemma_(other.wordnet_lemma_), stemmer_(std::make_unique<Stemmer>()),
      opencc_(other.opencc_), lowercase_string_buffer_(term_string_buffer_limit_), fine_grained_(other.fine_grained_) {
    InitStemmer(STEM_LANG_ENGLISH);
}

RAGAnalyzer::~RAGAnalyzer() {
    if (own_dict_) {
        delete trie_;
        delete pos_table_;
        delete wordnet_lemma_;
        delete opencc_;
    }
}

int32_t RAGAnalyzer::Load() {
    fs::path root(dict_path_);
    fs::path dict_path(root / DICT_PATH);

    if (!fs::exists(dict_path)) {
        printf("Invalid analyzer file: %s", dict_path.string().c_str());
        // return Status::InvalidAnalyzerFile(dict_path);
        return -1;
    }

    fs::path pos_def_path(root / POS_DEF_PATH);
    if (!fs::exists(pos_def_path)) {
        printf("Invalid post file: %s", pos_def_path.string().c_str());
        // return Status::InvalidAnalyzerFile(pos_def_path);
        return -1;
    }
    own_dict_ = true;
    trie_ = new DartsTrie();
    pos_table_ = new POSTable(pos_def_path.string());
    if (pos_table_->Load() != 0) {
        printf("Fail to load post table: %s", pos_def_path.string().c_str());
        return -1;
        // return Status::InvalidAnalyzerFile("Failed to load RAGAnalyzer POS definition");
    }

    fs::path trie_path(root / TRIE_PATH);
    if (fs::exists(trie_path)) {
        trie_->Load(trie_path.string());
    } else {
        // Build trie
        try {
            std::ifstream from(dict_path.string());
            std::string line;
            re2::RE2 re_pattern(R"([\r\n]+)");
            std::string split_pattern("([ \t])");

            while (getline(from, line)) {
                line = line.substr(0, line.find('\r'));
                if (line.empty())
                    continue;
                line = Replace(re_pattern, "", line);
                std::vector<std::string> results;
                Split(line, split_pattern, results);
                if (results.size() != 3)
                    throw std::runtime_error("Invalid dictionary format");
                int32_t freq = std::stoi(results[1]);
                freq = int32_t(std::log(float(freq) / DENOMINATOR) + 0.5);
                int32_t pos_idx = pos_table_->GetPOSIndex(results[2]);
                int value = Encode(freq, pos_idx);
                trie_->Add(results[0], value);
                std::string rkey = RKey(results[0]);
                trie_->Add(rkey, Encode(1, 0));
            }
            trie_->Build();
        } catch (const std::exception &e) {
            return -1;
            // return Status::InvalidAnalyzerFile("Failed to load RAGAnalyzer analyzer");
        }
        trie_->Save(trie_path.string());
    }

    fs::path lemma_path(root / WORDNET_PATH);
    if (!fs::exists(lemma_path)) {
        return -1;
        // return Status::InvalidAnalyzerFile(lemma_path);
    }

    wordnet_lemma_ = new WordNetLemmatizer(lemma_path.string());

    fs::path opencc_path(root / OPENCC_PATH);

    if (!fs::exists(opencc_path)) {
        return -1;
        // return Status::InvalidAnalyzerFile(opencc_path);
    }
    try {
        opencc_ = new ::OpenCC(opencc_path.string());
    } catch (const std::exception &e) {
        return -1;
        // return Status::InvalidAnalyzerFile("Failed to load OpenCC");
    }

    // return Status::OK();
    return 0;
}

void RAGAnalyzer::BuildPositionMapping(const std::string &original, const std::string &converted, std::vector<unsigned> &pos_mapping) {
    pos_mapping.clear();
    pos_mapping.resize(converted.size() + 1);

    size_t orig_pos = 0;
    size_t conv_pos = 0;

    // Map each character position from converted string to original string
    while (orig_pos < original.size() && conv_pos < converted.size()) {
        // Get character lengths
        size_t orig_char_len = UTF8_BYTE_LENGTH_TABLE[static_cast<uint8_t>(original[orig_pos])];
        size_t conv_char_len = UTF8_BYTE_LENGTH_TABLE[static_cast<uint8_t>(converted[conv_pos])];

        // Map all bytes of current converted character to current original position
        for (size_t i = 0; i < conv_char_len && conv_pos + i < pos_mapping.size(); ++i) {
            pos_mapping[conv_pos + i] = static_cast<unsigned>(orig_pos);
        }

        // Move to next character in both strings
        orig_pos += orig_char_len;
        conv_pos += conv_char_len;
    }

    // Fill any remaining positions
    for (size_t i = conv_pos; i < pos_mapping.size(); ++i) {
        pos_mapping[i] = static_cast<unsigned>(original.size());
    }
}

std::string RAGAnalyzer::StrQ2B(const std::string &input) {
    std::string output;
    size_t i = 0;

    while (i < input.size()) {
        unsigned char c = input[i];

        uint32_t codepoint = 0;
        if (c < 0x80) {
            codepoint = c;
            i += 1;
        } else if ((c & 0xE0) == 0xC0) {
            codepoint = (c & 0x1F) << 6;
            codepoint |= (input[i + 1] & 0x3F);
            i += 2;
        } else if ((c & 0xF0) == 0xE0) {
            codepoint = (c & 0x0F) << 12;
            codepoint |= (input[i + 1] & 0x3F) << 6;
            codepoint |= (input[i + 2] & 0x3F);
            i += 3;
        } else {
            output += c;
            i += 1;
            continue;
        }

        if (codepoint >= 0xFF01 && codepoint <= 0xFF5E) {
            output += static_cast<char>(codepoint - 0xFEE0);
        } else if (codepoint == 0x3000) {
            output += ' ';
        } else {
            if (codepoint < 0x80) {
                output += static_cast<char>(codepoint);
            } else if (codepoint < 0x800) {
                output += static_cast<char>(0xC0 | (codepoint >> 6));
                output += static_cast<char>(0x80 | (codepoint & 0x3F));
            } else if (codepoint < 0x10000) {
                output += static_cast<char>(0xE0 | (codepoint >> 12));
                output += static_cast<char>(0x80 | ((codepoint >> 6) & 0x3F));
                output += static_cast<char>(0x80 | (codepoint & 0x3F));
            }
        }
    }

    return output;
}

int32_t RAGAnalyzer::Freq(const std::string_view key) const {
    int32_t v = trie_->Get(key);
    v = DecodeFreq(v);
    return static_cast<int32_t>(std::exp(v) * DENOMINATOR + 0.5);
}

std::string RAGAnalyzer::Tag(std::string_view key) const {
    std::string lower_key = Key(std::string(key));
    int32_t encoded_value = trie_->Get(lower_key);
    if (encoded_value == -1) {
        return "";
    }
    int32_t pos_idx = DecodePOSIndex(encoded_value);
    if (pos_table_ == nullptr) {
        return "";
    }
    const char* pos_tag = pos_table_->GetPOS(pos_idx);
    return pos_tag ? std::string(pos_tag) : "";
}

std::string RAGAnalyzer::Key(const std::string_view line) { return ToLowerString(line); }

std::string RAGAnalyzer::RKey(const std::string_view line) {
    std::string reversed;
    reversed.reserve(line.size() + 2);
    reversed += "DD";
    for (size_t i = line.size(); i > 0;) {
        size_t start = i - 1;
        while (start > 0 && (line[start] & 0xC0) == 0x80) {
            --start;
        }
        reversed += line.substr(start, i - start);
        i = start;
    }
    ToLower(reversed.data() + 2, reversed.size() - 2);
    return reversed;
}

std::pair<std::vector<std::string>, double> RAGAnalyzer::Score(const std::vector<std::pair<std::string, int>> &token_freqs) {
    constexpr int64_t B = 30;
    int64_t F = 0, L = 0;
    std::vector<std::string> tokens;
    tokens.reserve(token_freqs.size());
    for (const auto &[token, freq_tag] : token_freqs) {
        F += DecodeFreq(freq_tag);
        L += (UTF8Length(token) < 2) ? 0 : 1;
        tokens.push_back(token);
    }
    const auto score = B / static_cast<double>(tokens.size()) + L / static_cast<double>(tokens.size()) + F;
    return {std::move(tokens), score};
}

void RAGAnalyzer::SortTokens(const std::vector<std::vector<std::pair<std::string, int>>> &token_list,
                             std::vector<std::pair<std::vector<std::string>, double>> &res) {
    for (const auto &tfts : token_list) {
        res.push_back(Score(tfts));
    }
    std::sort(res.begin(), res.end(), [](const auto &a, const auto &b) { return a.second > b.second; });
}

std::pair<std::vector<std::string>, double> RAGAnalyzer::MaxForward(const std::string &line) const {
    std::vector<std::pair<std::string, int>> res;
    std::size_t s = 0;
    std::size_t len = UTF8Length(line);

    while (s < len) {
        std::size_t e = s + 1;
        std::string t = UTF8Substr(line, s, e - s);

        while (e < len && trie_->HasKeysWithPrefix(Key(t))) {
            e += 1;
            t = UTF8Substr(line, s, e - s);
        }

        while (e - 1 > s && trie_->Get(Key(t)) == -1) {
            e -= 1;
            t = UTF8Substr(line, s, e - s);
        }

        int v = trie_->Get(Key(t));
        if (v != -1) {
            res.emplace_back(std::move(t), v);
        } else {
            res.emplace_back(std::move(t), 0);
        }

        s = e;
    }

    return Score(res);
}

std::pair<std::vector<std::string>, double> RAGAnalyzer::MaxBackward(const std::string &line) const {
    std::vector<std::pair<std::string, int>> res;
    int s = UTF8Length(line) - 1;

    while (s >= 0) {
        const int e = s + 1;
        std::string t = UTF8Substr(line, s, e - s);
        while (s > 0 && trie_->HasKeysWithPrefix(RKey(t))) {
            s -= 1;
            t = UTF8Substr(line, s, e - s);
        }
        while (s + 1 < e && trie_->Get(Key(t)) == -1) {
            s += 1;
            t = UTF8Substr(line, s, e - s);
        }

        int v = trie_->Get(Key(t));
        if (v != -1) {
            res.emplace_back(std::move(t), v);
        } else {
            res.emplace_back(std::move(t), 0);
        }

        s -= 1;
    }

    std::reverse(res.begin(), res.end());
    return Score(res);
}

int RAGAnalyzer::DFS(const std::string &chars,
                     const int s,
                     std::vector<std::pair<std::string, int>> &pre_tokens,
                     std::vector<std::vector<std::pair<std::string, int>>> &token_list,
                     std::vector<std::string> &best_tokens,
                     double &max_score,
                     const bool memo_all) const {
    int res = s;
    const int len = UTF8Length(chars);
    if (s >= len) {
        if (memo_all) {
            token_list.push_back(pre_tokens);
        } else if (auto [vec_str, current_score] = Score(pre_tokens); current_score > max_score) {
            best_tokens = std::move(vec_str);
            max_score = current_score;
        }
        return res;
    }
    // pruning
    int S = s + 1;
    if (s + 2 <= len) {
        std::string t1 = UTF8Substr(chars, s, 1);
        std::string t2 = UTF8Substr(chars, s, 2);
        if (trie_->HasKeysWithPrefix(Key(t1)) && !trie_->HasKeysWithPrefix(Key(t2))) {
            S = s + 2;
        }
    }

    if (pre_tokens.size() > 2 && UTF8Length(pre_tokens[pre_tokens.size() - 1].first) == 1 &&
        UTF8Length(pre_tokens[pre_tokens.size() - 2].first) == 1 && UTF8Length(pre_tokens[pre_tokens.size() - 3].first) == 1) {
        std::string t1 = pre_tokens[pre_tokens.size() - 1].first + UTF8Substr(chars, s, 1);
        if (trie_->HasKeysWithPrefix(Key(t1))) {
            S = s + 2;
        }
    }

    for (int e = S; e <= len; ++e) {
        std::string t = UTF8Substr(chars, s, e - s);
        std::string k = Key(t);

        if (e > s + 1 && !trie_->HasKeysWithPrefix(k)) {
            break;
        }

        if (const int v = trie_->Get(k); v != -1) {
            auto pretks = pre_tokens;
            pretks.emplace_back(std::move(t), v);
            res = std::max(res, DFS(chars, e, pretks, token_list, best_tokens, max_score, memo_all));
        }
    }

    if (res > s) {
        return res;
    }

    std::string t = UTF8Substr(chars, s, 1);
    if (const int v = trie_->Get(Key(t)); v != -1) {
        pre_tokens.emplace_back(std::move(t), v);
    } else {
        pre_tokens.emplace_back(std::move(t), Encode(-12, 0));
    }

    return DFS(chars, s + 1, pre_tokens, token_list, best_tokens, max_score, memo_all);
}

struct TokensList {
    const TokensList *prev = nullptr;
    std::string_view token = {};
};

struct BestTokenCandidate {
    static constexpr int64_t B = 30;
    TokensList tl{};
    // N: token num
    // L: num of tokens with length >= 2
    // F: sum of freq
    uint32_t N{};
    uint32_t L{};
    int64_t F{};

    auto k() const {
#ifdef DIVIDE_F_BY_N
        return N;
#else
        return std::make_pair(N, L);
#endif
    }

    auto v() const { return F; }

    auto score() const {
#ifdef DIVIDE_F_BY_N
        return static_cast<double>(B + L + F) / N;
#else
        return F + (static_cast<double>(B + L) / N);
#endif
    }

    BestTokenCandidate update(const std::string_view new_token_sv, const int32_t key_f, const uint32_t add_l) const {
        return {{&tl, new_token_sv}, N + 1, L + add_l, F + key_f};
    }
};

struct GrowingBestTokenCandidatesTopN {
    int32_t top_n{};
    std::vector<BestTokenCandidate> candidates{};

    explicit GrowingBestTokenCandidatesTopN(const int32_t top_n) : top_n(top_n) {
    }

    void AddBestTokenCandidateTopN(const BestTokenCandidate &add_candidate) {
        const auto [it_b, it_e] =
            std::equal_range(candidates.begin(), candidates.end(), add_candidate, [](const auto &a, const auto &b) { return a.k() < b.k(); });
        auto target_it = it_b;
        bool do_replace = false;
        if (const auto match_cnt = std::distance(it_b, it_e); match_cnt >= top_n) {
            assert(match_cnt == top_n);
            const auto it = std::min_element(it_b, it_e, [](const auto &a, const auto &b) { return a.v() < b.v(); });
            if (it->v() >= add_candidate.v()) {
                return;
            }
            target_it = it;
            do_replace = true;
        }
        if (do_replace) {
            *target_it = add_candidate;
        } else {
            candidates.insert(target_it, add_candidate);
        }
    }
};

std::vector<std::pair<std::vector<std::string_view>, double>> RAGAnalyzer::GetBestTokensTopN(const std::string_view chars, const uint32_t n) const {
    const auto utf8_len = UTF8Length(chars);
    std::vector<GrowingBestTokenCandidatesTopN> dp_vec(utf8_len + 1, GrowingBestTokenCandidatesTopN(n));
    dp_vec[0].candidates.resize(1);
    const char *current_utf8_ptr = chars.data();
    uint32_t current_left_chars = chars.size();
    std::string growing_key; // in lower case
    for (uint32_t i = 0; i < utf8_len; ++i) {
        const std::string_view current_chars{current_utf8_ptr, current_left_chars};
        const uint32_t left_utf8_cnt = utf8_len - i;
        growing_key.clear();
        const char *lookup_until = current_utf8_ptr;
        uint32_t lookup_left_chars = current_left_chars;
        std::size_t reuse_node_pos = 0;
        std::size_t reuse_key_pos = 0;
        for (uint32_t j = 1; j <= left_utf8_cnt; ++j) {
            {
                // handle growing_key
                const auto next_one_utf8 = UTF8Substrview({lookup_until, lookup_left_chars}, 0, 1);
                if (next_one_utf8.size() == 1 && next_one_utf8[0] >= 'A' && next_one_utf8[0] <= 'Z') {
                    growing_key.push_back(next_one_utf8[0] - 'A' + 'a');
                } else {
                    growing_key.append(next_one_utf8);
                }
                lookup_until += next_one_utf8.size();
                lookup_left_chars -= next_one_utf8.size();
            }
            auto dp_f = [&dp_vec, i, j, original_sv = std::string_view{current_utf8_ptr, growing_key.size()}](
                const int32_t key_f,
                const uint32_t add_l) {
                auto &target_dp = dp_vec[i + j];
                for (const auto &c : dp_vec[i].candidates) {
                    target_dp.AddBestTokenCandidateTopN(c.update(original_sv, key_f, add_l));
                }
            };
            if (const auto traverse_result = trie_->Traverse(growing_key.data(), reuse_node_pos, reuse_key_pos, growing_key.size());
                traverse_result >= 0) {
                // in dictionary
                const int32_t key_f = DecodeFreq(traverse_result);
                const auto add_l = static_cast<uint32_t>(j >= 2);
                dp_f(key_f, add_l);
            } else {
                // not in dictionary
                if (j == 1) {
                    // also give a score: -12
                    dp_f(-12, 0);
                }
                if (traverse_result == -2) {
                    // no more results
                    break;
                }
            }
        }
        // update current_utf8_ptr and current_left_chars
        const auto forward_cnt = UTF8Substrview(current_chars, 0, 1).size();
        current_utf8_ptr += forward_cnt;
        current_left_chars -= forward_cnt;
    }
    std::vector<std::pair<const TokensList *, double>> mid_result;
    mid_result.reserve(n);
    for (const auto &c : dp_vec.back().candidates) {
        const auto new_pair = std::make_pair(&(c.tl), c.score());
        if (mid_result.size() < n) {
            mid_result.push_back(new_pair);
        } else {
            assert(mid_result.size() == n);
            if (new_pair.second > mid_result.back().second) {
                mid_result.pop_back();
                const auto insert_pos = std::lower_bound(mid_result.begin(),
                                                         mid_result.end(),
                                                         new_pair,
                                                         [](const auto &a, const auto &b) {
                                                             return a.second > b.second;
                                                         });
                mid_result.insert(insert_pos, new_pair);
            }
        }
    }
    class HelperFunc {
        uint32_t cnt = 0;
        std::vector<std::string_view> result{};

        void GetTokensInner(const TokensList *tl) {
            if (!tl->prev) {
                result.reserve(cnt);
                return;
            }
            ++cnt;
            GetTokensInner(tl->prev);
            result.push_back(tl->token);
        }

    public:
        std::vector<std::string_view> GetTokens(const TokensList *tl) {
            GetTokensInner(tl);
            return std::move(result);
        }
    };
    std::vector<std::pair<std::vector<std::string_view>, double>> result;
    result.reserve(mid_result.size());
    for (const auto [tl, score] : mid_result) {
        result.emplace_back(HelperFunc{}.GetTokens(tl), score);
    }
    return result;
}

// TODO: for test
// #ifndef INFINITY_DEBUG
// #define INFINITY_DEBUG 1
// #endif

#ifdef INFINITY_DEBUG
namespace dp_debug {
template <typename T>
std::string TestPrintTokens(const std::vector<T> &tokens) {
    std::ostringstream oss;
    for (std::size_t i = 0; i < tokens.size(); ++i) {
        oss << (i ? " #" : "#") << tokens[i] << "#";
    }
    return std::move(oss).str();
}

auto print_1 = [](const bool b) { return b ? "✅" : "❌"; };
auto print_2 = [](const bool b) { return b ? "equal" : "not equal"; };

void compare_score_and_tokens(const std::vector<std::string> &dfs_tokens,
                              const double dfs_score,
                              const std::vector<std::string_view> &dp_tokens,
                              const double dp_score,
                              const std::string &prefix) {
    std::ostringstream oss;
    const auto b_score_eq = dp_score == dfs_score;
    oss << fmt::format("\n{} {} DFS and DP score {}:\nDFS: {}\nDP : {}\n", print_1(b_score_eq), prefix, print_2(b_score_eq), dfs_score, dp_score);
    bool vec_equal = true;
    if (dp_tokens.size() != dfs_tokens.size()) {
        vec_equal = false;
    } else {
        for (std::size_t k = 0; k < dp_tokens.size(); ++k) {
            if (dp_tokens[k] != dfs_tokens[k]) {
                vec_equal = false;
                break;
            }
        }
    }
    oss << fmt::format("{} {} DFS and DP result {}:\nDFS: {}\nDP : {}\n",
                       print_1(vec_equal),
                       prefix,
                       print_2(vec_equal),
                       TestPrintTokens(dfs_tokens),
                       TestPrintTokens(dp_tokens));
    std::cerr << std::move(oss).str() << std::endl;
}

inline void CheckDP(const RAGAnalyzer *this_ptr,
                    const std::string_view input_str,
                    const std::vector<std::string> &dfs_tokens,
                    const double dfs_score,
                    const auto t0,
                    const auto t1) {
    const auto dp_result = this_ptr->GetBestTokensTopN(input_str, 1);
    const auto t2 = std::chrono::high_resolution_clock::now();
    const auto dfs_duration = std::chrono::duration_cast<std::chrono::duration<float, std::milli>>(t1 - t0);
    const auto dp_duration = std::chrono::duration_cast<std::chrono::duration<float, std::milli>>(t2 - t1);
    const auto dp_faster = dp_duration < dfs_duration;
    std::cerr << "\n!!! " << print_1(dp_faster) << "\nTOP1 DFS duration: " << dfs_duration << " \nDP  duration: " << dp_duration;
    const auto &[dp_vec, dp_score] = dp_result[0];
    compare_score_and_tokens(dfs_tokens, dfs_score, dp_vec, dp_score, "[1 in top1]");
}

inline void CheckDP2(const RAGAnalyzer *this_ptr, const std::string_view input_str, auto get_dfs_sorted_tokens, const auto t0, const auto t1) {
    constexpr int topn = 2;
    const auto dp_result = this_ptr->GetBestTokensTopN(input_str, topn);
    const auto t2 = std::chrono::high_resolution_clock::now();
    const auto dfs_duration = std::chrono::duration_cast<std::chrono::duration<float, std::milli>>(t1 - t0);
    const auto dp_duration = std::chrono::duration_cast<std::chrono::duration<float, std::milli>>(t2 - t1);
    const auto dp_faster = dp_duration < dfs_duration;
    std::cerr << "\n!!! " << print_1(dp_faster) << "\nTOP2 DFS duration: " << dfs_duration << " \nTOP2 DP  duration: " << dp_duration;
    const auto dfs_sorted_tokens = get_dfs_sorted_tokens();
    for (int i = 0; i < std::min(topn, (int)dfs_sorted_tokens.size()); ++i) {
        compare_score_and_tokens(dfs_sorted_tokens[i].first,
                                 dfs_sorted_tokens[i].second,
                                 dp_result[i].first,
                                 dp_result[i].second,
                                 std::format("[{} in top{}]", i + 1, topn));
    }
}
} // namespace dp_debug
#endif

std::string RAGAnalyzer::Merge(const std::string &tks_str) const {
    std::string tks = tks_str;

    tks = Replace(replace_space_pattern_, " ", tks);

    std::vector<std::string> tokens;
    Split(tks, blank_pattern_, tokens);
    std::vector<std::string> res;
    std::size_t s = 0;
    while (true) {
        if (s >= tokens.size())
            break;

        std::size_t E = s + 1;
        for (std::size_t e = s + 2; e < std::min(tokens.size() + 1, s + 6); ++e) {
            std::string tk = Join(tokens, s, e, "");
            if (re2::RE2::PartialMatch(tk, regex_split_pattern_)) {
                if (Freq(tk) > 0) {
                    E = e;
                }
            }
        }
        res.push_back(Join(tokens, s, E, ""));
        s = E;
    }

    return Join(res, 0, res.size());
}

void RAGAnalyzer::MergeWithPosition(const std::vector<std::string> &tokens,
                                    const std::vector<std::pair<unsigned, unsigned>> &positions,
                                    std::vector<std::string> &merged_tokens,
                                    std::vector<std::pair<unsigned, unsigned>> &merged_positions) {
    // Filter out empty tokens first (like spaces) to match Merge behavior
    std::vector<std::string> filtered_tokens;
    std::vector<std::pair<unsigned, unsigned>> filtered_positions;

    for (size_t i = 0; i < tokens.size(); ++i) {
        if (!tokens[i].empty() && tokens[i] != " ") {
            filtered_tokens.push_back(tokens[i]);
            filtered_positions.push_back(positions[i]);
        }
    }

    std::vector<std::string> res;
    std::size_t s = 0;
    std::vector<std::pair<unsigned, unsigned>> res_positions;

    while (true) {
        if (s >= filtered_tokens.size())
            break;

        std::size_t E = s + 1;
        for (std::size_t e = s + 2; e < std::min(filtered_tokens.size() + 1, s + 6); ++e) {
            std::string tk = Join(filtered_tokens, s, e, "");
            if (re2::RE2::PartialMatch(tk, regex_split_pattern_)) {
                if (Freq(tk) > 0) {
                    E = e;
                }
            }
        }

        std::string merged_token = Join(filtered_tokens, s, E, "");
        res.push_back(merged_token);

        unsigned start_pos = filtered_positions[s].first;
        unsigned end_pos = filtered_positions[E - 1].second;
        res_positions.emplace_back(start_pos, end_pos);

        s = E;
    }

    merged_tokens = std::move(res);
    merged_positions = std::move(res_positions);
}

void RAGAnalyzer::EnglishNormalize(const std::vector<std::string> &tokens, std::vector<std::string> &res) {
    for (auto &t : tokens) {
        if (re2::RE2::PartialMatch(t, pattern1_)) {
            //"[a-zA-Z_-]+$"
            std::string lemma_term = wordnet_lemma_->Lemmatize(t);
            char *lowercase_term = lowercase_string_buffer_.data();
            ToLower(lemma_term.c_str(), lemma_term.size(), lowercase_term, term_string_buffer_limit_);
            std::string stem_term;
            stemmer_->Stem(lowercase_term, stem_term);
            res.push_back(stem_term);
        } else {
            res.push_back(t);
        }
    }
}

void RAGAnalyzer::SplitByLang(const std::string &line, std::vector<std::pair<std::string, bool>> &txt_lang_pairs) const {
    std::vector<std::string> arr;
    Split(line, regex_split_pattern_, arr, true);

    for (const auto &a : arr) {
        if (a.empty()) {
            continue;
        }

        std::size_t s = 0;
        std::size_t e = s + 1;
        bool zh = IsChinese(UTF8Substr(a, s, 1));

        while (e < UTF8Length(a)) {
            bool _zh = IsChinese(UTF8Substr(a, e, 1));
            if (_zh == zh) {
                e++;
                continue;
            }

            std::string segment = UTF8Substr(a, s, e - s);
            txt_lang_pairs.emplace_back(segment, zh);

            s = e;
            e = s + 1;
            zh = _zh;
        }

        if (s >= UTF8Length(a)) {
            continue;
        }

        std::string segment = UTF8Substr(a, s, e - s);
        txt_lang_pairs.emplace_back(segment, zh);
    }
}

void RAGAnalyzer::TokenizeInner(std::vector<std::string> &res, const std::string &L) const {
    auto [tks, s] = MaxForward(L);
    auto [tks1, s1] = MaxBackward(L);

#if 0
    std::size_t i = 0, j = 0, _i = 0, _j = 0, same = 0;
    while ((i + same < tks1.size()) && (j + same < tks.size()) && tks1[i + same] == tks[j + same]) {
        same++;
    }
    if (same > 0) {
        res.push_back(Join(tks, j, j + same));
    }
    _i = i + same;
    _j = j + same;
    j = _j + 1;
    i = _i + 1;
    while (i < tks1.size() && j < tks.size()) {
        std::string tk1 = Join(tks1, _i, i, "");
        std::string tk = Join(tks, _j, j, "");
        if (tk1 != tk) {
            if (tk1.length() > tk.length()) {
                j++;
            } else {
                i++;
            }
            continue;
        }
        if (tks1[i] != tks[j]) {
            i++;
            j++;
            continue;
        }
        std::vector<std::pair<std::string, int>> pre_tokens;
        std::vector<std::vector<std::pair<std::string, int>>> token_list;
        std::vector<std::string> best_tokens;
        double max_score = std::numeric_limits<double>::lowest();
        const auto str_for_dfs = Join(tks, _j, j, "");
#ifdef INFINITY_DEBUG
    const auto t0 = std::chrono::high_resolution_clock::now();
#endif
    DFS(str_for_dfs, 0, pre_tokens, token_list, best_tokens, max_score, false);
#ifdef INFINITY_DEBUG
    const auto t1 = std::chrono::high_resolution_clock::now();
    dp_debug::CheckDP(this, str_for_dfs, best_tokens, max_score, t0, t1);
#endif
    res.push_back(Join(best_tokens, 0));

    same = 1;
    while (i + same < tks1.size() && j + same < tks.size() && tks1[i + same] == tks[j + same])
        same++;
    res.push_back(Join(tks, j, j + same));
    _i = i + same;
    _j = j + same;
    j = _j + 1;
    i = _i + 1;
    }
    if (_i < tks1.size()) {
        std::vector<std::pair<std::string, int>> pre_tokens;
        std::vector<std::vector<std::pair<std::string, int>>> token_list;
        std::vector<std::string> best_tokens;
        double max_score = std::numeric_limits<double>::lowest();
        const auto str_for_dfs = Join(tks, _j, tks.size(), "");
#ifdef INFINITY_DEBUG
    const auto t0 = std::chrono::high_resolution_clock::now();
#endif
    DFS(str_for_dfs, 0, pre_tokens, token_list, best_tokens, max_score, false);
#ifdef INFINITY_DEBUG
    const auto t1 = std::chrono::high_resolution_clock::now();
    dp_debug::CheckDP(this, str_for_dfs, best_tokens, max_score, t0, t1);
#endif
    res.push_back(Join(best_tokens, 0));
    }

#else
    std::size_t i = 0, j = 0, _i = 0, _j = 0, same = 0;
    while ((i + same < tks1.size()) && (j + same < tks.size()) && tks1[i + same] == tks[j + same]) {
        same++;
    }
    if (same > 0) {
        res.push_back(Join(tks, j, j + same));
    }
    _i = i + same;
    _j = j + same;
    j = _j + 1;
    i = _i + 1;
    while (i < tks1.size() && j < tks.size()) {
        std::string tk1 = Join(tks1, _i, i, "");
        std::string tk = Join(tks, _j, j, "");
        if (tk1 != tk) {
            if (tk1.length() > tk.length()) {
                j++;
            } else {
                i++;
            }
            continue;
        }
        if (tks1[i] != tks[j]) {
            i++;
            j++;
            continue;
        }

        std::vector<std::pair<std::string, int>> pre_tokens;
        std::vector<std::vector<std::pair<std::string, int>>> token_list;
        std::vector<std::string> best_tokens;
        double max_score = std::numeric_limits<double>::lowest();
        const auto str_for_dfs = Join(tks, _j, j, "");
#ifdef INFINITY_DEBUG
        const auto t0 = std::chrono::high_resolution_clock::now();
#endif
        DFS(str_for_dfs, 0, pre_tokens, token_list, best_tokens, max_score, false);
#ifdef INFINITY_DEBUG
        const auto t1 = std::chrono::high_resolution_clock::now();
        dp_debug::CheckDP(this, str_for_dfs, best_tokens, max_score, t0, t1);
#endif
        res.push_back(Join(best_tokens, 0));

        same = 1;
        while (i + same < tks1.size() && j + same < tks.size() && tks1[i + same] == tks[j + same])
            same++;
        res.push_back(Join(tks, j, j + same));
        _i = i + same;
        _j = j + same;
        j = _j + 1;
        i = _i + 1;
    }
    if (_i < tks1.size()) {
        std::vector<std::pair<std::string, int>> pre_tokens;
        std::vector<std::vector<std::pair<std::string, int>>> token_list;
        std::vector<std::string> best_tokens;
        double max_score = std::numeric_limits<double>::lowest();
        const auto str_for_dfs = Join(tks, _j, tks.size(), "");
#ifdef INFINITY_DEBUG
        const auto t0 = std::chrono::high_resolution_clock::now();
#endif
        DFS(str_for_dfs, 0, pre_tokens, token_list, best_tokens, max_score, false);
#ifdef INFINITY_DEBUG
        const auto t1 = std::chrono::high_resolution_clock::now();
        dp_debug::CheckDP(this, str_for_dfs, best_tokens, max_score, t0, t1);
#endif
        res.push_back(Join(best_tokens, 0));
    }
#endif
}

void RAGAnalyzer::SplitLongText(const std::string &L, uint32_t length, std::vector<std::string> &sublines) const {
    uint32_t slice_count = length / MAX_SENTENCE_LEN + 1;
    sublines.reserve(slice_count);
    std::size_t last_sentence_start = 0;
    std::size_t next_sentence_start = 0;
    for (unsigned i = 0; i < slice_count; ++i) {
        next_sentence_start = MAX_SENTENCE_LEN * (i + 1) - 5;
        if (next_sentence_start + 5 < length) {
            std::size_t sentence_length = MAX_SENTENCE_LEN * (i + 1) + 5 > length ? length - next_sentence_start : 10;
            std::string substr = UTF8Substr(L, next_sentence_start, sentence_length);
            auto [tks, s] = MaxForward(substr);
            auto [tks1, s1] = MaxBackward(substr);
            std::vector<int> diff(std::max(tks.size(), tks1.size()), 0);
            for (std::size_t j = 0; j < std::min(tks.size(), tks1.size()); ++j) {
                if (tks[j] != tks1[j]) {
                    diff[j] = 1;
                }
            }

            if (s1 > s) {
                tks = tks1;
            }
            std::size_t start = 0;
            std::size_t forward_same_len = 0;
            while (start < tks.size() && diff[start] == 0) {
                forward_same_len += UTF8Length(tks[start]);
                start++;
            }
            if (forward_same_len == 0) {
                std::size_t end = tks.size() - 1;
                std::size_t backward_same_len = 0;
                while (end >= 0 && diff[end] == 0) {
                    backward_same_len += UTF8Length(tks[end]);
                    end--;
                }
                next_sentence_start += sentence_length - backward_same_len;
            } else
                next_sentence_start += forward_same_len;
        } else
            next_sentence_start = length;
        if (next_sentence_start == last_sentence_start)
            continue;
        std::string str = UTF8Substr(L, last_sentence_start, next_sentence_start - last_sentence_start);
        sublines.push_back(str);
        last_sentence_start = next_sentence_start;
    }
}

// PCRE2-based replacement function to match Python's re.sub behavior
// Returns processed string and position mapping from processed to original
std::pair<std::string, std::vector<std::pair<unsigned, unsigned>>>
PCRE2GlobalReplaceWithPosition(const std::string &text, const std::string &pattern, const std::string &replacement) {

    std::vector<std::pair<unsigned, unsigned>> pos_mapping;
    std::string result;

    pcre2_code *re;
    PCRE2_SPTR pcre2_pattern = reinterpret_cast<PCRE2_SPTR>(pattern.c_str());
    PCRE2_SPTR pcre2_subject = reinterpret_cast<PCRE2_SPTR>(text.c_str());
    // Note: pcre2_replacement is used in the replacement logic below
    int errorcode;
    PCRE2_SIZE erroroffset;

    // Compile the pattern with UTF and UCP flags for Unicode support
    re = pcre2_compile(pcre2_pattern, PCRE2_ZERO_TERMINATED, PCRE2_UCP | PCRE2_UTF, &errorcode, &erroroffset, nullptr);

    if (re == nullptr) {
        PCRE2_UCHAR buffer[256];
        pcre2_get_error_message(errorcode, buffer, sizeof(buffer));
        std::cerr << "PCRE2 compilation failed at offset " << erroroffset << ": " << buffer << std::endl;
        return {text, {}};
    }

    pcre2_match_data *match_data = pcre2_match_data_create_from_pattern(re, nullptr);

    PCRE2_SIZE current_pos = 0;
    PCRE2_SIZE last_match_end = 0;

    // Process the string match by match
    while (current_pos < text.length()) {
        int rc = pcre2_match(re, pcre2_subject, text.length(), current_pos, 0, match_data, nullptr);

        if (rc < 0) {
            // No more matches, copy remaining text
            if (last_match_end < text.length()) {
                std::string remaining = text.substr(last_match_end);
                result += remaining;

                // Map each character in remaining text
                for (size_t i = 0; i < remaining.length(); ++i) {
                    pos_mapping.emplace_back(last_match_end + i, last_match_end + i);
                }
            }
            break;
        }

        PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(match_data);
        PCRE2_SIZE match_start = ovector[0];
        PCRE2_SIZE match_end = ovector[1];

        // Copy text before the match
        if (last_match_end < match_start) {
            std::string before_match = text.substr(last_match_end, match_start - last_match_end);
            result += before_match;

            // Map each character in before_match
            for (size_t i = 0; i < before_match.length(); ++i) {
                pos_mapping.emplace_back(last_match_end + i, last_match_end + i);
            }
        }

        // Add the replacement string
        result += replacement;

        // Map each character in replacement to the start of the match
        for (size_t i = 0; i < replacement.length(); ++i) {
            pos_mapping.emplace_back(match_start, match_start);
        }

        last_match_end = match_end;
        current_pos = match_end;

        // If the match was zero-length, move forward one character to avoid infinite loop
        if (match_start == match_end) {
            if (current_pos < text.length()) {
                current_pos++;
            } else {
                break;
            }
        }
    }

    pcre2_match_data_free(match_data);
    pcre2_code_free(re);

    return {result, pos_mapping};
}

// Original PCRE2GlobalReplace for backward compatibility
std::string PCRE2GlobalReplace(const std::string &text, const std::string &pattern, const std::string &replacement) {
    auto [result, _] = PCRE2GlobalReplaceWithPosition(text, pattern, replacement);
    return result;
}

std::string RAGAnalyzer::Tokenize(const std::string &line) {
    // Python-style simple tokenization: re.sub(r"\\W+", " ", line)
    std::string processed_line = PCRE2GlobalReplace(line, R"#(\W+)#", " ");
    std::string str1 = StrQ2B(processed_line);
    std::string strline;
    opencc_->convert(str1, strline);

    std::vector<std::string> res;

    // Use SplitByLang to separate by language
    std::vector<std::pair<std::string, bool>> arr;
    SplitByLang(strline, arr);

    for (const auto &[L, lang] : arr) {
        if (!lang) {
            // Non-Chinese text: use NLTK tokenizer, lemmatize and stem
            std::vector<std::string> term_list;
            std::vector<std::string> sentences;
            SentenceSplitter(L, sentences);
            for (auto &sentence : sentences) {
                NLTKWordTokenizer::GetInstance().Tokenize(sentence, term_list);
            }
            for (unsigned i = 0; i < term_list.size(); ++i) {
                std::string t = wordnet_lemma_->Lemmatize(term_list[i]);
                char *lowercase_term = lowercase_string_buffer_.data();
                ToLower(t.c_str(), t.size(), lowercase_term, term_string_buffer_limit_);
                std::string stem_term;
                stemmer_->Stem(lowercase_term, stem_term);
                res.push_back(stem_term);
            }
            continue;
        }
        auto length = UTF8Length(L);
        if (length < 2 || re2::RE2::PartialMatch(L, pattern2_) || re2::RE2::PartialMatch(L, pattern3_)) {
            //[a-z\\.-]+$  [0-9\\.-]+$
            res.push_back(L);
            continue;
        }

        // Chinese processing: use TokenizeInner
#if 0
        if (length > MAX_SENTENCE_LEN) {
            std::vector<std::string> sublines;
            SplitLongText(L, length, sublines);
            for (auto &l : sublines) {
                TokenizeInner(res, l);
            }
        } else
#endif
        TokenizeInner(res, L);
    }

    // std::vector<std::string> normalize_res;
    // EnglishNormalize(res, normalize_res);
    std::string r = Join(res, 0);
    std::string ret = Merge(r);
    return ret;
}

std::pair<std::vector<std::string>, std::vector<std::pair<unsigned, unsigned>>> RAGAnalyzer::TokenizeWithPosition(const std::string &line) {
    // Python-style simple tokenization: re.sub(r"\W+", " ", line)
    // Get processed line and position mapping from PCRE2GlobalReplace
    auto [processed_line, pcre2_pos_mapping] = PCRE2GlobalReplaceWithPosition(line, R"#(\W+)#", " ");

    std::string str1 = StrQ2B(processed_line);
    std::string strline;
    opencc_->convert(str1, strline);
    std::vector<std::string> tokens;
    std::vector<std::pair<unsigned, unsigned>> positions;

    // Build character position mapping from StrQ2B conversion
    std::vector<unsigned> strq2b_pos_mapping;
    BuildPositionMapping(processed_line, str1, strq2b_pos_mapping);

    // Build character position mapping from OpenCC conversion
    std::vector<unsigned> opencc_pos_mapping;
    BuildPositionMapping(str1, strline, opencc_pos_mapping);

    // Combine all position mappings: strline -> str1 -> processed_line -> line
    std::vector<unsigned> final_pos_mapping;
    final_pos_mapping.resize(strline.size() + 1);

    for (size_t i = 0; i < strline.size(); ++i) {
        if (i < opencc_pos_mapping.size()) {
            unsigned str1_pos = opencc_pos_mapping[i];
            if (str1_pos < strq2b_pos_mapping.size()) {
                unsigned processed_pos = strq2b_pos_mapping[str1_pos];
                if (processed_pos < pcre2_pos_mapping.size()) {
                    final_pos_mapping[i] = pcre2_pos_mapping[processed_pos].first;
                } else {
                    final_pos_mapping[i] = static_cast<unsigned>(line.size());
                }
            } else {
                final_pos_mapping[i] = static_cast<unsigned>(line.size());
            }
        } else {
            final_pos_mapping[i] = static_cast<unsigned>(line.size());
        }
    }

    // Fill the last position
    if (strline.size() < final_pos_mapping.size()) {
        final_pos_mapping[strline.size()] = static_cast<unsigned>(line.size());
    }

    // Use SplitByLang to separate by language
    std::vector<std::pair<std::string, bool>> arr;
    SplitByLang(strline, arr);
    unsigned current_pos = 0;

    for (const auto &[L, lang] : arr) {
        if (L.empty()) {
            continue;
        }

        std::size_t processed_pos = strline.find(L, current_pos);
        if (processed_pos == std::string::npos) {
            continue;
        }

        unsigned original_start = current_pos;
        current_pos = original_start + static_cast<unsigned>(L.size());

        if (!lang) {
            // Non-Chinese text: use NLTK tokenizer, lemmatize and stem
            std::vector<std::string> term_list;
            std::vector<std::string> sentences;
            SentenceSplitter(L, sentences);

            unsigned sentence_start_pos = original_start;
            for (auto &sentence : sentences) {
                std::vector<std::string> sentence_terms;
                NLTKWordTokenizer::GetInstance().Tokenize(sentence, sentence_terms);

                unsigned current_search_pos = 0;
                for (auto &term : sentence_terms) {
                    size_t pos_in_sentence = sentence.find(term, current_search_pos);
                    if (pos_in_sentence != std::string::npos) {
                        unsigned start_pos = sentence_start_pos + static_cast<unsigned>(pos_in_sentence);
                        unsigned end_pos = start_pos + static_cast<unsigned>(term.size());
                        std::string t = wordnet_lemma_->Lemmatize(term);
                        char *lowercase_term = lowercase_string_buffer_.data();
                        ToLower(t.c_str(), t.size(), lowercase_term, term_string_buffer_limit_);
                        std::string stem_term;
                        stemmer_->Stem(lowercase_term, stem_term);

                        tokens.push_back(stem_term);

                        // Map positions back to original string using final_pos_mapping
                        if (start_pos < final_pos_mapping.size()) {
                            positions.emplace_back(final_pos_mapping[start_pos], final_pos_mapping[end_pos]);
                        } else {
                            positions.emplace_back(static_cast<unsigned>(line.size()), static_cast<unsigned>(line.size()));
                        }

                        current_search_pos = pos_in_sentence + term.size();
                    }
                }
                sentence_start_pos += static_cast<unsigned>(sentence.size());
            }
            continue;
        }

        auto length = UTF8Length(L);
        if (length < 2 || re2::RE2::PartialMatch(L, pattern2_) || re2::RE2::PartialMatch(L, pattern3_)) {
            tokens.push_back(L);

            // Map positions back to original string using final_pos_mapping
            unsigned start_pos = original_start;
            unsigned end_pos = original_start + static_cast<unsigned>(L.size());
            if (start_pos < final_pos_mapping.size() && end_pos < final_pos_mapping.size()) {
                positions.emplace_back(final_pos_mapping[start_pos], final_pos_mapping[end_pos]);
            } else {
                positions.emplace_back(static_cast<unsigned>(line.size()), static_cast<unsigned>(line.size()));
            }
            continue;
        }

        // Chinese processing: use TokenizeInnerWithPosition
#if 0
        if (length > MAX_SENTENCE_LEN) {
            std::vector<std::string> sublines;
            SplitLongText(L, length, sublines);
            unsigned subline_start_pos = original_start;
            for (auto &l : sublines) {
                TokenizeInnerWithPosition(l, tokens, positions, subline_start_pos, &final_pos_mapping);
                subline_start_pos += static_cast<unsigned>(l.size());
            }
        } else
#endif
        TokenizeInnerWithPosition(L, tokens, positions, original_start, &final_pos_mapping);
    }

    // std::vector<std::string> normalize_tokens;
    // std::vector<std::pair<unsigned, unsigned>> normalize_positions;
    // EnglishNormalizeWithPosition(tokens, positions, normalize_tokens, normalize_positions);

    // Apply MergeWithPosition to match Tokenize behavior
    std::vector<std::string> merged_tokens;
    std::vector<std::pair<unsigned, unsigned>> merged_positions;
    MergeWithPosition(tokens, positions, merged_tokens, merged_positions);

    tokens = std::move(merged_tokens);
    positions = std::move(merged_positions);

    return {std::move(tokens), std::move(positions)};
}

unsigned RAGAnalyzer::MapToOriginalPosition(unsigned processed_pos, const std::vector<std::pair<unsigned, unsigned>> &mapping) {
    for (const auto &[orig, proc] : mapping) {
        if (proc == processed_pos) {
            return orig;
        }
    }
    return processed_pos;
}

static unsigned CalculateTokensLength(const std::vector<std::string> &tokens, int start, int end) {
    unsigned total_length = 0;
    for (int i = start; i < end; ++i) {
        total_length += static_cast<unsigned>(tokens[i].size());
    }
    return total_length;
}

void RAGAnalyzer::TokenizeInnerWithPosition(const std::string &L,
                                            std::vector<std::string> &tokens,
                                            std::vector<std::pair<unsigned, unsigned>> &positions,
                                            unsigned base_pos,
                                            const std::vector<unsigned> *pos_mapping) {
    auto [tks, s] = MaxForward(L);
    auto [tks1, s1] = MaxBackward(L);

    // Use the same algorithm as Python version
    std::size_t i = 0, j = 0, _i = 0, _j = 0, same = 0;
    while ((i + same < tks1.size()) && (j + same < tks.size()) && tks1[i + same] == tks[j + same]) {
        same++;
    }
    if (same > 0) {
        std::string token_str = Join(tks, j, j + same);
        unsigned token_len = static_cast<unsigned>(token_str.size());
        unsigned start_pos = base_pos + CalculateTokensLength(tks, 0, j);

        if (token_str.find(' ') != std::string::npos) {
            std::vector<std::string> space_split_tokens;
            Split(token_str, blank_pattern_, space_split_tokens, false);
            unsigned space_start_pos = start_pos;
            for (const auto &space_token : space_split_tokens) {
                if (space_token.empty()) {
                    continue;
                }
                unsigned space_token_len = static_cast<unsigned>(space_token.size());
                tokens.push_back(space_token);
                // Map position back to original string if mapping is provided
                if (pos_mapping) {
                    unsigned mapped_start = space_start_pos < pos_mapping->size() ? (*pos_mapping)[space_start_pos] : 0;
                    unsigned mapped_end =
                        (space_start_pos + space_token_len) < pos_mapping->size() ? (*pos_mapping)[space_start_pos + space_token_len] : 0;
                    positions.emplace_back(mapped_start, mapped_end);
                } else {
                    positions.emplace_back(space_start_pos, space_start_pos + space_token_len);
                }
                space_start_pos += space_token_len;
            }
        } else {
            tokens.push_back(token_str);
            // Map position back to original string if mapping is provided
            if (pos_mapping) {
                unsigned mapped_start = start_pos < pos_mapping->size() ? (*pos_mapping)[start_pos] : 0;
                unsigned mapped_end = (start_pos + token_len) < pos_mapping->size() ? (*pos_mapping)[start_pos + token_len] : 0;
                positions.emplace_back(mapped_start, mapped_end);
            } else {
                positions.emplace_back(start_pos, start_pos + token_len);
            }
        }
    }
    _i = i + same;
    _j = j + same;
    j = _j + 1;
    i = _i + 1;

    while (i < tks1.size() && j < tks.size()) {
        std::string tk1 = Join(tks1, _i, i, "");
        std::string tk = Join(tks, _j, j, "");
        if (tk1 != tk) {
            if (tk1.length() > tk.length()) {
                j++;
            } else {
                i++;
            }
            continue;
        }
        if (tks1[i] != tks[j]) {
            i++;
            j++;
            continue;
        }

        // Handle different part with DFS
        std::vector<std::pair<std::string, int>> pre_tokens;
        std::vector<std::vector<std::pair<std::string, int>>> token_list;
        std::vector<std::string> best_tokens;
        double max_score = std::numeric_limits<double>::lowest();
        const auto str_for_dfs = Join(tks, _j, j, "");
#ifdef INFINITY_DEBUG
        const auto t0 = std::chrono::high_resolution_clock::now();
#endif
        DFS(str_for_dfs, 0, pre_tokens, token_list, best_tokens, max_score, false);
#ifdef INFINITY_DEBUG
        const auto t1 = std::chrono::high_resolution_clock::now();
        dp_debug::CheckDP(this, str_for_dfs, best_tokens, max_score, t0, t1);
#endif

        std::string best_token_str = Join(best_tokens, 0);
        unsigned start_pos = base_pos + CalculateTokensLength(tks, 0, _j);
        std::string original_token_str = Join(tks, _j, j, "");
        unsigned end_pos = start_pos + static_cast<unsigned>(original_token_str.size());

        if (best_token_str.find(' ') != std::string::npos) {
            std::vector<std::string> space_split_tokens;
            Split(best_token_str, blank_pattern_, space_split_tokens, false);
            unsigned space_start_pos = start_pos;
            for (const auto &space_token : space_split_tokens) {
                if (space_token.empty()) {
                    continue;
                }
                unsigned space_token_len = static_cast<unsigned>(space_token.size());
                tokens.push_back(space_token);
                // Map position back to original string if mapping is provided
                if (pos_mapping) {
                    unsigned mapped_start = space_start_pos < pos_mapping->size() ? (*pos_mapping)[space_start_pos] : 0;
                    unsigned mapped_end =
                        (space_start_pos + space_token_len) < pos_mapping->size() ? (*pos_mapping)[space_start_pos + space_token_len] : 0;
                    positions.emplace_back(mapped_start, mapped_end);
                } else {
                    positions.emplace_back(space_start_pos, space_start_pos + space_token_len);
                }
                space_start_pos += space_token_len;
            }
        } else {
            tokens.push_back(best_token_str);
            // Map position back to original string if mapping is provided
            if (pos_mapping) {
                unsigned mapped_start = start_pos < pos_mapping->size() ? (*pos_mapping)[start_pos] : 0;
                unsigned mapped_end = end_pos < pos_mapping->size() ? (*pos_mapping)[end_pos] : 0;
                positions.emplace_back(mapped_start, mapped_end);
            } else {
                positions.emplace_back(start_pos, end_pos);
            }
        }

        same = 1;
        while (i + same < tks1.size() && j + same < tks.size() && tks1[i + same] == tks[j + same])
            same++;

        // Handle same part after different tokens
        std::string token_str = Join(tks, j, j + same);
        unsigned token_len = static_cast<unsigned>(token_str.size());
        start_pos = base_pos + CalculateTokensLength(tks, 0, j);

        if (token_str.find(' ') != std::string::npos) {
            std::vector<std::string> space_split_tokens;
            Split(token_str, blank_pattern_, space_split_tokens, false);
            unsigned space_start_pos = start_pos;
            for (const auto &space_token : space_split_tokens) {
                if (space_token.empty()) {
                    continue;
                }
                unsigned space_token_len = static_cast<unsigned>(space_token.size());
                tokens.push_back(space_token);
                // Map position back to original string if mapping is provided
                if (pos_mapping) {
                    unsigned mapped_start = space_start_pos < pos_mapping->size() ? (*pos_mapping)[space_start_pos] : 0;
                    unsigned mapped_end =
                        (space_start_pos + space_token_len) < pos_mapping->size() ? (*pos_mapping)[space_start_pos + space_token_len] : 0;
                    positions.emplace_back(mapped_start, mapped_end);
                } else {
                    positions.emplace_back(space_start_pos, space_start_pos + space_token_len);
                }
                space_start_pos += space_token_len;
            }
        } else {
            tokens.push_back(token_str);
            // Map position back to original string if mapping is provided
            if (pos_mapping) {
                unsigned mapped_start = start_pos < pos_mapping->size() ? (*pos_mapping)[start_pos] : 0;
                unsigned mapped_end = (start_pos + token_len) < pos_mapping->size() ? (*pos_mapping)[start_pos + token_len] : 0;
                positions.emplace_back(mapped_start, mapped_end);
            } else {
                positions.emplace_back(start_pos, start_pos + token_len);
            }
        }

        _i = i + same;
        _j = j + same;
        j = _j + 1;
        i = _i + 1;
    }

    // Handle remaining part
    if (_i < tks1.size()) {
        std::vector<std::pair<std::string, int>> pre_tokens;
        std::vector<std::vector<std::pair<std::string, int>>> token_list;
        std::vector<std::string> best_tokens;
        double max_score = std::numeric_limits<double>::lowest();
        const auto str_for_dfs = Join(tks, _j, tks.size(), "");
#ifdef INFINITY_DEBUG
        const auto t0 = std::chrono::high_resolution_clock::now();
#endif
        DFS(str_for_dfs, 0, pre_tokens, token_list, best_tokens, max_score, false);
#ifdef INFINITY_DEBUG
        const auto t1 = std::chrono::high_resolution_clock::now();
        dp_debug::CheckDP(this, str_for_dfs, best_tokens, max_score, t0, t1);
#endif

        std::string best_token_str = Join(best_tokens, 0);
        unsigned start_pos = base_pos + CalculateTokensLength(tks, 0, _j);
        std::string original_token_str = Join(tks, _j, tks.size(), "");
        unsigned end_pos = start_pos + static_cast<unsigned>(original_token_str.size());

        if (best_token_str.find(' ') != std::string::npos) {
            std::vector<std::string> space_split_tokens;
            Split(best_token_str, blank_pattern_, space_split_tokens, false);
            unsigned space_start_pos = start_pos;
            for (const auto &space_token : space_split_tokens) {
                if (space_token.empty()) {
                    continue;
                }
                unsigned space_token_len = static_cast<unsigned>(space_token.size());
                tokens.push_back(space_token);
                // Map position back to original string if mapping is provided
                if (pos_mapping) {
                    unsigned mapped_start = space_start_pos < pos_mapping->size() ? (*pos_mapping)[space_start_pos] : 0;
                    unsigned mapped_end =
                        (space_start_pos + space_token_len) < pos_mapping->size() ? (*pos_mapping)[space_start_pos + space_token_len] : 0;
                    positions.emplace_back(mapped_start, mapped_end);
                } else {
                    positions.emplace_back(space_start_pos, space_start_pos + space_token_len);
                }
                space_start_pos += space_token_len;
            }
        } else {
            tokens.push_back(best_token_str);
            // Map position back to original string if mapping is provided
            if (pos_mapping) {
                unsigned mapped_start = start_pos < pos_mapping->size() ? (*pos_mapping)[start_pos] : 0;
                unsigned mapped_end = end_pos < pos_mapping->size() ? (*pos_mapping)[end_pos] : 0;
                positions.emplace_back(mapped_start, mapped_end);
            } else {
                positions.emplace_back(start_pos, end_pos);
            }
        }
    }
}

void RAGAnalyzer::EnglishNormalizeWithPosition(const std::vector<std::string> &tokens,
                                               const std::vector<std::pair<unsigned, unsigned>> &positions,
                                               std::vector<std::string> &normalize_tokens,
                                               std::vector<std::pair<unsigned, unsigned>> &normalize_positions) {
    for (size_t i = 0; i < tokens.size(); ++i) {
        const auto &token = tokens[i];
        const auto &[start_pos, end_pos] = positions[i];

        if (re2::RE2::PartialMatch(token, pattern1_)) {
            //"[a-zA-Z_-]+$"
            std::string lemma_term = wordnet_lemma_->Lemmatize(token);
            char *lowercase_term = lowercase_string_buffer_.data();
            ToLower(lemma_term.c_str(), lemma_term.size(), lowercase_term, term_string_buffer_limit_);
            std::string stem_term;
            stemmer_->Stem(lowercase_term, stem_term);

            normalize_tokens.push_back(stem_term);
            normalize_positions.emplace_back(start_pos, end_pos);
        } else {
            normalize_tokens.push_back(token);
            normalize_positions.emplace_back(start_pos, end_pos);
        }
    }
}

void RAGAnalyzer::FineGrainedTokenizeWithPosition(const std::string &tokens_str,
                                                  const std::vector<std::pair<unsigned, unsigned>> &positions,
                                                  std::vector<std::string> &fine_tokens,
                                                  std::vector<std::pair<unsigned, unsigned>> &fine_positions) {
    std::vector<std::string> tks;
    Split(tokens_str, blank_pattern_, tks);

    std::size_t zh_num = 0;
    for (auto &token : tks) {
        int len = UTF8Length(token);
        for (int i = 0; i < len; ++i) {
            std::string t = UTF8Substr(token, i, 1);
            if (IsChinese(t)) {
                zh_num++;
            }
        }
    }

    if (zh_num < tks.size() * 0.2) {
        // English text processing - apply normalization
        std::vector<std::string> temp_tokens;
        for (size_t i = 0; i < tks.size(); ++i) {
            const auto &token = tks[i];
            const auto &[start_pos, end_pos] = positions[i];

            std::istringstream iss(token);
            std::string sub_token;
            unsigned sub_start = start_pos;

            while (std::getline(iss, sub_token, '/')) {
                if (!sub_token.empty()) {
                    unsigned sub_end = sub_start + sub_token.size();
                    fine_tokens.push_back(sub_token);
                    fine_positions.emplace_back(sub_start, sub_end);
                    sub_start = sub_end + 1;
                }
            }
        }

        // Apply English normalization to get lowercase and stemmed tokens
        // std::vector<std::pair<unsigned, unsigned>> temp_positions = fine_positions;
        // EnglishNormalizeWithPosition(temp_tokens, temp_positions, fine_tokens, fine_positions);
    } else {
        // Chinese or mixed text processing - match FineGrainedTokenize behavior
        for (size_t i = 0; i < tks.size(); ++i) {
            const auto &token = tks[i];
            const auto &[start_pos, end_pos] = positions[i];
            const auto token_len = UTF8Length(token);

            if (token_len < 3 || re2::RE2::PartialMatch(token, pattern4_)) {
                fine_tokens.push_back(token);
                fine_positions.emplace_back(start_pos, end_pos);
                continue;
            }

            std::vector<std::vector<std::pair<std::string, int>>> token_list;
            if (token_len > 10) {
                std::vector<std::pair<std::string, int>> tk;
                tk.emplace_back(token, Encode(-1, 0));
                token_list.push_back(tk);
            } else {
                std::vector<std::pair<std::string, int>> pre_tokens;
                std::vector<std::string> best_tokens;
                double max_score = 0.0F;
                DFS(token, 0, pre_tokens, token_list, best_tokens, max_score, true);
            }

            if (token_list.size() < 2) {
                fine_tokens.push_back(token);
                fine_positions.emplace_back(start_pos, end_pos);
                continue;
            }

            std::vector<std::pair<std::vector<std::string>, double>> sorted_tokens;
            SortTokens(token_list, sorted_tokens);
            const auto &stk = sorted_tokens[1].first;

            if (stk.size() == token_len) {
                fine_tokens.push_back(token);
                fine_positions.emplace_back(start_pos, end_pos);
            } else if (re2::RE2::PartialMatch(token, pattern5_)) {
                bool need_append_stk = true;
                for (auto &t : stk) {
                    if (UTF8Length(t) < 3) {
                        fine_tokens.push_back(token);
                        fine_positions.emplace_back(start_pos, end_pos);
                        need_append_stk = false;
                        break;
                    }
                }
                if (need_append_stk) {
                    unsigned sub_pos = start_pos;
                    for (auto &t : stk) {
                        unsigned sub_end = sub_pos + UTF8Length(t);
                        fine_tokens.push_back(t);
                        fine_positions.emplace_back(sub_pos, sub_end);
                        sub_pos = sub_end;
                    }
                }
            } else {
                unsigned sub_pos = start_pos;
                for (auto &t : stk) {
                    unsigned sub_end = sub_pos + static_cast<unsigned>(t.size());
                    fine_tokens.push_back(t);
                    fine_positions.emplace_back(sub_pos, sub_end);
                    sub_pos = sub_end;
                }
            }
        }
    }

    // Apply English normalization only if needed, similar to FineGrainedTokenize
    // For Chinese text, no additional normalization needed
    // fine_tokens already contains the correct Chinese tokens
}

void RAGAnalyzer::FineGrainedTokenize(const std::string &tokens, std::vector<std::string> &result) {
    std::vector<std::string> tks;
    Split(tokens, blank_pattern_, tks);
    std::vector<std::string> res;
    std::size_t zh_num = 0;
    for (auto &token : tks) {
        int len = UTF8Length(token);
        for (int i = 0; i < len; ++i) {
            std::string t = UTF8Substr(token, i, 1);
            if (IsChinese(t)) {
                zh_num++;
            }
        }
    }
    if (zh_num < tks.size() * 0.2) {
        for (auto &token : tks) {
            std::istringstream iss(token);
            std::string sub_token;
            while (std::getline(iss, sub_token, '/')) {
                result.push_back(sub_token);
            }
        }
        // std::string ret = Join(res, 0);
        return;
    }

    for (auto &token : tks) {
        const auto token_len = UTF8Length(token);
        if (token_len < 3 || re2::RE2::PartialMatch(token, pattern4_)) {
            //[0-9,\\.-]+$
            res.push_back(token);
            continue;
        }
        std::vector<std::vector<std::pair<std::string, int>>> token_list;
        if (token_len > 10) {
            std::vector<std::pair<std::string, int>> tk;
            tk.emplace_back(token, Encode(-1, 0));
            token_list.push_back(tk);
        } else {
            std::vector<std::pair<std::string, int>> pre_tokens;
            std::vector<std::string> best_tokens;
            double max_score = 0.0F;
#ifdef INFINITY_DEBUG
            const auto t0 = std::chrono::high_resolution_clock::now();
#endif
            DFS(token, 0, pre_tokens, token_list, best_tokens, max_score, true);
#ifdef INFINITY_DEBUG
            const auto t1 = std::chrono::high_resolution_clock::now();
            auto get_dfs_sorted_tokens = [&]() {
                std::vector<std::pair<std::vector<std::string>, double>> sorted_tokens;
                SortTokens(token_list, sorted_tokens);
                return sorted_tokens;
            };
            dp_debug::CheckDP2(this, token, get_dfs_sorted_tokens, t0, t1);
#endif
        }
        if (token_list.size() < 2) {
            res.push_back(token);
            continue;
        }
        std::vector<std::pair<std::vector<std::string>, double>> sorted_tokens;
        SortTokens(token_list, sorted_tokens);
        const auto &stk = sorted_tokens[1].first;
        if (stk.size() == token_len) {
            res.push_back(token);
        } else if (re2::RE2::PartialMatch(token, pattern5_)) {
            // [a-z\\.-]+
            bool need_append_stk = true;
            for (auto &t : stk) {
                if (UTF8Length(t) < 3) {
                    res.push_back(token);
                    need_append_stk = false;
                    break;
                }
            }
            if (need_append_stk) {
                for (auto &t : stk) {
                    res.push_back(t);
                }
            }
        } else {
            for (auto &t : stk) {
                res.push_back(t);
            }
        }
    }
    EnglishNormalize(res, result);
    // std::string ret = Join(normalize_res, 0);
    // return ret;
}

int RAGAnalyzer::AnalyzeImpl(const Term &input, void *data, bool fine_grained, bool enable_position, HookType func) {
    if (enable_position) {
        auto [tokens, positions] = TokenizeWithPosition(input.text_);

        if (fine_grained) {
            std::vector<std::string> fine_tokens;
            std::vector<std::pair<unsigned, unsigned>> fine_positions;
            FineGrainedTokenizeWithPosition(Join(tokens, 0), positions, fine_tokens, fine_positions);
            tokens = std::move(fine_tokens);
            positions = std::move(fine_positions);
        }

        for (size_t i = 0; i < tokens.size(); ++i) {
            if (tokens[i].empty())
                continue;
            const auto &[start_pos, end_pos] = positions[i];
            func(data, tokens[i].c_str(), tokens[i].size(), start_pos, end_pos, false, 0);
        }
    } else {
        std::string result = Tokenize(input.text_);
        std::vector<std::string> tokens;
        if (fine_grained) {
            FineGrainedTokenize(result, tokens);
        } else {
            Split(result, blank_pattern_, tokens);
        }
        unsigned offset = 0;
        for (auto &t : tokens) {
            if (t.empty())
                continue;
            func(data, t.c_str(), t.size(), offset++, 0, false, 0);
        }
    }
    return 0;
}