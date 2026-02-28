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

#include "encoding.h"
#include "opencc.h"

#define INITIAL_BUFF_SIZE 1024
#define GET_BIT(byte, pos) (((byte) >> (pos)) & 1)
#define BITMASK(length) ((1 << length) - 1)

ucs4_t *utf8_to_ucs4(const char *utf8, size_t length) {
    if (length == 0)
        length = (size_t)-1;
    size_t i;
    for (i = 0; i < length && utf8[i] != '\0'; i++)
        ;
    length = i;

    size_t freesize = INITIAL_BUFF_SIZE;
    ucs4_t *ucs4 = (ucs4_t *)malloc(sizeof(ucs4_t) * freesize);
    ucs4_t *pucs4 = ucs4;

    for (i = 0; i < length; i++) {
        ucs4_t byte[4] = {0};
        if (GET_BIT(utf8[i], 7) == 0) {
            /* U-00000000 - U-0000007F */
            /* 0xxxxxxx */
            byte[0] = utf8[i] & BITMASK(7);
        } else if (GET_BIT(utf8[i], 5) == 0) {
            /* U-00000080 - U-000007FF */
            /* 110xxxxx 10xxxxxx */
            if (i + 1 >= length)
                goto err;

            byte[0] = (utf8[i + 1] & BITMASK(6)) + ((utf8[i] & BITMASK(2)) << 6);
            byte[1] = (utf8[i] >> 2) & BITMASK(3);

            i += 1;
        } else if (GET_BIT(utf8[i], 4) == 0) {
            /* U-00000800 - U-0000FFFF */
            /* 1110xxxx 10xxxxxx 10xxxxxx */
            if (i + 2 >= length)
                goto err;

            byte[0] = (utf8[i + 2] & BITMASK(6)) + ((utf8[i + 1] & BITMASK(2)) << 6);
            byte[1] = ((utf8[i + 1] >> 2) & BITMASK(4)) + ((utf8[i] & BITMASK(4)) << 4);

            i += 2;
        } else if (GET_BIT(utf8[i], 3) == 0) {
            /* U-00010000 - U-001FFFFF */
            /* 11110xxx 10xxxxxx 10xxxxxx 10xxxxxx */
            if (i + 3 >= length)
                goto err;

            byte[0] = (utf8[i + 3] & BITMASK(6)) + ((utf8[i + 2] & BITMASK(2)) << 6);
            byte[1] = ((utf8[i + 2] >> 2) & BITMASK(4)) + ((utf8[i + 1] & BITMASK(4)) << 4);
            byte[2] = ((utf8[i + 1] >> 4) & BITMASK(2)) + ((utf8[i] & BITMASK(3)) << 2);

            i += 3;
        } else if (GET_BIT(utf8[i], 2) == 0) {
            /* U-00200000 - U-03FFFFFF */
            /* 111110xx 10xxxxxx 10xxxxxx 10xxxxxx 10xxxxxx */
            if (i + 4 >= length)
                goto err;

            byte[0] = (utf8[i + 4] & BITMASK(6)) + ((utf8[i + 3] & BITMASK(2)) << 6);
            byte[1] = ((utf8[i + 3] >> 2) & BITMASK(4)) + ((utf8[i + 2] & BITMASK(4)) << 4);
            byte[2] = ((utf8[i + 2] >> 4) & BITMASK(2)) + ((utf8[i + 1] & BITMASK(6)) << 2);
            byte[3] = utf8[i] & BITMASK(2);
            i += 4;
        } else if (GET_BIT(utf8[i], 2) == 0) {
            /* U-04000000 - U-7FFFFFFF */
            /* 1111110x 10xxxxxx 10xxxxxx 10xxxxxx 10xxxxxx 10xxxxxx */
            if (i + 5 >= length)
                goto err;

            byte[0] = (utf8[i + 5] & BITMASK(6)) + ((utf8[i + 4] & BITMASK(2)) << 6);
            byte[1] = ((utf8[i + 4] >> 2) & BITMASK(4)) + ((utf8[i + 3] & BITMASK(4)) << 4);
            byte[2] = ((utf8[i + 3] >> 4) & BITMASK(2)) + ((utf8[i + 2] & BITMASK(6)) << 2);
            byte[3] = (utf8[i + 1] & BITMASK(6)) + ((utf8[i] & BITMASK(1)) << 6);
            i += 5;
        } else
            goto err;

        if (freesize == 0) {
            freesize = pucs4 - ucs4;
            ucs4 = (ucs4_t *)realloc(ucs4, sizeof(ucs4_t) * (freesize + freesize));
            pucs4 = ucs4 + freesize;
        }

        *pucs4 = (byte[3] << 24) + (byte[2] << 16) + (byte[1] << 8) + byte[0];

        pucs4++;
        freesize--;
    }

    length = (pucs4 - ucs4 + 1);
    ucs4 = (ucs4_t *)realloc(ucs4, sizeof(ucs4_t) * length);
    ucs4[length - 1] = 0;
    return ucs4;

err:
    free(ucs4);
    return (ucs4_t *)-1;
}

char *ucs4_to_utf8(const ucs4_t *ucs4, size_t length) {
    if (length == 0)
        length = (size_t)-1;
    size_t i;
    for (i = 0; i < length && ucs4[i] != 0; i++)
        ;
    length = i;

    size_t freesize = INITIAL_BUFF_SIZE;
    char *utf8 = (char *)malloc(sizeof(char) * freesize);
    char *putf8 = utf8;

    for (i = 0; i < length; i++) {
        if ((ssize_t)freesize - 6 <= 0) {
            freesize = putf8 - utf8;
            utf8 = (char *)realloc(utf8, sizeof(char) * (freesize + freesize));
            putf8 = utf8 + freesize;
        }

        ucs4_t c = ucs4[i];
        ucs4_t byte[4] = {(c >> 0) & BITMASK(8), (c >> 8) & BITMASK(8), (c >> 16) & BITMASK(8), (c >> 24) & BITMASK(8)};

        size_t delta = 0;

        if (c <= 0x7F) {
            /* U-00000000 - U-0000007F */
            /* 0xxxxxxx */
            putf8[0] = byte[0] & BITMASK(7);
            delta = 1;
        } else if (c <= 0x7FF) {
            /* U-00000080 - U-000007FF */
            /* 110xxxxx 10xxxxxx */
            putf8[1] = 0x80 + (byte[0] & BITMASK(6));
            putf8[0] = 0xC0 + ((byte[0] >> 6) & BITMASK(2)) + ((byte[1] & BITMASK(3)) << 2);
            delta = 2;
        } else if (c <= 0xFFFF) {
            /* U-00000800 - U-0000FFFF */
            /* 1110xxxx 10xxxxxx 10xxxxxx */
            putf8[2] = 0x80 + (byte[0] & BITMASK(6));
            putf8[1] = 0x80 + ((byte[0] >> 6) & BITMASK(2)) + ((byte[1] & BITMASK(4)) << 2);
            putf8[0] = 0xE0 + ((byte[1] >> 4) & BITMASK(4));
            delta = 3;
        } else if (c <= 0x1FFFFF) {
            /* U-00010000 - U-001FFFFF */
            /* 11110xxx 10xxxxxx 10xxxxxx 10xxxxxx */
            putf8[3] = 0x80 + (byte[0] & BITMASK(6));
            putf8[2] = 0x80 + ((byte[0] >> 6) & BITMASK(2)) + ((byte[1] & BITMASK(4)) << 2);
            putf8[1] = 0x80 + ((byte[1] >> 4) & BITMASK(4)) + ((byte[2] & BITMASK(2)) << 4);
            putf8[0] = 0xF0 + ((byte[2] >> 2) & BITMASK(3));
            delta = 4;
        } else if (c <= 0x3FFFFFF) {
            /* U-00200000 - U-03FFFFFF */
            /* 111110xx 10xxxxxx 10xxxxxx 10xxxxxx 10xxxxxx */
            putf8[4] = 0x80 + (byte[0] & BITMASK(6));
            putf8[3] = 0x80 + ((byte[0] >> 6) & BITMASK(2)) + ((byte[1] & BITMASK(4)) << 2);
            putf8[2] = 0x80 + ((byte[1] >> 4) & BITMASK(4)) + ((byte[2] & BITMASK(2)) << 4);
            putf8[1] = 0x80 + ((byte[2] >> 2) & BITMASK(6));
            putf8[0] = 0xF8 + (byte[3] & BITMASK(2));
            delta = 5;

        } else if (c <= 0x7FFFFFFF) {
            /* U-04000000 - U-7FFFFFFF */
            /* 1111110x 10xxxxxx 10xxxxxx 10xxxxxx 10xxxxxx 10xxxxxx */
            putf8[5] = 0x80 + (byte[0] & BITMASK(6));
            putf8[4] = 0x80 + ((byte[0] >> 6) & BITMASK(2)) + ((byte[1] & BITMASK(4)) << 2);
            putf8[3] = 0x80 + ((byte[1] >> 4) & BITMASK(4)) + ((byte[2] & BITMASK(2)) << 4);
            putf8[2] = 0x80 + ((byte[2] >> 2) & BITMASK(6));
            putf8[1] = 0x80 + (byte[3] & BITMASK(6));
            putf8[0] = 0xFC + ((byte[3] >> 6) & BITMASK(1));
            delta = 6;
        } else {
            free(utf8);
            return (char *)-1;
        }

        putf8 += delta;
        freesize -= delta;
    }

    length = (putf8 - utf8 + 1);
    utf8 = (char *)realloc(utf8, sizeof(char) * length);
    utf8[length - 1] = '\0';
    return utf8;
}

size_t ucs4len(const ucs4_t *str) {
    const register ucs4_t *pstr = str;
    while (*pstr)
        ++pstr;
    return pstr - str;
}

int ucs4cmp(const ucs4_t *src, const ucs4_t *dst) {
    register int ret = 0;
    while (!(ret = *src - *dst) && *dst)
        ++src, ++dst;
    return ret;
}

void ucs4cpy(ucs4_t *dest, const ucs4_t *src) {
    while (*src)
        *dest++ = *src++;
    *dest = 0;
}

void ucs4ncpy(ucs4_t *dest, const ucs4_t *src, size_t len) {
    while (*src && len-- > 0)
        *dest++ = *src++;
}
