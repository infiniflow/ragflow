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

#include "opencc.h"
#include "config_reader.h"
#include "converter.h"
#include "dictionary_set.h"
#include "encoding.h"
#include "utils.h"

typedef struct {
    dictionary_set_t dictionary_set;
    converter_t converter;
} opencc_desc;

static opencc_error errnum = OPENCC_ERROR_VOID;
static int lib_initialized = FALSE;

static void lib_initialize(void) { lib_initialized = TRUE; }

size_t opencc_convert(opencc_t t_opencc, ucs4_t **inbuf, size_t *inbuf_left, ucs4_t **outbuf, size_t *outbuf_left) {
    if (!lib_initialized)
        lib_initialize();

    opencc_desc *opencc = (opencc_desc *)t_opencc;

    size_t retval = converter_convert(opencc->converter, inbuf, inbuf_left, outbuf, outbuf_left);

    if (retval == (size_t)-1)
        errnum = OPENCC_ERROR_CONVERTER;

    return retval;
}

char *opencc_convert_utf8(opencc_t t_opencc, const char *inbuf, size_t length) {
    if (!lib_initialized)
        lib_initialize();

    if (length == (size_t)-1 || length > strlen(inbuf))
        length = strlen(inbuf);

    /* 將輸入數據轉換爲ucs4_t字符串 */
    ucs4_t *winbuf = utf8_to_ucs4(inbuf, length);
    if (winbuf == (ucs4_t *)-1) {
        /* 輸入數據轉換失敗 */
        errnum = OPENCC_ERROR_ENCODIND;
        return (char *)-1;
    }

    /* 設置輸出UTF8文本緩衝區空間 */
    size_t outbuf_len = length;
    size_t outsize = outbuf_len;
    char *original_outbuf = (char *)malloc(sizeof(char) * (outbuf_len + 1));
    char *outbuf = original_outbuf;
    original_outbuf[0] = '\0';

    /* 設置轉換緩衝區空間 */
    size_t wbufsize = length + 64;
    ucs4_t *woutbuf = (ucs4_t *)malloc(sizeof(ucs4_t) * (wbufsize + 1));

    ucs4_t *pinbuf = winbuf;
    ucs4_t *poutbuf = woutbuf;
    size_t inbuf_left, outbuf_left;

    inbuf_left = ucs4len(winbuf);
    outbuf_left = wbufsize;

    while (inbuf_left > 0) {
        size_t retval = opencc_convert(t_opencc, &pinbuf, &inbuf_left, &poutbuf, &outbuf_left);
        if (retval == (size_t)-1) {
            free(outbuf);
            free(winbuf);
            free(woutbuf);
            return (char *)-1;
        }

        *poutbuf = L'\0';

        char *ubuff = ucs4_to_utf8(woutbuf, (size_t)-1);

        if (ubuff == (char *)-1) {
            free(outbuf);
            free(winbuf);
            free(woutbuf);
            errnum = OPENCC_ERROR_ENCODIND;
            return (char *)-1;
        }

        size_t ubuff_len = strlen(ubuff);

        while (ubuff_len > outsize) {
            size_t outbuf_offset = outbuf - original_outbuf;
            outsize += outbuf_len;
            outbuf_len += outbuf_len;
            original_outbuf = (char *)realloc(original_outbuf, sizeof(char) * outbuf_len);
            outbuf = original_outbuf + outbuf_offset;
        }

        strncpy(outbuf, ubuff, ubuff_len);
        free(ubuff);

        outbuf += ubuff_len;
        *outbuf = '\0';

        outbuf_left = wbufsize;
        poutbuf = woutbuf;
    }

    free(winbuf);
    free(woutbuf);

    original_outbuf = (char *)realloc(original_outbuf, sizeof(char) * (strlen(original_outbuf) + 1));

    return original_outbuf;
}

opencc_t opencc_open(const char *config_file, const char *home_path) {
    if (!lib_initialized)
        lib_initialize();

    opencc_desc *opencc;
    opencc = (opencc_desc *)malloc(sizeof(opencc_desc));

    opencc->dictionary_set = NULL;
    opencc->converter = converter_open();
    converter_set_conversion_mode(opencc->converter, OPENCC_CONVERSION_FAST);

    /* 加載默認辭典 */
    int retval;
    if (config_file == NULL)
        retval = 0;
    else {
        config_t config = config_open(config_file, home_path);

        if (config == (config_t)-1) {
            errnum = OPENCC_ERROR_CONFIG;
            return (opencc_t)-1;
        }

        opencc->dictionary_set = config_get_dictionary_set(config);
        converter_assign_dictionary(opencc->converter, opencc->dictionary_set);

        config_close(config);
    }

    return (opencc_t)opencc;
}

int opencc_close(opencc_t t_opencc) {
    if (!lib_initialized)
        lib_initialize();

    opencc_desc *opencc = (opencc_desc *)t_opencc;

    converter_close(opencc->converter);
    if (opencc->dictionary_set != NULL)
        dictionary_set_close(opencc->dictionary_set);
    free(opencc);

    return 0;
}

void opencc_set_conversion_mode(opencc_t t_opencc, opencc_conversion_mode conversion_mode) {
    if (!lib_initialized)
        lib_initialize();

    opencc_desc *opencc = (opencc_desc *)t_opencc;

    converter_set_conversion_mode(opencc->converter, conversion_mode);
}

opencc_error opencc_errno(void) {
    if (!lib_initialized)
        lib_initialize();

    return errnum;
}

void opencc_perror(const char *spec) {
    if (!lib_initialized)
        lib_initialize();

    perr(spec);
    perr("\n");
    switch (errnum) {
        case OPENCC_ERROR_VOID:
            break;
        case OPENCC_ERROR_DICTLOAD:
            dictionary_perror(_("Dictionary loading error"));
            break;
        case OPENCC_ERROR_CONFIG:
            config_perror(_("Configuration error"));
            break;
        case OPENCC_ERROR_CONVERTER:
            converter_perror(_("Converter error"));
            break;
        case OPENCC_ERROR_ENCODIND:
            perr(_("Encoding error"));
            break;
        default:
            perr(_("Unknown"));
    }
    perr("\n");
}
