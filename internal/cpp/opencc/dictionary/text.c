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

#include "text.h"
#include "../encoding.h"

#define INITIAL_DICTIONARY_SIZE 1024
#define ENTRY_BUFF_SIZE 128
#define ENTRY_WBUFF_SIZE ENTRY_BUFF_SIZE / sizeof(size_t)

struct _text_dictionary {
    size_t entry_count;
    size_t max_length;
    entry *lexicon;
    ucs4_t *word_buff;
};
typedef struct _text_dictionary text_dictionary_desc;

int qsort_entry_cmp(const void *a, const void *b) { return ucs4cmp(((entry *)a)->key, ((entry *)b)->key); }

int parse_entry(const char *buff, entry *entry_i) {
    size_t length;
    const char *pbuff;

    /* 解析鍵 */
    for (pbuff = buff; *pbuff != '\t' && *pbuff != '\0'; ++pbuff)
        ;
    if (*pbuff == '\0')
        return -1;
    length = pbuff - buff;

    ucs4_t *ucs4_buff;
    ucs4_buff = utf8_to_ucs4(buff, length);
    if (ucs4_buff == (ucs4_t *)-1)
        return -1;
    entry_i->key = (ucs4_t *)malloc((length + 1) * sizeof(ucs4_t));
    ucs4cpy(entry_i->key, ucs4_buff);
    free(ucs4_buff);

    /* 解析值 */
    size_t value_i, value_count = INITIAL_DICTIONARY_SIZE;
    entry_i->value = (ucs4_t **)malloc(value_count * sizeof(ucs4_t *));

    for (value_i = 0; *pbuff != '\0' && *pbuff != '\n'; ++value_i) {
        if (value_i >= value_count) {
            value_count += value_count;
            entry_i->value = (ucs4_t **)realloc(entry_i->value, value_count * sizeof(ucs4_t *));
        }

        for (buff = ++pbuff; *pbuff != ' ' && *pbuff != '\0' && *pbuff != '\n'; ++pbuff)
            ;
        length = pbuff - buff;
        ucs4_buff = utf8_to_ucs4(buff, length);
        if (ucs4_buff == (ucs4_t *)-1) {
            /* 發生錯誤 回退內存申請 */
            ssize_t i;
            for (i = value_i - 1; i >= 0; --i)
                free(entry_i->value[i]);
            free(entry_i->value);
            free(entry_i->key);
            return -1;
        }

        entry_i->value[value_i] = (ucs4_t *)malloc((length + 1) * sizeof(ucs4_t));
        ucs4cpy(entry_i->value[value_i], ucs4_buff);
        free(ucs4_buff);
    }

    entry_i->value = (ucs4_t **)realloc(entry_i->value, value_count * sizeof(ucs4_t *));
    entry_i->value[value_i] = NULL;

    return 0;
}

dictionary_t dictionary_text_open(const char *filename) {
    text_dictionary_desc *text_dictionary;
    text_dictionary = (text_dictionary_desc *)malloc(sizeof(text_dictionary_desc));
    text_dictionary->entry_count = INITIAL_DICTIONARY_SIZE;
    text_dictionary->max_length = 0;
    text_dictionary->lexicon = (entry *)malloc(sizeof(entry) * text_dictionary->entry_count);
    text_dictionary->word_buff = NULL;

    static char buff[ENTRY_BUFF_SIZE];

    FILE *fp = fopen(filename, "rb");
    if (fp == NULL) {
        dictionary_text_close((dictionary_t)text_dictionary);
        return (dictionary_t)-1;
    }

    size_t i = 0;
    while (fgets(buff, ENTRY_BUFF_SIZE, fp)) {
        if (i >= text_dictionary->entry_count) {
            text_dictionary->entry_count += text_dictionary->entry_count;
            text_dictionary->lexicon = (entry *)realloc(text_dictionary->lexicon, sizeof(entry) * text_dictionary->entry_count);
        }

        if (parse_entry(buff, text_dictionary->lexicon + i) == -1) {
            text_dictionary->entry_count = i;
            dictionary_text_close((dictionary_t)text_dictionary);
            return (dictionary_t)-1;
        }

        size_t length = ucs4len(text_dictionary->lexicon[i].key);
        if (length > text_dictionary->max_length)
            text_dictionary->max_length = length;

        i++;
    }

    fclose(fp);

    text_dictionary->entry_count = i;
    text_dictionary->lexicon = (entry *)realloc(text_dictionary->lexicon, sizeof(entry) * text_dictionary->entry_count);
    text_dictionary->word_buff = (ucs4_t *)malloc(sizeof(ucs4_t) * (text_dictionary->max_length + 1));

    qsort(text_dictionary->lexicon, text_dictionary->entry_count, sizeof(text_dictionary->lexicon[0]), qsort_entry_cmp);

    return (dictionary_t)text_dictionary;
}

void dictionary_text_close(dictionary_t t_dictionary) {
    text_dictionary_desc *text_dictionary = (text_dictionary_desc *)t_dictionary;

    size_t i;
    for (i = 0; i < text_dictionary->entry_count; ++i) {
        free(text_dictionary->lexicon[i].key);

        ucs4_t **j;
        for (j = text_dictionary->lexicon[i].value; *j; ++j) {
            free(*j);
        }
        free(text_dictionary->lexicon[i].value);
    }

    free(text_dictionary->lexicon);
    free(text_dictionary->word_buff);
    free(text_dictionary);
}

const ucs4_t *const *dictionary_text_match_longest(dictionary_t t_dictionary, const ucs4_t *word, size_t maxlen, size_t *match_length) {
    text_dictionary_desc *text_dictionary = (text_dictionary_desc *)t_dictionary;

    if (text_dictionary->entry_count == 0)
        return NULL;

    if (maxlen == 0)
        maxlen = ucs4len(word);
    size_t len = text_dictionary->max_length;
    if (maxlen < len)
        len = maxlen;

    ucs4ncpy(text_dictionary->word_buff, word, len);
    text_dictionary->word_buff[len] = L'\0';

    entry buff;
    buff.key = text_dictionary->word_buff;

    for (; len > 0; len--) {
        text_dictionary->word_buff[len] = L'\0';
        entry *brs =
            (entry *)bsearch(&buff, text_dictionary->lexicon, text_dictionary->entry_count, sizeof(text_dictionary->lexicon[0]), qsort_entry_cmp);

        if (brs != NULL) {
            if (match_length != NULL)
                *match_length = len;
            return (const ucs4_t *const *)brs->value;
        }
    }

    if (match_length != NULL)
        *match_length = 0;
    return NULL;
}

size_t dictionary_text_get_all_match_lengths(dictionary_t t_dictionary, const ucs4_t *word, size_t *match_length) {
    text_dictionary_desc *text_dictionary = (text_dictionary_desc *)t_dictionary;

    size_t rscnt = 0;

    if (text_dictionary->entry_count == 0)
        return rscnt;

    size_t length = ucs4len(word);
    size_t len = text_dictionary->max_length;
    if (length < len)
        len = length;

    ucs4ncpy(text_dictionary->word_buff, word, len);
    text_dictionary->word_buff[len] = L'\0';

    entry buff;
    buff.key = text_dictionary->word_buff;

    for (; len > 0; len--) {
        text_dictionary->word_buff[len] = L'\0';
        entry *brs =
            (entry *)bsearch(&buff, text_dictionary->lexicon, text_dictionary->entry_count, sizeof(text_dictionary->lexicon[0]), qsort_entry_cmp);

        if (brs != NULL)
            match_length[rscnt++] = len;
    }

    return rscnt;
}

size_t dictionary_text_get_lexicon(dictionary_t t_dictionary, entry *lexicon) {
    text_dictionary_desc *text_dictionary = (text_dictionary_desc *)t_dictionary;

    size_t i;
    for (i = 0; i < text_dictionary->entry_count; i++) {
        lexicon[i].key = text_dictionary->lexicon[i].key;
        lexicon[i].value = text_dictionary->lexicon[i].value;
    }

    return text_dictionary->entry_count;
}
