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

#include "utils.h"

void perr(const char *str) { fputs(str, stderr); }

int qsort_int_cmp(const void *a, const void *b) { return *((int *)a) - *((int *)b); }

char *mstrcpy(const char *str) {
    char *strbuf = (char *)malloc(sizeof(char) * (strlen(str) + 1));
    strcpy(strbuf, str);
    return strbuf;
}

char *mstrncpy(const char *str, size_t n) {
    char *strbuf = (char *)malloc(sizeof(char) * (n + 1));
    strncpy(strbuf, str, n);
    strbuf[n] = '\0';
    return strbuf;
}
