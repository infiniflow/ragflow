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

#include "dictionary_set.h"

#define DICTIONARY_GROUP_MAX_COUNT 128

struct _dictionary_set {
    size_t count;
    dictionary_group_t groups[DICTIONARY_GROUP_MAX_COUNT];
};
typedef struct _dictionary_set dictionary_set_desc;

dictionary_set_t dictionary_set_open(void) {
    dictionary_set_desc *dictionary_set = (dictionary_set_desc *)malloc(sizeof(dictionary_set_desc));

    dictionary_set->count = 0;

    return dictionary_set;
}

void dictionary_set_close(dictionary_set_t t_dictionary) {
    dictionary_set_desc *dictionary_set = (dictionary_set_desc *)t_dictionary;

    size_t i;
    for (i = 0; i < dictionary_set->count; i++)
        dictionary_group_close(dictionary_set->groups[i]);

    free(dictionary_set);
}

dictionary_group_t dictionary_set_new_group(dictionary_set_t t_dictionary) {
    dictionary_set_desc *dictionary_set = (dictionary_set_desc *)t_dictionary;

    if (dictionary_set->count + 1 == DICTIONARY_GROUP_MAX_COUNT) {
        return (dictionary_group_t)-1;
    }

    dictionary_group_t group = dictionary_group_open();
    dictionary_set->groups[dictionary_set->count++] = group;

    return group;
}

dictionary_group_t dictionary_set_get_group(dictionary_set_t t_dictionary, size_t index) {
    dictionary_set_desc *dictionary_set = (dictionary_set_desc *)t_dictionary;

    if (index < 0 || index >= dictionary_set->count) {
        return (dictionary_group_t)-1;
    }

    return dictionary_set->groups[index];
}

size_t dictionary_set_count_group(dictionary_set_t t_dictionary) {
    dictionary_set_desc *dictionary_set = (dictionary_set_desc *)t_dictionary;
    return dictionary_set->count;
}
