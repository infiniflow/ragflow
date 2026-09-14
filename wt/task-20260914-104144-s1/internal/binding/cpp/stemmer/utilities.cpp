// Copyright(C) 2023 InfiniFlow, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#include "header.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define unless(C) if (!(C))

#define CREATE_SIZE 1

extern symbol *create_s(void) {
    symbol *p;
    void *mem = malloc(HEAD + (CREATE_SIZE + 1) * sizeof(symbol));
    if (mem == NULL)
        return NULL;
    p = (symbol *)(HEAD + (char *)mem);
    CAPACITY(p) = CREATE_SIZE;
    SET_SIZE(p, CREATE_SIZE);
    return p;
}

extern void lose_s(symbol *p) {
    if (p == NULL)
        return;
    free((char *)p - HEAD);
}

/*
   new_p = skip_utf8(p, c, lb, l, n); skips n characters forwards from p + c
   if n +ve, or n characters backwards from p + c - 1 if n -ve. new_p is the new
   position, or 0 on failure.

   -- used to implement hop and next in the utf8 case.
*/

extern int skip_utf8(const symbol *p, int c, int lb, int l, int n) {
    int b;
    if (n >= 0) {
        for (; n > 0; n--) {
            if (c >= l)
                return -1;
            b = p[c++];
            if (b >= 0xC0) { /* 1100 0000 */
                while (c < l) {
                    b = p[c];
                    if (b >= 0xC0 || b < 0x80)
                        break;
                    /* break unless b is 10------ */
                    c++;
                }
            }
        }
    } else {
        for (; n < 0; n++) {
            if (c <= lb)
                return -1;
            b = p[--c];
            if (b >= 0x80) { /* 1000 0000 */
                while (c > lb) {
                    b = p[c];
                    if (b >= 0xC0)
                        break; /* 1100 0000 */
                    c--;
                }
            }
        }
    }
    return c;
}

/* Code for character groupings: utf8 cases */

static int get_utf8(const symbol *p, int c, int l, int *slot) {
    int b0, b1;
    if (c >= l)
        return 0;
    b0 = p[c++];
    if (b0 < 0xC0 || c == l) { /* 1100 0000 */
        *slot = b0;
        return 1;
    }
    b1 = p[c++];
    if (b0 < 0xE0 || c == l) { /* 1110 0000 */
        *slot = (b0 & 0x1F) << 6 | (b1 & 0x3F);
        return 2;
    }
    *slot = (b0 & 0xF) << 12 | (b1 & 0x3F) << 6 | (p[c] & 0x3F);
    return 3;
}

static int get_b_utf8(const symbol *p, int c, int lb, int *slot) {
    int b0, b1;
    if (c <= lb)
        return 0;
    b0 = p[--c];
    if (b0 < 0x80 || c == lb) { /* 1000 0000 */
        *slot = b0;
        return 1;
    }
    b1 = p[--c];
    if (b1 >= 0xC0 || c == lb) { /* 1100 0000 */
        *slot = (b1 & 0x1F) << 6 | (b0 & 0x3F);
        return 2;
    }
    *slot = (p[c] & 0xF) << 12 | (b1 & 0x3F) << 6 | (b0 & 0x3F);
    return 3;
}

extern int in_grouping_U(struct SN_env *z, const unsigned char *s, int min, int max, int repeat) {
    do {
        int ch;
        int w = get_utf8(z->p, z->c, z->l, &ch);
        unless(w) return -1;
        if (ch > max || (ch -= min) < 0 || (s[ch >> 3] & (0X1 << (ch & 0X7))) == 0)
            return w;
        z->c += w;
    } while (repeat);
    return 0;
}

extern int in_grouping_b_U(struct SN_env *z, const unsigned char *s, int min, int max, int repeat) {
    do {
        int ch;
        int w = get_b_utf8(z->p, z->c, z->lb, &ch);
        unless(w) return -1;
        if (ch > max || (ch -= min) < 0 || (s[ch >> 3] & (0X1 << (ch & 0X7))) == 0)
            return w;
        z->c -= w;
    } while (repeat);
    return 0;
}

extern int out_grouping_U(struct SN_env *z, const unsigned char *s, int min, int max, int repeat) {
    do {
        int ch;
        int w = get_utf8(z->p, z->c, z->l, &ch);
        unless(w) return -1;
        unless(ch > max || (ch -= min) < 0 || (s[ch >> 3] & (0X1 << (ch & 0X7))) == 0) return w;
        z->c += w;
    } while (repeat);
    return 0;
}

extern int out_grouping_b_U(struct SN_env *z, const unsigned char *s, int min, int max, int repeat) {
    do {
        int ch;
        int w = get_b_utf8(z->p, z->c, z->lb, &ch);
        unless(w) return -1;
        unless(ch > max || (ch -= min) < 0 || (s[ch >> 3] & (0X1 << (ch & 0X7))) == 0) return w;
        z->c -= w;
    } while (repeat);
    return 0;
}

/* Code for character groupings: non-utf8 cases */

extern int in_grouping(struct SN_env *z, const unsigned char *s, int min, int max, int repeat) {
    do {
        int ch;
        if (z->c >= z->l)
            return -1;
        ch = z->p[z->c];
        if (ch > max || (ch -= min) < 0 || (s[ch >> 3] & (0X1 << (ch & 0X7))) == 0)
            return 1;
        z->c++;
    } while (repeat);
    return 0;
}

extern int in_grouping_b(struct SN_env *z, const unsigned char *s, int min, int max, int repeat) {
    do {
        int ch;
        if (z->c <= z->lb)
            return -1;
        ch = z->p[z->c - 1];
        if (ch > max || (ch -= min) < 0 || (s[ch >> 3] & (0X1 << (ch & 0X7))) == 0)
            return 1;
        z->c--;
    } while (repeat);
    return 0;
}

extern int out_grouping(struct SN_env *z, const unsigned char *s, int min, int max, int repeat) {
    do {
        int ch;
        if (z->c >= z->l)
            return -1;
        ch = z->p[z->c];
        unless(ch > max || (ch -= min) < 0 || (s[ch >> 3] & (0X1 << (ch & 0X7))) == 0) return 1;
        z->c++;
    } while (repeat);
    return 0;
}

extern int out_grouping_b(struct SN_env *z, const unsigned char *s, int min, int max, int repeat) {
    do {
        int ch;
        if (z->c <= z->lb)
            return -1;
        ch = z->p[z->c - 1];
        unless(ch > max || (ch -= min) < 0 || (s[ch >> 3] & (0X1 << (ch & 0X7))) == 0) return 1;
        z->c--;
    } while (repeat);
    return 0;
}

extern int eq_s(struct SN_env *z, int s_size, const symbol *s) {
    if (z->l - z->c < s_size || memcmp(z->p + z->c, s, s_size * sizeof(symbol)) != 0)
        return 0;
    z->c += s_size;
    return 1;
}

extern int eq_s_b(struct SN_env *z, int s_size, const symbol *s) {
    if (z->c - z->lb < s_size || memcmp(z->p + z->c - s_size, s, s_size * sizeof(symbol)) != 0)
        return 0;
    z->c -= s_size;
    return 1;
}

extern int eq_v(struct SN_env *z, const symbol *p) { return eq_s(z, SIZE(p), p); }

extern int eq_v_b(struct SN_env *z, const symbol *p) { return eq_s_b(z, SIZE(p), p); }

extern int find_among(struct SN_env *z, const struct among *v, int v_size) {

    int i = 0;
    int j = v_size;

    int c = z->c;
    int l = z->l;
    symbol *q = z->p + c;

    const struct among *w;

    int common_i = 0;
    int common_j = 0;

    int first_key_inspected = 0;

    while (1) {
        int k = i + ((j - i) >> 1);
        int diff = 0;
        int common = common_i < common_j ? common_i : common_j; /* smaller */
        w = v + k;
        {
            int i2;
            for (i2 = common; i2 < w->s_size; i2++) {
                if (c + common == l) {
                    diff = -1;
                    break;
                }
                diff = q[common] - w->s[i2];
                if (diff != 0)
                    break;
                common++;
            }
        }
        if (diff < 0) {
            j = k;
            common_j = common;
        } else {
            i = k;
            common_i = common;
        }
        if (j - i <= 1) {
            if (i > 0)
                break; /* v->s has been inspected */
            if (j == i)
                break; /* only one item in v */

            /* - but now we need to go round once more to get
               v->s inspected. This looks messy, but is actually
               the optimal approach.  */

            if (first_key_inspected)
                break;
            first_key_inspected = 1;
        }
    }
    while (1) {
        w = v + i;
        if (common_i >= w->s_size) {
            z->c = c + w->s_size;
            if (w->function == 0)
                return w->result;
            {
                int res = w->function(z);
                z->c = c + w->s_size;
                if (res)
                    return w->result;
            }
        }
        i = w->substring_i;
        if (i < 0)
            return 0;
    }
}

/* find_among_b is for backwards processing. Same comments apply */

extern int find_among_b(struct SN_env *z, const struct among *v, int v_size) {

    int i = 0;
    int j = v_size;

    int c = z->c;
    int lb = z->lb;
    symbol *q = z->p + c - 1;

    const struct among *w;

    int common_i = 0;
    int common_j = 0;

    int first_key_inspected = 0;

    while (1) {
        int k = i + ((j - i) >> 1);
        int diff = 0;
        int common = common_i < common_j ? common_i : common_j;
        w = v + k;
        {
            int i2;
            for (i2 = w->s_size - 1 - common; i2 >= 0; i2--) {
                if (c - common == lb) {
                    diff = -1;
                    break;
                }
                diff = q[-common] - w->s[i2];
                if (diff != 0)
                    break;
                common++;
            }
        }
        if (diff < 0) {
            j = k;
            common_j = common;
        } else {
            i = k;
            common_i = common;
        }
        if (j - i <= 1) {
            if (i > 0)
                break;
            if (j == i)
                break;
            if (first_key_inspected)
                break;
            first_key_inspected = 1;
        }
    }
    while (1) {
        w = v + i;
        if (common_i >= w->s_size) {
            z->c = c - w->s_size;
            if (w->function == 0)
                return w->result;
            {
                int res = w->function(z);
                z->c = c - w->s_size;
                if (res)
                    return w->result;
            }
        }
        i = w->substring_i;
        if (i < 0)
            return 0;
    }
}

/* Increase the size of the buffer pointed to by p to at least n symbols.
 * If insufficient memory, returns NULL and frees the old buffer.
 */
static symbol *increase_size(symbol *p, int n) {
    symbol *q;
    int new_size = n + 20;
    void *mem = realloc((char *)p - HEAD, HEAD + (new_size + 1) * sizeof(symbol));
    if (mem == NULL) {
        lose_s(p);
        return NULL;
    }
    q = (symbol *)(HEAD + (char *)mem);
    CAPACITY(q) = new_size;
    return q;
}

/* to replace symbols between c_bra and c_ket in z->p by the
   s_size symbols at s.
   Returns 0 on success, -1 on error.
   Also, frees z->p (and sets it to NULL) on error.
*/
extern int replace_s(struct SN_env *z, int c_bra, int c_ket, int s_size, const symbol *s, int *adjptr) {
    int adjustment;
    int len;
    if (z->p == NULL) {
        z->p = create_s();
        if (z->p == NULL)
            return -1;
    }
    adjustment = s_size - (c_ket - c_bra);
    len = SIZE(z->p);
    if (adjustment != 0) {
        if (adjustment + len > CAPACITY(z->p)) {
            z->p = increase_size(z->p, adjustment + len);
            if (z->p == NULL)
                return -1;
        }
        memmove(z->p + c_ket + adjustment, z->p + c_ket, (len - c_ket) * sizeof(symbol));
        SET_SIZE(z->p, adjustment + len);
        z->l += adjustment;
        if (z->c >= c_ket)
            z->c += adjustment;
        else if (z->c > c_bra)
            z->c = c_bra;
    }
    unless(s_size == 0) memmove(z->p + c_bra, s, s_size * sizeof(symbol));
    if (adjptr != NULL)
        *adjptr = adjustment;
    return 0;
}

static int slice_check(struct SN_env *z) {

    if (z->bra < 0 || z->bra > z->ket || z->ket > z->l || z->p == NULL || z->l > SIZE(z->p)) /* this line could be removed */
    {
#if 0
        fprintf(stderr, "faulty slice operation:\n");
        debug(z, -1, 0);
#endif
        return -1;
    }
    return 0;
}

extern int slice_from_s(struct SN_env *z, int s_size, const symbol *s) {
    if (slice_check(z))
        return -1;
    return replace_s(z, z->bra, z->ket, s_size, s, NULL);
}

extern int slice_from_v(struct SN_env *z, const symbol *p) { return slice_from_s(z, SIZE(p), p); }

extern int slice_del(struct SN_env *z) { return slice_from_s(z, 0, 0); }

extern int insert_s(struct SN_env *z, int bra, int ket, int s_size, const symbol *s) {
    int adjustment;
    if (replace_s(z, bra, ket, s_size, s, &adjustment))
        return -1;
    if (bra <= z->bra)
        z->bra += adjustment;
    if (bra <= z->ket)
        z->ket += adjustment;
    return 0;
}

extern int insert_v(struct SN_env *z, int bra, int ket, const symbol *p) {
    int adjustment;
    if (replace_s(z, bra, ket, SIZE(p), p, &adjustment))
        return -1;
    if (bra <= z->bra)
        z->bra += adjustment;
    if (bra <= z->ket)
        z->ket += adjustment;
    return 0;
}

extern symbol *slice_to(struct SN_env *z, symbol *p) {
    if (slice_check(z)) {
        lose_s(p);
        return NULL;
    }
    {
        int len = z->ket - z->bra;
        if (CAPACITY(p) < len) {
            p = increase_size(p, len);
            if (p == NULL)
                return NULL;
        }
        memmove(p, z->p + z->bra, len * sizeof(symbol));
        SET_SIZE(p, len);
    }
    return p;
}

extern symbol *assign_to(struct SN_env *z, symbol *p) {
    int len = z->l;
    if (CAPACITY(p) < len) {
        p = increase_size(p, len);
        if (p == NULL)
            return NULL;
    }
    memmove(p, z->p, len * sizeof(symbol));
    SET_SIZE(p, len);
    return p;
}
