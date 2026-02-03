// Copyright(C) 2023 InfiniFlow, Inc. All rights reserved.
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

#pragma once

#include <string>
#include <memory>
#include <cstdint>
#include "term.h"

constexpr unsigned BYTE_MAX = 255;

class TokenizeConfig {
public:
    void AddAllows(std::string astr) { allows_ += astr; }
    void AddDivides(std::string dstr) { divides_ += dstr; }
    void AddUnites(std::string ustr) { unites_ += ustr; }
    std::string allows_;
    std::string divides_;
    std::string unites_;
};

typedef unsigned char CharType;

extern const CharType ALLOW_CHR;     /// < regular term
extern const CharType DELIMITER_CHR; /// < delimiter
extern const CharType SPACE_CHR;     /// < space term
extern const CharType UNITE_CHR;     /// < united term

class CharTypeTable {
    CharType char_type_table_[BYTE_MAX];

public:
    CharTypeTable(bool use_def_delim = true);

    void SetConfig(const TokenizeConfig &conf);

    CharType GetType(uint8_t c) { return char_type_table_[c]; }

    bool IsAllow(uint8_t c) { return char_type_table_[c] == ALLOW_CHR; }

    bool IsDivide(uint8_t c) { return char_type_table_[c] == DELIMITER_CHR; }

    bool IsUnite(uint8_t c) { return char_type_table_[c] == UNITE_CHR; }

    bool IsEqualType(uint8_t c1, uint8_t c2) { return char_type_table_[c1] == char_type_table_[c2]; }
};

class Tokenizer {
public:
    Tokenizer(bool use_def_delim = true) : table_(use_def_delim) { output_buffer_ = std::make_unique<char[]>(output_buffer_size_); }

    ~Tokenizer() {}

    /// \brief set the user defined char types
    /// \param list char type option list
    void SetConfig(const TokenizeConfig &conf);

    /// \brief tokenize the input text, call nextToken(), getToken(), getLength() to get the result.
    /// \param input input text string
    void Tokenize(const std::string &input);

    bool NextToken();

    inline const char *GetToken() { return output_buffer_.get(); }

    inline size_t GetLength() { return output_buffer_cursor_; }

    inline bool IsDelimiter() { return is_delimiter_; }

    inline size_t GetTokenStartCursor() const { return token_start_cursor_; }

    inline size_t GetInputCursor() const { return input_cursor_; }

    bool Tokenize(const std::string &input_string, TermList &special_terms, TermList &prim_terms);

    /// \brief tokenize the input text, remove the space chars, output raw term list
    bool TokenizeWhite(const std::string &input_string, TermList &raw_terms);

    /// \brief tokenize the input text, output two term lists: raw term list and primary term list
    bool Tokenize(const std::string &input_string, TermList &prim_terms);

private:
    bool GrowOutputBuffer();

private:
    CharTypeTable table_;

    std::string *input_{nullptr};

    size_t token_start_cursor_{0};

    size_t input_cursor_{0};

    size_t output_buffer_size_{4096};

    std::unique_ptr<char[]> output_buffer_;

    size_t output_buffer_cursor_{0};

    bool is_delimiter_{false};
};
