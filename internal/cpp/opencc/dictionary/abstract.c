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

#include "abstract.h"
#include "datrie.h"
#include "text.h"

struct _dictionary {
    opencc_dictionary_type type;
    dictionary_t dict;
};
typedef struct _dictionary dictionary_desc;

dictionary_t dictionary_open(const char *filename, opencc_dictionary_type type) {
    dictionary_desc *dictionary = (dictionary_desc *)malloc(sizeof(dictionary_desc));
    dictionary->type = type;
    switch (type) {
        case OPENCC_DICTIONARY_TYPE_TEXT:
            dictionary->dict = dictionary_text_open(filename);
            break;
        case OPENCC_DICTIONARY_TYPE_DATRIE:
            dictionary->dict = dictionary_datrie_open(filename);
            break;
        default:
            free(dictionary);
            dictionary = (dictionary_t)-1; /* TODO:辭典格式不支持 */
    }
    return dictionary;
}

dictionary_t dictionary_get(dictionary_t t_dictionary) {
    dictionary_desc *dictionary = (dictionary_desc *)t_dictionary;
    return dictionary->dict;
}

void dictionary_close(dictionary_t t_dictionary) {
    dictionary_desc *dictionary = (dictionary_desc *)t_dictionary;
    switch (dictionary->type) {
        case OPENCC_DICTIONARY_TYPE_TEXT:
            dictionary_text_close(dictionary->dict);
            break;
        case OPENCC_DICTIONARY_TYPE_DATRIE:
            dictionary_datrie_close(dictionary->dict);
            break;
        default:
            debug_should_not_be_here();
    }
    free(dictionary);
}

const ucs4_t *const *dictionary_match_longest(dictionary_t t_dictionary, const ucs4_t *word, size_t maxlen, size_t *match_length) {
    dictionary_desc *dictionary = (dictionary_desc *)t_dictionary;
    switch (dictionary->type) {
        case OPENCC_DICTIONARY_TYPE_TEXT:
            return dictionary_text_match_longest(dictionary->dict, word, maxlen, match_length);
            break;
        case OPENCC_DICTIONARY_TYPE_DATRIE:
            return dictionary_datrie_match_longest(dictionary->dict, word, maxlen, match_length);
            break;
        default:
            debug_should_not_be_here();
    }
    return (const ucs4_t *const *)-1;
}

size_t dictionary_get_all_match_lengths(dictionary_t t_dictionary, const ucs4_t *word, size_t *match_length) {
    dictionary_desc *dictionary = (dictionary_desc *)t_dictionary;
    switch (dictionary->type) {
        case OPENCC_DICTIONARY_TYPE_TEXT:
            return dictionary_text_get_all_match_lengths(dictionary->dict, word, match_length);
            break;
        case OPENCC_DICTIONARY_TYPE_DATRIE:
            return dictionary_datrie_get_all_match_lengths(dictionary->dict, word, match_length);
            break;
        default:
            debug_should_not_be_here();
    }
    return (size_t)-1;
}
