/*
* Open Chinese Convert
*
* Copyright 2010 BYVoid <byvoid.kcp@gmail.com>
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*/

#ifndef __OPENCC_DICTIONARY_DATRIE_H_
#define __OPENCC_DICTIONARY_DATRIE_H_

#include "abstract.h"

#define DATRIE_UNUSED -1

typedef struct
{
    int base;
    int parent;
    int word;
} DoubleArrayTrieItem;

dictionary_t dictionary_datrie_open(const char * filename);

int dictionary_datrie_close(dictionary_t t_dictionary);

const ucs4_t * const * dictionary_datrie_match_longest(dictionary_t t_dictionary, const ucs4_t * word,
        size_t maxlen, size_t * match_length);

size_t dictionary_datrie_get_all_match_lengths(dictionary_t t_dictionary, const ucs4_t * word,
        size_t * match_length);

int encode_char(ucs4_t ch);

#endif /* __OPENCC_DICTIONARY_DATRIE_H_ */
