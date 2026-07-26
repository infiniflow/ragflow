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

#include "datrie.h"
#include <fcntl.h>
#include <unistd.h>

#ifdef __WIN32
/* Todo: Win32 mmap*/
#else
#include <sys/mman.h>
#define MMAP_ENABLED
#endif

typedef enum { MEMORY_TYPE_MMAP, MEMORY_TYPE_ALLOCATE } memory_type;

struct _datrie_dictionary {
    const DoubleArrayTrieItem *dat;
    uint32_t dat_item_count;
    ucs4_t *lexicon;
    uint32_t lexicon_count;

    ucs4_t ***lexicon_set;
    void *dic_memory;
    size_t dic_size;
    memory_type dic_memory_type;
};
typedef struct _datrie_dictionary datrie_dictionary_desc;

static int load_allocate(datrie_dictionary_desc *datrie_dictionary, int fd) {
    datrie_dictionary->dic_memory_type = MEMORY_TYPE_ALLOCATE;
    datrie_dictionary->dic_memory = malloc(datrie_dictionary->dic_size);
    if (datrie_dictionary->dic_memory == NULL) {
        /* 內存申請失敗 */
        return -1;
    }
    lseek(fd, 0, SEEK_SET);
    if (read(fd, datrie_dictionary->dic_memory, datrie_dictionary->dic_size) == -1) {
        /* 讀取失敗 */
        return -1;
    }
    return 0;
}

static int load_mmap(datrie_dictionary_desc *datrie_dictionary, int fd) {
#ifdef MMAP_ENABLED
    datrie_dictionary->dic_memory_type = MEMORY_TYPE_MMAP;
    datrie_dictionary->dic_memory = mmap(NULL, datrie_dictionary->dic_size, PROT_READ, MAP_PRIVATE, fd, 0);
    if (datrie_dictionary->dic_memory == MAP_FAILED) {
        /* 內存映射創建失敗 */
        datrie_dictionary->dic_memory = NULL;
        return -1;
    }
    return 0;
#else
    return -1;
#endif
}

static int load_dict(datrie_dictionary_desc *datrie_dictionary, FILE *fp) {
    int fd = fileno(fp);

    fseek(fp, 0, SEEK_END);
    datrie_dictionary->dic_size = ftell(fp);

    /* 首先嘗試mmap，如果失敗嘗試申請內存 */
    if (load_mmap(datrie_dictionary, fd) == -1) {
        if (load_allocate(datrie_dictionary, fd) == -1) {
            return -1;
        }
    }

    size_t header_len = strlen("OPENCCDATRIE");

    if (strncmp((const char *)datrie_dictionary->dic_memory, "OPENCCDATRIE", header_len) != 0) {
        return -1;
    }

    size_t offset = 0;

    offset += header_len * sizeof(char);

    /* 詞彙表 */
    uint32_t lexicon_length = *((uint32_t *)(datrie_dictionary->dic_memory + offset));
    offset += sizeof(uint32_t);

    datrie_dictionary->lexicon = (ucs4_t *)(datrie_dictionary->dic_memory + offset);
    offset += lexicon_length * sizeof(ucs4_t);

    /* 詞彙索引表 */
    uint32_t lexicon_index_length = *((uint32_t *)(datrie_dictionary->dic_memory + offset));
    offset += sizeof(uint32_t);

    uint32_t *lexicon_index = (uint32_t *)(datrie_dictionary->dic_memory + offset);
    offset += lexicon_index_length * sizeof(uint32_t);

    datrie_dictionary->lexicon_count = *((uint32_t *)(datrie_dictionary->dic_memory + offset));
    offset += sizeof(uint32_t);

    datrie_dictionary->dat_item_count = *((uint32_t *)(datrie_dictionary->dic_memory + offset));
    offset += sizeof(uint32_t);

    datrie_dictionary->dat = (DoubleArrayTrieItem *)(datrie_dictionary->dic_memory + offset);

    /* 構造索引表 */
    datrie_dictionary->lexicon_set = (ucs4_t ***)malloc(datrie_dictionary->lexicon_count * sizeof(ucs4_t **));
    size_t i, last = 0;
    for (i = 0; i < datrie_dictionary->lexicon_count; i++) {
        size_t count, j;
        for (j = last; j < lexicon_index_length; j++) {
            if (lexicon_index[j] == (uint32_t)-1)
                break;
        }
        count = j - last;

        datrie_dictionary->lexicon_set[i] = (ucs4_t **)malloc((count + 1) * sizeof(ucs4_t *));
        for (j = 0; j < count; j++) {
            datrie_dictionary->lexicon_set[i][j] = datrie_dictionary->lexicon + lexicon_index[last + j];
        }
        datrie_dictionary->lexicon_set[i][count] = NULL;
        last += j + 1;
    }

    return 0;
}

static int unload_dict(datrie_dictionary_desc *datrie_dictionary) {
    if (datrie_dictionary->dic_memory != NULL) {
        size_t i;
        for (i = 0; i < datrie_dictionary->lexicon_count; i++) {
            free(datrie_dictionary->lexicon_set[i]);
        }
        free(datrie_dictionary->lexicon_set);

        if (MEMORY_TYPE_MMAP == datrie_dictionary->dic_memory_type) {
#ifdef MMAP_ENABLED
            return munmap(datrie_dictionary->dic_memory, datrie_dictionary->dic_size);
#else
            debug_should_not_be_here();
#endif
        } else if (MEMORY_TYPE_ALLOCATE == datrie_dictionary->dic_memory_type) {
            free(datrie_dictionary->dic_memory);
        } else {
            return -1;
        }
    }
    return 0;
}

dictionary_t dictionary_datrie_open(const char *filename) {
    datrie_dictionary_desc *datrie_dictionary = (datrie_dictionary_desc *)malloc(sizeof(datrie_dictionary_desc));
    datrie_dictionary->dat = NULL;
    datrie_dictionary->lexicon = NULL;

    FILE *fp = fopen(filename, "rb");

    if (load_dict(datrie_dictionary, fp) == -1) {
        dictionary_datrie_close((dictionary_t)datrie_dictionary);
        return (dictionary_t)-1;
    }

    fclose(fp);

    return (dictionary_t)datrie_dictionary;
}

int dictionary_datrie_close(dictionary_t t_dictionary) {
    datrie_dictionary_desc *datrie_dictionary = (datrie_dictionary_desc *)t_dictionary;

    if (unload_dict(datrie_dictionary) == -1) {
        free(datrie_dictionary);
        return -1;
    }

    free(datrie_dictionary);
    return 0;
}

int encode_char(ucs4_t ch) { return (int)ch; }

void datrie_match(const datrie_dictionary_desc *datrie_dictionary, const ucs4_t *word, size_t *match_pos, size_t *id, size_t limit) {
    size_t i, p;
    for (i = 0, p = 0; word[p] && (limit == 0 || p < limit) && datrie_dictionary->dat[i].base != DATRIE_UNUSED; p++) {
        int k = encode_char(word[p]);
        int j = datrie_dictionary->dat[i].base + k;
        if (j < 0 || j >= datrie_dictionary->dat_item_count || datrie_dictionary->dat[j].parent != i)
            break;
        i = j;
    }
    if (match_pos)
        *match_pos = p;
    if (id)
        *id = i;
}

const ucs4_t *const *dictionary_datrie_match_longest(dictionary_t t_dictionary, const ucs4_t *word, size_t maxlen, size_t *match_length) {
    datrie_dictionary_desc *datrie_dictionary = (datrie_dictionary_desc *)t_dictionary;

    size_t pos, item;
    datrie_match(datrie_dictionary, word, &pos, &item, maxlen);

    while (datrie_dictionary->dat[item].word == -1 && pos > 1)
        datrie_match(datrie_dictionary, word, &pos, &item, pos - 1);

    if (pos == 0 || datrie_dictionary->dat[item].word == -1) {
        if (match_length != NULL)
            *match_length = 0;
        return NULL;
    }

    if (match_length != NULL)
        *match_length = pos;

    return (const ucs4_t *const *)datrie_dictionary->lexicon_set[datrie_dictionary->dat[item].word];
}

size_t dictionary_datrie_get_all_match_lengths(dictionary_t t_dictionary, const ucs4_t *word, size_t *match_length) {
    datrie_dictionary_desc *datrie_dictionary = (datrie_dictionary_desc *)t_dictionary;

    size_t rscnt = 0;

    size_t i, p;
    for (i = 0, p = 0; word[p] && datrie_dictionary->dat[i].base != DATRIE_UNUSED; p++) {
        int k = encode_char(word[p]);
        int j = datrie_dictionary->dat[i].base + k;
        if (j < 0 || j >= datrie_dictionary->dat_item_count || datrie_dictionary->dat[j].parent != i)
            break;
        i = j;

        if (datrie_dictionary->dat[i].word != -1)
            match_length[rscnt++] = p + 1;
    }

    return rscnt;
}
