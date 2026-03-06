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

#include "tokenizer.h"
#include <cstring>
#include <cstdint>

const CharType ALLOW_CHR = 0; /// < regular term
const CharType DELIMITER_CHR = 1; /// < delimiter
const CharType SPACE_CHR = 2; /// < space term
const CharType UNITE_CHR = 3; /// < united term

CharTypeTable::CharTypeTable(bool use_def_delim) {
    memset(char_type_table_, 0, BYTE_MAX);
    // if use_def_delim is set, all the characters are allows
    if (!use_def_delim)
        return;
    // set the lower 4 bit to record default char type
    for (uint8_t i = 0; i < BYTE_MAX; i++) {
        if (std::isalnum(i) || i > 127)
            continue;
        else if (std::isspace(i))
            char_type_table_[i] = SPACE_CHR;
        else
            char_type_table_[i] = DELIMITER_CHR;
    }
}

void CharTypeTable::SetConfig(const TokenizeConfig &conf) {
    // set the higher 4 bit to record user defined option type
    std::string str; // why need to copy?

    str = conf.divides_;
    if (!str.empty()) {
        for (unsigned int j = 0; j < str.length(); j++) {
            char_type_table_[(uint8_t)str[j]] = DELIMITER_CHR;
        }
    }

    str = conf.unites_;
    if (!str.empty()) {
        for (unsigned int j = 0; j < str.length(); j++) {
            char_type_table_[(uint8_t)str[j]] = UNITE_CHR;
        }
    }

    str = conf.allows_;
    if (!str.empty()) {
        for (unsigned int j = 0; j < str.length(); j++) {
            char_type_table_[(uint8_t)str[j]] = ALLOW_CHR;
        }
    }
}

void Tokenizer::SetConfig(const TokenizeConfig &conf) { table_.SetConfig(conf); }

void Tokenizer::Tokenize(const std::string &input) {
    input_ = (std::string *)&input;
    input_cursor_ = 0;
}

bool Tokenizer::NextToken() {
    while (input_cursor_ < input_->length() && table_.GetType(input_->at(input_cursor_)) == SPACE_CHR) {
        input_cursor_++;
    }
    if (input_cursor_ == input_->length())
        return false;

    output_buffer_cursor_ = 0;

    if (output_buffer_cursor_ >= output_buffer_size_) {
        GrowOutputBuffer();
    }
    token_start_cursor_ = input_cursor_;
    output_buffer_[output_buffer_cursor_++] = input_->at(input_cursor_);
    if (table_.GetType(input_->at(input_cursor_)) == DELIMITER_CHR) {
        ++input_cursor_;
        is_delimiter_ = true;
        return true;
    } else {
        ++input_cursor_;
        is_delimiter_ = false;

        while (input_cursor_ < input_->length()) {
            CharType cur_type = table_.GetType(input_->at(input_cursor_));
            if (cur_type == SPACE_CHR || cur_type == DELIMITER_CHR) {
                return true;
            } else if (cur_type == ALLOW_CHR) {
                if (output_buffer_cursor_ >= output_buffer_size_) {
                    GrowOutputBuffer();
                }
                output_buffer_[output_buffer_cursor_++] = input_->at(input_cursor_++);
            } else {
                ++input_cursor_;
            }
        }
        return true;
    }
}

bool Tokenizer::GrowOutputBuffer() {
    output_buffer_size_ *= 2;
    output_buffer_ = std::make_unique<char[]>(output_buffer_size_);
    return true;
}

bool Tokenizer::Tokenize(const std::string &input_string, TermList &special_terms, TermList &prim_terms) {
    special_terms.clear();
    prim_terms.clear();

    size_t len = input_string.length();
    if (len == 0)
        return false;

    Term t;
    TermList::iterator it;

    unsigned int word_off = 0, char_off = 0;

    char cur_char;
    CharType cur_type;

    for (char_off = 0; char_off < len;) // char_off++ )   // char_off is always incremented inside
    {
        cur_type = table_.GetType(input_string.at(char_off));

        if (cur_type == ALLOW_CHR || cur_type == UNITE_CHR) {
            it = prim_terms.insert(prim_terms.end(), t);

            do {
                cur_char = input_string.at(char_off);
                cur_type = table_.GetType(cur_char);

                if (cur_type == ALLOW_CHR) {
                    it->text_ += cur_char;
                } else if (cur_type == SPACE_CHR || cur_type == DELIMITER_CHR) {
                    break;
                }

                char_off++;
            } while (char_off < len);

            if (it->text_.length() == 0) {
                prim_terms.erase(it);
                continue;
                // char_off--;
            }

            it->word_offset_ = word_off++;

            // char_off--;
        } else if (cur_type == DELIMITER_CHR) {

            it = special_terms.insert(special_terms.end(), t);

            do {
                cur_char = input_string.at(char_off);
                cur_type = table_.GetType(cur_char);

                if (cur_type == DELIMITER_CHR)
                    it->text_ += cur_char;
                else
                    break;
                char_off++;
            } while (char_off < len);

            it->word_offset_ = word_off++;

            // char_off--;
        } else
            char_off++;
    }

    return true;
}

bool Tokenizer::Tokenize(const std::string &input_string, TermList &prim_terms) {
    prim_terms.clear();
    size_t len = input_string.length();
    if (len == 0)
        return false;

    Term t;
    TermList::iterator it;

    unsigned int word_off = 0, char_off = 0;

    char cur_char;
    CharType cur_type;

    for (char_off = 0; char_off < len;) // char_off++ )
    {
        cur_type = table_.GetType(input_string.at(char_off));

        if (cur_type == ALLOW_CHR || cur_type == UNITE_CHR) {

            it = prim_terms.insert(prim_terms.end(), t);
            // it->begin_ = char_off;

            do {
                cur_char = input_string.at(char_off);
                cur_type = table_.GetType(cur_char);

                if (cur_type == ALLOW_CHR) {
                    it->text_ += cur_char;
                } else if (cur_type == SPACE_CHR || cur_type == DELIMITER_CHR) {
                    break;
                }

                char_off++;
            } while (char_off < len);

            if (it->text_.length() == 0) {
                prim_terms.erase(it);
                continue;
                // char_off--;
            }

            it->word_offset_ = word_off++;

            // char_off--;
        } else if (cur_type == DELIMITER_CHR) {
            if (((char_off + 1) < len) && table_.GetType(input_string.at(char_off + 1)) != DELIMITER_CHR) {
                word_off++;
            }
            char_off++;
        } else
            char_off++;
    }

    return true;
}

bool Tokenizer::TokenizeWhite(const std::string &input_string, TermList &raw_terms) {
    raw_terms.clear();

    size_t len = input_string.length();
    if (len == 0)
        return false;

    Term t;
    TermList::iterator it;

    unsigned int word_off = 0, char_off = 0;

    char cur_char;
    CharType cur_type;
    // CharType cur_type, preType;

    for (char_off = 0; char_off < len;) // char_off++ )
    {
        cur_type = table_.GetType(input_string.at(char_off));

        if (cur_type == ALLOW_CHR || cur_type == UNITE_CHR) {
            it = raw_terms.insert(raw_terms.end(), t);
            // it->begin_ = char_off;

            do {
                cur_char = input_string.at(char_off);
                cur_type = table_.GetType(cur_char);

                if (cur_type == ALLOW_CHR) {
                    it->text_ += cur_char;
                } else if (cur_type == SPACE_CHR || cur_type == DELIMITER_CHR) {
                    break;
                }

                char_off++;
            } while (char_off < len);

            if (it->text_.length() == 0) {
                raw_terms.erase(it);
                continue;
                // char_off--;
            }

            it->word_offset_ = word_off++;

            // char_off--;
        } else if (cur_type == DELIMITER_CHR) {

            it = raw_terms.insert(raw_terms.end(), t);

            do {
                cur_char = input_string.at(char_off);
                cur_type = table_.GetType(cur_char);
                if (cur_type == DELIMITER_CHR)
                    it->text_ += cur_char;
                else
                    break;
                char_off++;
            } while (char_off < len);

            it->word_offset_ = word_off++;

            // char_off--;
        } else {
            // SPACE_CHR  nothing to do
            char_off++;
        }
    }

    return true;
}