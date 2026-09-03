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

enum Language
{
    STEM_LANG_UNKNOWN = 0,
    STEM_LANG_DANISH = 1,
    STEM_LANG_DUTCH = 2,
    STEM_LANG_ENGLISH,
    STEM_LANG_FINNISH,
    STEM_LANG_FRENCH,
    STEM_LANG_GERMAN,
    STEM_LANG_HUNGARIAN,
    STEM_LANG_ITALIAN,
    STEM_LANG_NORWEGIAN,
    STEM_LANG_PORT,
    STEM_LANG_PORTUGUESE,
    STEM_LANG_ROMANIAN,
    STEM_LANG_RUSSIAN,
    STEM_LANG_SPANISH,
    STEM_LANG_SWEDISH,
    STEM_LANG_TURKISH,
    STEM_LANG_EOS,
};

class Stemmer
{
public:
    Stemmer();

    virtual ~Stemmer();

    bool Init(Language language);

    void DeInit();

    bool Stem(const std::string& term, std::string& resultWord);

private:
    // int stemLang_; ///< language for stemming

    void* stem_function_; ///< stemming function
};
