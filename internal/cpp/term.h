//
// Created by infiniflow on 1/31/26.
//

#pragma once

#include <string>
#include <cstdint>
#include <deque>

class Term {
public:
    Term() : word_offset_(0), end_offset_(0), payload_(0) {
    }

    Term(const std::string &str) : text_(str), word_offset_(0), end_offset_(0), payload_(0) {
    }

    ~Term() {
    }

    void Reset();

    uint32_t Length() { return text_.length(); }

    std::string Text() const { return text_; }

public:
    std::string text_;
    uint32_t word_offset_;
    uint32_t end_offset_;
    uint16_t payload_;
};

class TermList : public std::deque<Term> {
public:
    void Add(const char *text, const uint32_t len, const uint32_t offset, const uint32_t end_offset,
             const uint16_t payload = 0) {
        push_back(global_temporary_);
        back().text_.assign(text, len);
        back().word_offset_ = offset;
        back().end_offset_ = end_offset;
        back().payload_ = payload;
    }

    // void Add(cppjieba::Word &cut_word) {
    //     push_back(global_temporary_);
    //     std::swap(back().text_, cut_word.word);
    //     back().word_offset_ = cut_word.offset;
    // }

    void Add(const std::string &token, const uint32_t offset, const uint32_t end_offset, const uint16_t payload = 0) {
        push_back(global_temporary_);
        back().text_ = token;
        back().word_offset_ = offset;
        back().end_offset_ = end_offset;
        back().payload_ = payload;
    }

    void Add(std::string &token, const uint32_t offset, const uint32_t end_offset, const uint16_t payload = 0) {
        push_back(global_temporary_);
        std::swap(back().text_, token);
        back().word_offset_ = offset;
        back().end_offset_ = end_offset;
        back().payload_ = payload;
    }

private:
    static Term global_temporary_;
};

extern std::string PLACE_HOLDER;
