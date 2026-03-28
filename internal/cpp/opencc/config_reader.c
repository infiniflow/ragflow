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

#include "config_reader.h"
#include "dictionary_set.h"

#include <stdio.h>

#define BUFFER_SIZE 8192
#define DICTIONARY_MAX_COUNT 1024
#define CONFIG_DICT_TYPE_OCD "OCD"
#define CONFIG_DICT_TYPE_TEXT "TEXT"

typedef struct {
    opencc_dictionary_type dict_type;
    char *file_name;
    size_t index;
    size_t stamp;
} dictionary_buffer;

struct _config_desc {
    char *title;
    char *description;
    dictionary_set_t dictionary_set;
    char *home_dir;
    dictionary_buffer dicts[DICTIONARY_MAX_COUNT];
    size_t dicts_count;
    size_t stamp;
};
typedef struct _config_desc config_desc;

static config_error errnum = CONFIG_ERROR_VOID;

static int qsort_dictionary_buffer_cmp(const void *a, const void *b) {
    if (((dictionary_buffer *)a)->index < ((dictionary_buffer *)b)->index)
        return -1;
    if (((dictionary_buffer *)a)->index > ((dictionary_buffer *)b)->index)
        return 1;
    return ((dictionary_buffer *)a)->stamp < ((dictionary_buffer *)b)->stamp ? -1 : 1;
}

static int load_dictionary(config_desc *config) {
    if (config->dicts_count == 0)
        return 0;

    qsort(config->dicts, config->dicts_count, sizeof(config->dicts[0]), qsort_dictionary_buffer_cmp);

    size_t i, last_index = 0;
    dictionary_group_t group = dictionary_set_new_group(config->dictionary_set);

    for (i = 0; i < config->dicts_count; i++) {
        if (config->dicts[i].index > last_index) {
            last_index = config->dicts[i].index;
            group = dictionary_set_new_group(config->dictionary_set);
        }
        dictionary_group_load(group, config->dicts[i].file_name, config->home_dir, config->dicts[i].dict_type);
    }

    return 0;
}

static int parse_add_dict(config_desc *config, size_t index, const char *dictstr) {
    const char *pstr = dictstr;

    while (*pstr != '\0' && *pstr != ' ')
        pstr++;

    opencc_dictionary_type dict_type;

    if (strncmp(dictstr, CONFIG_DICT_TYPE_OCD, sizeof(CONFIG_DICT_TYPE_OCD) - 1) == 0)
        dict_type = OPENCC_DICTIONARY_TYPE_DATRIE;
    else if (strncmp(dictstr, CONFIG_DICT_TYPE_TEXT, sizeof(CONFIG_DICT_TYPE_OCD) - 1) == 0)
        dict_type = OPENCC_DICTIONARY_TYPE_TEXT;
    else {
        errnum = CONFIG_ERROR_INVALID_DICT_TYPE;
        return -1;
    }

    while (*pstr != '\0' && (*pstr == ' ' || *pstr == '\t'))
        pstr++;

    size_t i = config->dicts_count++;

    config->dicts[i].dict_type = dict_type;
    config->dicts[i].file_name = mstrcpy(pstr);
    config->dicts[i].index = index;
    config->dicts[i].stamp = config->stamp++;

    return 0;
}

static int parse_property(config_desc *config, const char *key, const char *value) {
    if (strncmp(key, "dict", 4) == 0) {
        int index = 0;
        sscanf(key + 4, "%d", &index);
        return parse_add_dict(config, index, value);
    } else if (strcmp(key, "title") == 0) {
        free(config->title);
        config->title = mstrcpy(value);
        return 0;
    } else if (strcmp(key, "description") == 0) {
        free(config->description);
        config->description = mstrcpy(value);
        return 0;
    }

    errnum = CONFIG_ERROR_NO_PROPERTY;
    return -1;
}

static int parse_line(const char *line, char **key, char **value) {
    const char *line_begin = line;

    while (*line != '\0' && (*line != ' ' && *line != '\t' && *line != '='))
        line++;

    size_t key_len = line - line_begin;

    while (*line != '\0' && *line != '=')
        line++;

    if (*line == '\0')
        return -1;

    assert(*line == '=');

    *key = mstrncpy(line_begin, key_len);

    line++;
    while (*line != '\0' && (*line == ' ' || *line == '\t'))
        line++;

    if (*line == '\0') {
        free(*key);
        return -1;
    }

    *value = mstrcpy(line);

    return 0;
}

static char *parse_trim(char *str) {
    for (; *str != '\0' && (*str == ' ' || *str == '\t'); str++)
        ;
    register char *prs = str;
    for (; *prs != '\0' && *prs != '\n' && *prs != '\r'; prs++)
        ;
    for (prs--; prs > str && (*prs == ' ' || *prs == '\t'); prs--)
        ;
    *(++prs) = '\0';
    return str;
}

static int parse(config_desc *config, const char *filename, const char *home_path) {
    FILE *fp = fopen(filename, "rb");
    if (!fp) {
        char *pkg_filename = (char *)malloc(sizeof(char) * (strlen(filename) + strlen(home_path) + 2));
        sprintf(pkg_filename, "%s/%s", home_path, filename);
        printf("pkg_filename %s\n", pkg_filename);
        fp = fopen(pkg_filename, "rb");
        if (!fp) {
            free(pkg_filename);
            errnum = CONFIG_ERROR_CANNOT_ACCESS_CONFIG_FILE;
            return -1;
        }
        free(pkg_filename);
    }

    config->home_dir = (char *)malloc(sizeof(char) * (strlen(home_path) + 1));
    sprintf(config->home_dir, "%s", home_path);

    static char buff[BUFFER_SIZE];

    while (fgets(buff, BUFFER_SIZE, fp) != NULL) {
        char *trimed_buff = parse_trim(buff);
        if (*trimed_buff == ';' || *trimed_buff == '#' || *trimed_buff == '\0') {
            /* Comment Line or empty line */
            continue;
        }

        char *key = NULL, *value = NULL;

        if (parse_line(trimed_buff, &key, &value) == -1) {
            free(key);
            free(value);
            fclose(fp);
            errnum = CONFIG_ERROR_PARSE;
            return -1;
        }

        if (parse_property(config, key, value) == -1) {
            free(key);
            free(value);
            fclose(fp);
            return -1;
        }

        free(key);
        free(value);
    }

    fclose(fp);
    return 0;
}

dictionary_set_t config_get_dictionary_set(config_t t_config) {
    config_desc *config = (config_desc *)t_config;

    if (config->dictionary_set != NULL) {
        dictionary_set_close(config->dictionary_set);
    }

    config->dictionary_set = dictionary_set_open();
    load_dictionary(config);

    return config->dictionary_set;
}

config_error config_errno(void) { return errnum; }

void config_perror(const char *spec) {
    perr(spec);
    perr("\n");
    switch (errnum) {
        case CONFIG_ERROR_VOID:
            break;
        case CONFIG_ERROR_CANNOT_ACCESS_CONFIG_FILE:
            perror(_("Can not access configuration file"));
            break;
        case CONFIG_ERROR_PARSE:
            perr(_("Configuration file parse error"));
            break;
        case CONFIG_ERROR_NO_PROPERTY:
            perr(_("Invalid property"));
            break;
        case CONFIG_ERROR_INVALID_DICT_TYPE:
            perr(_("Invalid dictionary type"));
            break;
        default:
            perr(_("Unknown"));
    }
}

config_t config_open(const char *filename, const char *home_path) {
    config_desc *config = (config_desc *)malloc(sizeof(config_desc));

    config->title = NULL;
    config->description = NULL;
    config->home_dir = NULL;
    config->dicts_count = 0;
    config->stamp = 0;
    config->dictionary_set = NULL;

    if (parse(config, filename, home_path) == -1) {
        config_close((config_t)config);
        return (config_t)-1;
    }

    return (config_t)config;
}

void config_close(config_t t_config) {
    config_desc *config = (config_desc *)t_config;

    size_t i;
    for (i = 0; i < config->dicts_count; i++)
        free(config->dicts[i].file_name);

    free(config->title);
    free(config->description);
    free(config->home_dir);
    free(config);
}
