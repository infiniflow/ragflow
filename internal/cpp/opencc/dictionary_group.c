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

#include "dictionary_group.h"

#define DICTIONARY_MAX_COUNT 128

struct _dictionary_group {
    size_t count;
    dictionary_t dicts[DICTIONARY_MAX_COUNT];
};
typedef struct _dictionary_group dictionary_group_desc;

static dictionary_error errnum = DICTIONARY_ERROR_VOID;

dictionary_group_t dictionary_group_open(void) {
    dictionary_group_desc *dictionary_group = (dictionary_group_desc *)malloc(sizeof(dictionary_group_desc));

    dictionary_group->count = 0;

    return dictionary_group;
}

void dictionary_group_close(dictionary_group_t t_dictionary) {
    dictionary_group_desc *dictionary_group = (dictionary_group_desc *)t_dictionary;

    size_t i;
    for (i = 0; i < dictionary_group->count; i++)
        dictionary_close(dictionary_group->dicts[i]);

    free(dictionary_group);
}

int dictionary_group_load(dictionary_group_t t_dictionary, const char *filename, const char *home_path, opencc_dictionary_type type) {
    dictionary_group_desc *dictionary_group = (dictionary_group_desc *)t_dictionary;
    dictionary_t dictionary;

    FILE *fp = fopen(filename, "rb");
    if (!fp) {
        char *new_filename = (char *)malloc(sizeof(char) * (strlen(filename) + strlen(home_path) + 2));
        sprintf(new_filename, "%s/%s", home_path, filename);

        fp = fopen(new_filename, "rb");
        if (!fp) {
            free(new_filename);
            errnum = DICTIONARY_ERROR_CANNOT_ACCESS_DICTFILE;
            return -1;
        }
        dictionary = dictionary_open(new_filename, type);
        free(new_filename);
    } else {
        dictionary = dictionary_open(filename, type);
    }
    fclose(fp);

    if (dictionary == (dictionary_t)-1) {
        errnum = DICTIONARY_ERROR_INVALID_DICT;
        return -1;
    }
    dictionary_group->dicts[dictionary_group->count++] = dictionary;
    return 0;
}

dictionary_t dictionary_group_get_dictionary(dictionary_group_t t_dictionary, size_t index) {
    dictionary_group_desc *dictionary_group = (dictionary_group_desc *)t_dictionary;

    if (index < 0 || index >= dictionary_group->count) {
        errnum = DICTIONARY_ERROR_INVALID_INDEX;
        return (dictionary_t)-1;
    }

    return dictionary_group->dicts[index];
}

size_t dictionary_group_count(dictionary_group_t t_dictionary) {
    dictionary_group_desc *dictionary_group = (dictionary_group_desc *)t_dictionary;
    return dictionary_group->count;
}

const ucs4_t *const *dictionary_group_match_longest(dictionary_group_t t_dictionary, const ucs4_t *word, size_t maxlen, size_t *match_length) {
    dictionary_group_desc *dictionary_group = (dictionary_group_desc *)t_dictionary;

    if (dictionary_group->count == 0) {
        errnum = DICTIONARY_ERROR_NODICT;
        return (const ucs4_t *const *)-1;
    }

    const ucs4_t *const *retval = NULL;
    size_t t_match_length, max_length = 0;

    size_t i;
    for (i = 0; i < dictionary_group->count; i++) {
        /* 依次查找每個辭典，取得最長匹配長度 */
        const ucs4_t *const *t_retval = dictionary_match_longest(dictionary_group->dicts[i], word, maxlen, &t_match_length);

        if (t_retval != NULL) {
            if (t_match_length > max_length) {
                max_length = t_match_length;
                retval = t_retval;
            }
        }
    }

    if (match_length != NULL) {
        *match_length = max_length;
    }

    return retval;
}

size_t dictionary_group_get_all_match_lengths(dictionary_group_t t_dictionary, const ucs4_t *word, size_t *match_length) {
    dictionary_group_desc *dictionary_group = (dictionary_group_desc *)t_dictionary;

    if (dictionary_group->count == 0) {
        errnum = DICTIONARY_ERROR_NODICT;
        return (size_t)-1;
    }

    size_t rscnt = 0;
    size_t i;
    for (i = 0; i < dictionary_group->count; i++) {
        size_t retval;
        retval = dictionary_get_all_match_lengths(dictionary_group->dicts[i], word, match_length + rscnt);
        rscnt += retval;
        /* 去除重複長度 */
        if (i > 0 && rscnt > 1) {
            qsort(match_length, rscnt, sizeof(match_length[0]), qsort_int_cmp);
            int j, k;
            for (j = 0, k = 1; k < rscnt; k++) {
                if (match_length[k] != match_length[j])
                    match_length[++j] = match_length[k];
            }
            rscnt = j + 1;
        }
    }
    return rscnt;
}

dictionary_error dictionary_errno(void) { return errnum; }

void dictionary_perror(const char *spec) {
    perr(spec);
    perr("\n");
    switch (errnum) {
        case DICTIONARY_ERROR_VOID:
            break;
        case DICTIONARY_ERROR_NODICT:
            perr(_("No dictionary loaded"));
            break;
        case DICTIONARY_ERROR_CANNOT_ACCESS_DICTFILE:
            perror(_("Can not open dictionary file"));
            break;
        case DICTIONARY_ERROR_INVALID_DICT:
            perror(_("Invalid dictionary file"));
            break;
        case DICTIONARY_ERROR_INVALID_INDEX:
            perror(_("Invalid dictionary index"));
            break;
        default:
            perr(_("Unknown"));
    }
}
