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

#ifndef __OPENCC_UTILS_H_
#define __OPENCC_UTILS_H_

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "opencc_types.h"

#ifdef __cplusplus
extern "C" {
#endif

#define FALSE (0)
#define TRUE (!(0))
#define INFINITY_INT ((~0U) >> 1)

#ifndef BIG_ENDIAN
#define BIG_ENDIAN (0)
#endif

#ifndef LITTLE_ENDIAN
#define LITTLE_ENDIAN (1)
#endif

#ifdef ENABLE_GETTEXT
#include <libintl.h>
#include <locale.h>
#define _(STRING) dgettext(PACKAGE_NAME, STRING)
#else
#define _(STRING) STRING
#endif

#define debug_should_not_be_here()                                                                                                                   \
    do {                                                                                                                                             \
        fprintf(stderr, "Should not be here %s: %d\n", __FILE__, __LINE__);                                                                          \
        assert(0);                                                                                                                                   \
    } while (0)

void perr(const char *str);

int qsort_int_cmp(const void *a, const void *b);

char *mstrcpy(const char *str);

char *mstrncpy(const char *str, size_t n);

#ifdef __cplusplus
};
#endif

#endif /* __OPENCC_UTILS_H_ */
