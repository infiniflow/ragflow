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

#include "api.h"
#include "stem_UTF_8_danish.h"
#include "stem_UTF_8_dutch.h"
#include "stem_UTF_8_english.h"
#include "stem_UTF_8_finnish.h"
#include "stem_UTF_8_french.h"
#include "stem_UTF_8_german.h"
#include "stem_UTF_8_hungarian.h"
#include "stem_UTF_8_italian.h"
#include "stem_UTF_8_norwegian.h"
#include "stem_UTF_8_porter.h"
#include "stem_UTF_8_portuguese.h"
#include "stem_UTF_8_romanian.h"
#include "stem_UTF_8_russian.h"
#include "stem_UTF_8_spanish.h"
#include "stem_UTF_8_swedish.h"
#include "stem_UTF_8_turkish.h"
#include "stemmer.h"

#ifdef __cplusplus

extern "C" {
#endif
struct StemFunc {

    struct SN_env *(*create)(void);
    void (*close)(struct SN_env *);
    int (*stem)(struct SN_env *);

    struct SN_env *env;
};

#ifdef __cplusplus
}
#endif

StemFunc STEM_FUNCTION[STEM_LANG_EOS] = {
    {0, 0, 0, 0},
    {danish_UTF_8_create_env, danish_UTF_8_close_env, danish_UTF_8_stem, 0},
    {dutch_UTF_8_create_env, dutch_UTF_8_close_env, dutch_UTF_8_stem, 0},
    {english_UTF_8_create_env, english_UTF_8_close_env, english_UTF_8_stem, 0},
    {finnish_UTF_8_create_env, finnish_UTF_8_close_env, finnish_UTF_8_stem, 0},
    {french_UTF_8_create_env, french_UTF_8_close_env, french_UTF_8_stem, 0},
    {german_UTF_8_create_env, german_UTF_8_close_env, german_UTF_8_stem, 0},
    {hungarian_UTF_8_create_env, hungarian_UTF_8_close_env, hungarian_UTF_8_stem, 0},
    {italian_UTF_8_create_env, italian_UTF_8_close_env, italian_UTF_8_stem, 0},
    {norwegian_UTF_8_create_env, norwegian_UTF_8_close_env, norwegian_UTF_8_stem, 0},
    {porter_UTF_8_create_env, porter_UTF_8_close_env, porter_UTF_8_stem, 0},
    {portuguese_UTF_8_create_env, portuguese_UTF_8_close_env, portuguese_UTF_8_stem, 0},
    {romanian_UTF_8_create_env, romanian_UTF_8_close_env, romanian_UTF_8_stem, 0},
    {russian_UTF_8_create_env, russian_UTF_8_close_env, russian_UTF_8_stem, 0},
    {spanish_UTF_8_create_env, spanish_UTF_8_close_env, spanish_UTF_8_stem, 0},
    {swedish_UTF_8_create_env, swedish_UTF_8_close_env, swedish_UTF_8_stem, 0},
    {turkish_UTF_8_create_env, turkish_UTF_8_close_env, turkish_UTF_8_stem, 0},
};

Stemmer::Stemmer() {
    // stemLang_ = STEM_LANG_UNKNOWN;
    stem_function_ = 0;
}

Stemmer::~Stemmer() { DeInit(); }

bool Stemmer::Init(Language language) {
    // create stemming function structure
    stem_function_ = static_cast<void *>(new StemFunc);
    if (stem_function_ == 0) {
        return false;
    }

    // set stemming functions
    if (language > 0 && language < STEM_LANG_EOS) {
        static_cast<StemFunc *>(stem_function_)->create = STEM_FUNCTION[language].create;
        static_cast<StemFunc *>(stem_function_)->close = STEM_FUNCTION[language].close;
        static_cast<StemFunc *>(stem_function_)->stem = STEM_FUNCTION[language].stem;
        static_cast<StemFunc *>(stem_function_)->env = STEM_FUNCTION[language].env;
    } else {
        delete static_cast<StemFunc *>(stem_function_);
        stem_function_ = 0;
        return false;
    }

    // create env
    static_cast<StemFunc *>(stem_function_)->env = static_cast<StemFunc *>(stem_function_)->create();
    if (static_cast<StemFunc *>(stem_function_)->env == 0) {
        DeInit();
        return false;
    }

    return true;
}
////////////
// struct SN_env {
//     symbol *p;
//     int c;
//     int l;
//     int lb;
//     int bra;
//     int ket;
//     symbol **S;
//     int *I;
//     unsigned char *B;
// };
////////////

void Stemmer::DeInit(void) {
    if (stem_function_) {
        static_cast<StemFunc *>(stem_function_)->close(((StemFunc *)stem_function_)->env);
        delete static_cast<StemFunc *>(stem_function_);
        stem_function_ = 0;
    }
}

bool Stemmer::Stem(const std::string &term, std::string &resultWord) {
    if (!stem_function_) {
        return false;
    }

    // set environment
    if (SN_set_current(static_cast<StemFunc *>(stem_function_)->env, term.length(), (const symbol *)term.c_str())) {
        static_cast<StemFunc *>(stem_function_)->env->l = 0;
        return false;
    }

    // stemming
    if (((StemFunc *)stem_function_)->stem(((StemFunc *)stem_function_)->env) < 0) {
        return false;
    }

    ((StemFunc *)stem_function_)->env->p[((StemFunc *)stem_function_)->env->l] = 0;

    resultWord = (char *)((StemFunc *)stem_function_)->env->p;

    return true;
}
