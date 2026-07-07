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

#ifndef __DICTIONARY_GROUP_H_
#define __DICTIONARY_GROUP_H_

#include "utils.h"
#include "dictionary/abstract.h"

typedef void * dictionary_group_t;

typedef enum
{
    DICTIONARY_ERROR_VOID,
    DICTIONARY_ERROR_NODICT,
    DICTIONARY_ERROR_CANNOT_ACCESS_DICTFILE,
    DICTIONARY_ERROR_INVALID_DICT,
    DICTIONARY_ERROR_INVALID_INDEX,
} dictionary_error;

dictionary_group_t dictionary_group_open(void);

void dictionary_group_close(dictionary_group_t t_dictionary);

int dictionary_group_load(dictionary_group_t t_dictionary, const char * filename, const char* home_dir,
                          opencc_dictionary_type type);

const ucs4_t * const * dictionary_group_match_longest(dictionary_group_t t_dictionary, const ucs4_t * word,
        size_t maxlen, size_t * match_length);

size_t dictionary_group_get_all_match_lengths(dictionary_group_t t_dictionary, const ucs4_t * word,
        size_t * match_length);

dictionary_t dictionary_group_get_dictionary(dictionary_group_t t_dictionary, size_t index);

size_t dictionary_group_count(dictionary_group_t t_dictionary);

dictionary_error dictionary_errno(void);

void dictionary_perror(const char * spec);

#endif /* __DICTIONARY_GROUP_H_ */
