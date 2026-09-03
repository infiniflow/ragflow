
/* This file was generated automatically by the Snowball to ANSI C compiler */

#include "header.h"

#ifdef __cplusplus
extern "C" {
#endif
extern int norwegian_UTF_8_stem(struct SN_env *z);
#ifdef __cplusplus
}
#endif
static int r_other_suffix(struct SN_env *z);
static int r_consonant_pair(struct SN_env *z);
static int r_main_suffix(struct SN_env *z);
static int r_mark_regions(struct SN_env *z);
#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *norwegian_UTF_8_create_env(void);
extern void norwegian_UTF_8_close_env(struct SN_env *z);

#ifdef __cplusplus
}
#endif
static const symbol s_0_0[1] = {'a'};
static const symbol s_0_1[1] = {'e'};
static const symbol s_0_2[3] = {'e', 'd', 'e'};
static const symbol s_0_3[4] = {'a', 'n', 'd', 'e'};
static const symbol s_0_4[4] = {'e', 'n', 'd', 'e'};
static const symbol s_0_5[3] = {'a', 'n', 'e'};
static const symbol s_0_6[3] = {'e', 'n', 'e'};
static const symbol s_0_7[6] = {'h', 'e', 't', 'e', 'n', 'e'};
static const symbol s_0_8[4] = {'e', 'r', 't', 'e'};
static const symbol s_0_9[2] = {'e', 'n'};
static const symbol s_0_10[5] = {'h', 'e', 't', 'e', 'n'};
static const symbol s_0_11[2] = {'a', 'r'};
static const symbol s_0_12[2] = {'e', 'r'};
static const symbol s_0_13[5] = {'h', 'e', 't', 'e', 'r'};
static const symbol s_0_14[1] = {'s'};
static const symbol s_0_15[2] = {'a', 's'};
static const symbol s_0_16[2] = {'e', 's'};
static const symbol s_0_17[4] = {'e', 'd', 'e', 's'};
static const symbol s_0_18[5] = {'e', 'n', 'd', 'e', 's'};
static const symbol s_0_19[4] = {'e', 'n', 'e', 's'};
static const symbol s_0_20[7] = {'h', 'e', 't', 'e', 'n', 'e', 's'};
static const symbol s_0_21[3] = {'e', 'n', 's'};
static const symbol s_0_22[6] = {'h', 'e', 't', 'e', 'n', 's'};
static const symbol s_0_23[3] = {'e', 'r', 's'};
static const symbol s_0_24[3] = {'e', 't', 's'};
static const symbol s_0_25[2] = {'e', 't'};
static const symbol s_0_26[3] = {'h', 'e', 't'};
static const symbol s_0_27[3] = {'e', 'r', 't'};
static const symbol s_0_28[3] = {'a', 's', 't'};

static const struct among a_0[29] = {
    /*  0 */ {1, s_0_0, -1, 1, 0},
    /*  1 */ {1, s_0_1, -1, 1, 0},
    /*  2 */ {3, s_0_2, 1, 1, 0},
    /*  3 */ {4, s_0_3, 1, 1, 0},
    /*  4 */ {4, s_0_4, 1, 1, 0},
    /*  5 */ {3, s_0_5, 1, 1, 0},
    /*  6 */ {3, s_0_6, 1, 1, 0},
    /*  7 */ {6, s_0_7, 6, 1, 0},
    /*  8 */ {4, s_0_8, 1, 3, 0},
    /*  9 */ {2, s_0_9, -1, 1, 0},
    /* 10 */ {5, s_0_10, 9, 1, 0},
    /* 11 */ {2, s_0_11, -1, 1, 0},
    /* 12 */ {2, s_0_12, -1, 1, 0},
    /* 13 */ {5, s_0_13, 12, 1, 0},
    /* 14 */ {1, s_0_14, -1, 2, 0},
    /* 15 */ {2, s_0_15, 14, 1, 0},
    /* 16 */ {2, s_0_16, 14, 1, 0},
    /* 17 */ {4, s_0_17, 16, 1, 0},
    /* 18 */ {5, s_0_18, 16, 1, 0},
    /* 19 */ {4, s_0_19, 16, 1, 0},
    /* 20 */ {7, s_0_20, 19, 1, 0},
    /* 21 */ {3, s_0_21, 14, 1, 0},
    /* 22 */ {6, s_0_22, 21, 1, 0},
    /* 23 */ {3, s_0_23, 14, 1, 0},
    /* 24 */ {3, s_0_24, 14, 1, 0},
    /* 25 */ {2, s_0_25, -1, 1, 0},
    /* 26 */ {3, s_0_26, 25, 1, 0},
    /* 27 */ {3, s_0_27, -1, 3, 0},
    /* 28 */ {3, s_0_28, -1, 1, 0}};

static const symbol s_1_0[2] = {'d', 't'};
static const symbol s_1_1[2] = {'v', 't'};

static const struct among a_1[2] = {
    /*  0 */ {2, s_1_0, -1, -1, 0},
    /*  1 */ {2, s_1_1, -1, -1, 0}};

static const symbol s_2_0[3] = {'l', 'e', 'g'};
static const symbol s_2_1[4] = {'e', 'l', 'e', 'g'};
static const symbol s_2_2[2] = {'i', 'g'};
static const symbol s_2_3[3] = {'e', 'i', 'g'};
static const symbol s_2_4[3] = {'l', 'i', 'g'};
static const symbol s_2_5[4] = {'e', 'l', 'i', 'g'};
static const symbol s_2_6[3] = {'e', 'l', 's'};
static const symbol s_2_7[3] = {'l', 'o', 'v'};
static const symbol s_2_8[4] = {'e', 'l', 'o', 'v'};
static const symbol s_2_9[4] = {'s', 'l', 'o', 'v'};
static const symbol s_2_10[7] = {'h', 'e', 't', 's', 'l', 'o', 'v'};

static const struct among a_2[11] = {
    /*  0 */ {3, s_2_0, -1, 1, 0},
    /*  1 */ {4, s_2_1, 0, 1, 0},
    /*  2 */ {2, s_2_2, -1, 1, 0},
    /*  3 */ {3, s_2_3, 2, 1, 0},
    /*  4 */ {3, s_2_4, 2, 1, 0},
    /*  5 */ {4, s_2_5, 4, 1, 0},
    /*  6 */ {3, s_2_6, -1, 1, 0},
    /*  7 */ {3, s_2_7, -1, 1, 0},
    /*  8 */ {4, s_2_8, 7, 1, 0},
    /*  9 */ {4, s_2_9, 7, 1, 0},
    /* 10 */ {7, s_2_10, 9, 1, 0}};

static const unsigned char g_v[] = {17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 48, 0, 128};

static const unsigned char g_s_ending[] = {119, 125, 149, 1};

static const symbol s_0[] = {'k'};
static const symbol s_1[] = {'e', 'r'};

static int r_mark_regions(struct SN_env *z) {
    z->I[0] = z->l;
    {
        int c_test = z->c; /* test, line 30 */
        {
            int ret = skip_utf8(z->p, z->c, 0, z->l, +3);
            if (ret < 0)
                return 0;
            z->c = ret; /* hop, line 30 */
        }
        z->I[1] = z->c; /* setmark x, line 30 */
        z->c = c_test;
    }
    if (out_grouping_U(z, g_v, 97, 248, 1) < 0)
        return 0; /* goto */ /* grouping v, line 31 */
    { /* gopast */           /* non v, line 31 */
        int ret = in_grouping_U(z, g_v, 97, 248, 1);
        if (ret < 0)
            return 0;
        z->c += ret;
    }
    z->I[0] = z->c; /* setmark p1, line 31 */
                    /* try, line 32 */
    if (!(z->I[0] < z->I[1]))
        goto lab0;
    z->I[0] = z->I[1];
lab0:
    return 1;
}

static int r_main_suffix(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 38 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 38 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 38 */
        if (z->c <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((1851426 >> (z->p[z->c - 1] & 0x1f)) & 1)) {
            z->lb = mlimit;
            return 0;
        }
        among_var = find_among_b(z, a_0, 29); /* substring, line 38 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 38 */
        z->lb = mlimit;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 44 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int m2 = z->l - z->c;
            (void)m2; /* or, line 46 */
            if (in_grouping_b_U(z, g_s_ending, 98, 122, 0))
                goto lab1;
            goto lab0;
        lab1:
            z->c = z->l - m2;
            if (!(eq_s_b(z, 1, s_0)))
                return 0;
            if (out_grouping_b_U(z, g_v, 97, 248, 0))
                return 0;
        }
        lab0: {
            int ret = slice_del(z); /* delete, line 46 */
            if (ret < 0)
                return ret;
        } break;
        case 3: {
            int ret = slice_from_s(z, 2, s_1); /* <-, line 48 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_consonant_pair(struct SN_env *z) {
    {
        int m_test = z->l - z->c; /* test, line 53 */
        {
            int mlimit; /* setlimit, line 54 */
            int m1 = z->l - z->c;
            (void)m1;
            if (z->c < z->I[0])
                return 0;
            z->c = z->I[0]; /* tomark, line 54 */
            mlimit = z->lb;
            z->lb = z->c;
            z->c = z->l - m1;
            z->ket = z->c; /* [, line 54 */
            if (z->c - 1 <= z->lb || z->p[z->c - 1] != 116) {
                z->lb = mlimit;
                return 0;
            }
            if (!(find_among_b(z, a_1, 2))) {
                z->lb = mlimit;
                return 0;
            } /* substring, line 54 */
            z->bra = z->c; /* ], line 54 */
            z->lb = mlimit;
        }
        z->c = z->l - m_test;
    }
    {
        int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
        if (ret < 0)
            return 0;
        z->c = ret; /* next, line 59 */
    }
    z->bra = z->c; /* ], line 59 */
    {
        int ret = slice_del(z); /* delete, line 59 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_other_suffix(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 63 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 63 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 63 */
        if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((4718720 >> (z->p[z->c - 1] & 0x1f)) & 1)) {
            z->lb = mlimit;
            return 0;
        }
        among_var = find_among_b(z, a_2, 11); /* substring, line 63 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 63 */
        z->lb = mlimit;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 67 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

extern int norwegian_UTF_8_stem(struct SN_env *z) {
    {
        int c1 = z->c; /* do, line 74 */
        {
            int ret = r_mark_regions(z);
            if (ret == 0)
                goto lab0; /* call mark_regions, line 74 */
            if (ret < 0)
                return ret;
        }
    lab0:
        z->c = c1;
    }
    z->lb = z->c;
    z->c = z->l; /* backwards, line 75 */

    {
        int m2 = z->l - z->c;
        (void)m2; /* do, line 76 */
        {
            int ret = r_main_suffix(z);
            if (ret == 0)
                goto lab1; /* call main_suffix, line 76 */
            if (ret < 0)
                return ret;
        }
    lab1:
        z->c = z->l - m2;
    }
    {
        int m3 = z->l - z->c;
        (void)m3; /* do, line 77 */
        {
            int ret = r_consonant_pair(z);
            if (ret == 0)
                goto lab2; /* call consonant_pair, line 77 */
            if (ret < 0)
                return ret;
        }
    lab2:
        z->c = z->l - m3;
    }
    {
        int m4 = z->l - z->c;
        (void)m4; /* do, line 78 */
        {
            int ret = r_other_suffix(z);
            if (ret == 0)
                goto lab3; /* call other_suffix, line 78 */
            if (ret < 0)
                return ret;
        }
    lab3:
        z->c = z->l - m4;
    }
    z->c = z->lb;
    return 1;
}

extern struct SN_env *norwegian_UTF_8_create_env(void) { return SN_create_env(0, 2, 0); }

extern void norwegian_UTF_8_close_env(struct SN_env *z) { SN_close_env(z, 0); }
