
/* This file was generated automatically by the Snowball to ANSI C compiler */

#include "header.h"

#ifdef __cplusplus
extern "C" {
#endif
extern int finnish_UTF_8_stem(struct SN_env *z);
#ifdef __cplusplus
}
#endif
static int r_tidy(struct SN_env *z);
static int r_other_endings(struct SN_env *z);
static int r_t_plural(struct SN_env *z);
static int r_i_plural(struct SN_env *z);
static int r_case_ending(struct SN_env *z);
static int r_VI(struct SN_env *z);
static int r_LONG(struct SN_env *z);
static int r_possessive(struct SN_env *z);
static int r_particle_etc(struct SN_env *z);
static int r_R2(struct SN_env *z);
static int r_mark_regions(struct SN_env *z);
#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *finnish_UTF_8_create_env(void);
extern void finnish_UTF_8_close_env(struct SN_env *z);

#ifdef __cplusplus
}
#endif
static const symbol s_0_0[2] = {'p', 'a'};
static const symbol s_0_1[3] = {'s', 't', 'i'};
static const symbol s_0_2[4] = {'k', 'a', 'a', 'n'};
static const symbol s_0_3[3] = {'h', 'a', 'n'};
static const symbol s_0_4[3] = {'k', 'i', 'n'};
static const symbol s_0_5[4] = {'h', 0xC3, 0xA4, 'n'};
static const symbol s_0_6[6] = {'k', 0xC3, 0xA4, 0xC3, 0xA4, 'n'};
static const symbol s_0_7[2] = {'k', 'o'};
static const symbol s_0_8[3] = {'p', 0xC3, 0xA4};
static const symbol s_0_9[3] = {'k', 0xC3, 0xB6};

static const struct among a_0[10] = {
    /*  0 */ {2, s_0_0, -1, 1, 0},
    /*  1 */ {3, s_0_1, -1, 2, 0},
    /*  2 */ {4, s_0_2, -1, 1, 0},
    /*  3 */ {3, s_0_3, -1, 1, 0},
    /*  4 */ {3, s_0_4, -1, 1, 0},
    /*  5 */ {4, s_0_5, -1, 1, 0},
    /*  6 */ {6, s_0_6, -1, 1, 0},
    /*  7 */ {2, s_0_7, -1, 1, 0},
    /*  8 */ {3, s_0_8, -1, 1, 0},
    /*  9 */ {3, s_0_9, -1, 1, 0}};

static const symbol s_1_0[3] = {'l', 'l', 'a'};
static const symbol s_1_1[2] = {'n', 'a'};
static const symbol s_1_2[3] = {'s', 's', 'a'};
static const symbol s_1_3[2] = {'t', 'a'};
static const symbol s_1_4[3] = {'l', 't', 'a'};
static const symbol s_1_5[3] = {'s', 't', 'a'};

static const struct among a_1[6] = {
    /*  0 */ {3, s_1_0, -1, -1, 0},
    /*  1 */ {2, s_1_1, -1, -1, 0},
    /*  2 */ {3, s_1_2, -1, -1, 0},
    /*  3 */ {2, s_1_3, -1, -1, 0},
    /*  4 */ {3, s_1_4, 3, -1, 0},
    /*  5 */ {3, s_1_5, 3, -1, 0}};

static const symbol s_2_0[4] = {'l', 'l', 0xC3, 0xA4};
static const symbol s_2_1[3] = {'n', 0xC3, 0xA4};
static const symbol s_2_2[4] = {'s', 's', 0xC3, 0xA4};
static const symbol s_2_3[3] = {'t', 0xC3, 0xA4};
static const symbol s_2_4[4] = {'l', 't', 0xC3, 0xA4};
static const symbol s_2_5[4] = {'s', 't', 0xC3, 0xA4};

static const struct among a_2[6] = {
    /*  0 */ {4, s_2_0, -1, -1, 0},
    /*  1 */ {3, s_2_1, -1, -1, 0},
    /*  2 */ {4, s_2_2, -1, -1, 0},
    /*  3 */ {3, s_2_3, -1, -1, 0},
    /*  4 */ {4, s_2_4, 3, -1, 0},
    /*  5 */ {4, s_2_5, 3, -1, 0}};

static const symbol s_3_0[3] = {'l', 'l', 'e'};
static const symbol s_3_1[3] = {'i', 'n', 'e'};

static const struct among a_3[2] = {
    /*  0 */ {3, s_3_0, -1, -1, 0},
    /*  1 */ {3, s_3_1, -1, -1, 0}};

static const symbol s_4_0[3] = {'n', 's', 'a'};
static const symbol s_4_1[3] = {'m', 'm', 'e'};
static const symbol s_4_2[3] = {'n', 'n', 'e'};
static const symbol s_4_3[2] = {'n', 'i'};
static const symbol s_4_4[2] = {'s', 'i'};
static const symbol s_4_5[2] = {'a', 'n'};
static const symbol s_4_6[2] = {'e', 'n'};
static const symbol s_4_7[3] = {0xC3, 0xA4, 'n'};
static const symbol s_4_8[4] = {'n', 's', 0xC3, 0xA4};

static const struct among a_4[9] = {
    /*  0 */ {3, s_4_0, -1, 3, 0},
    /*  1 */ {3, s_4_1, -1, 3, 0},
    /*  2 */ {3, s_4_2, -1, 3, 0},
    /*  3 */ {2, s_4_3, -1, 2, 0},
    /*  4 */ {2, s_4_4, -1, 1, 0},
    /*  5 */ {2, s_4_5, -1, 4, 0},
    /*  6 */ {2, s_4_6, -1, 6, 0},
    /*  7 */ {3, s_4_7, -1, 5, 0},
    /*  8 */ {4, s_4_8, -1, 3, 0}};

static const symbol s_5_0[2] = {'a', 'a'};
static const symbol s_5_1[2] = {'e', 'e'};
static const symbol s_5_2[2] = {'i', 'i'};
static const symbol s_5_3[2] = {'o', 'o'};
static const symbol s_5_4[2] = {'u', 'u'};
static const symbol s_5_5[4] = {0xC3, 0xA4, 0xC3, 0xA4};
static const symbol s_5_6[4] = {0xC3, 0xB6, 0xC3, 0xB6};

static const struct among a_5[7] = {
    /*  0 */ {2, s_5_0, -1, -1, 0},
    /*  1 */ {2, s_5_1, -1, -1, 0},
    /*  2 */ {2, s_5_2, -1, -1, 0},
    /*  3 */ {2, s_5_3, -1, -1, 0},
    /*  4 */ {2, s_5_4, -1, -1, 0},
    /*  5 */ {4, s_5_5, -1, -1, 0},
    /*  6 */ {4, s_5_6, -1, -1, 0}};

static const symbol s_6_0[1] = {'a'};
static const symbol s_6_1[3] = {'l', 'l', 'a'};
static const symbol s_6_2[2] = {'n', 'a'};
static const symbol s_6_3[3] = {'s', 's', 'a'};
static const symbol s_6_4[2] = {'t', 'a'};
static const symbol s_6_5[3] = {'l', 't', 'a'};
static const symbol s_6_6[3] = {'s', 't', 'a'};
static const symbol s_6_7[3] = {'t', 't', 'a'};
static const symbol s_6_8[3] = {'l', 'l', 'e'};
static const symbol s_6_9[3] = {'i', 'n', 'e'};
static const symbol s_6_10[3] = {'k', 's', 'i'};
static const symbol s_6_11[1] = {'n'};
static const symbol s_6_12[3] = {'h', 'a', 'n'};
static const symbol s_6_13[3] = {'d', 'e', 'n'};
static const symbol s_6_14[4] = {'s', 'e', 'e', 'n'};
static const symbol s_6_15[3] = {'h', 'e', 'n'};
static const symbol s_6_16[4] = {'t', 't', 'e', 'n'};
static const symbol s_6_17[3] = {'h', 'i', 'n'};
static const symbol s_6_18[4] = {'s', 'i', 'i', 'n'};
static const symbol s_6_19[3] = {'h', 'o', 'n'};
static const symbol s_6_20[4] = {'h', 0xC3, 0xA4, 'n'};
static const symbol s_6_21[4] = {'h', 0xC3, 0xB6, 'n'};
static const symbol s_6_22[2] = {0xC3, 0xA4};
static const symbol s_6_23[4] = {'l', 'l', 0xC3, 0xA4};
static const symbol s_6_24[3] = {'n', 0xC3, 0xA4};
static const symbol s_6_25[4] = {'s', 's', 0xC3, 0xA4};
static const symbol s_6_26[3] = {'t', 0xC3, 0xA4};
static const symbol s_6_27[4] = {'l', 't', 0xC3, 0xA4};
static const symbol s_6_28[4] = {'s', 't', 0xC3, 0xA4};
static const symbol s_6_29[4] = {'t', 't', 0xC3, 0xA4};

static const struct among a_6[30] = {
    /*  0 */ {1, s_6_0, -1, 8, 0},
    /*  1 */ {3, s_6_1, 0, -1, 0},
    /*  2 */ {2, s_6_2, 0, -1, 0},
    /*  3 */ {3, s_6_3, 0, -1, 0},
    /*  4 */ {2, s_6_4, 0, -1, 0},
    /*  5 */ {3, s_6_5, 4, -1, 0},
    /*  6 */ {3, s_6_6, 4, -1, 0},
    /*  7 */ {3, s_6_7, 4, 9, 0},
    /*  8 */ {3, s_6_8, -1, -1, 0},
    /*  9 */ {3, s_6_9, -1, -1, 0},
    /* 10 */ {3, s_6_10, -1, -1, 0},
    /* 11 */ {1, s_6_11, -1, 7, 0},
    /* 12 */ {3, s_6_12, 11, 1, 0},
    /* 13 */ {3, s_6_13, 11, -1, r_VI},
    /* 14 */ {4, s_6_14, 11, -1, r_LONG},
    /* 15 */ {3, s_6_15, 11, 2, 0},
    /* 16 */ {4, s_6_16, 11, -1, r_VI},
    /* 17 */ {3, s_6_17, 11, 3, 0},
    /* 18 */ {4, s_6_18, 11, -1, r_VI},
    /* 19 */ {3, s_6_19, 11, 4, 0},
    /* 20 */ {4, s_6_20, 11, 5, 0},
    /* 21 */ {4, s_6_21, 11, 6, 0},
    /* 22 */ {2, s_6_22, -1, 8, 0},
    /* 23 */ {4, s_6_23, 22, -1, 0},
    /* 24 */ {3, s_6_24, 22, -1, 0},
    /* 25 */ {4, s_6_25, 22, -1, 0},
    /* 26 */ {3, s_6_26, 22, -1, 0},
    /* 27 */ {4, s_6_27, 26, -1, 0},
    /* 28 */ {4, s_6_28, 26, -1, 0},
    /* 29 */ {4, s_6_29, 26, 9, 0}};

static const symbol s_7_0[3] = {'e', 'j', 'a'};
static const symbol s_7_1[3] = {'m', 'm', 'a'};
static const symbol s_7_2[4] = {'i', 'm', 'm', 'a'};
static const symbol s_7_3[3] = {'m', 'p', 'a'};
static const symbol s_7_4[4] = {'i', 'm', 'p', 'a'};
static const symbol s_7_5[3] = {'m', 'm', 'i'};
static const symbol s_7_6[4] = {'i', 'm', 'm', 'i'};
static const symbol s_7_7[3] = {'m', 'p', 'i'};
static const symbol s_7_8[4] = {'i', 'm', 'p', 'i'};
static const symbol s_7_9[4] = {'e', 'j', 0xC3, 0xA4};
static const symbol s_7_10[4] = {'m', 'm', 0xC3, 0xA4};
static const symbol s_7_11[5] = {'i', 'm', 'm', 0xC3, 0xA4};
static const symbol s_7_12[4] = {'m', 'p', 0xC3, 0xA4};
static const symbol s_7_13[5] = {'i', 'm', 'p', 0xC3, 0xA4};

static const struct among a_7[14] = {
    /*  0 */ {3, s_7_0, -1, -1, 0},
    /*  1 */ {3, s_7_1, -1, 1, 0},
    /*  2 */ {4, s_7_2, 1, -1, 0},
    /*  3 */ {3, s_7_3, -1, 1, 0},
    /*  4 */ {4, s_7_4, 3, -1, 0},
    /*  5 */ {3, s_7_5, -1, 1, 0},
    /*  6 */ {4, s_7_6, 5, -1, 0},
    /*  7 */ {3, s_7_7, -1, 1, 0},
    /*  8 */ {4, s_7_8, 7, -1, 0},
    /*  9 */ {4, s_7_9, -1, -1, 0},
    /* 10 */ {4, s_7_10, -1, 1, 0},
    /* 11 */ {5, s_7_11, 10, -1, 0},
    /* 12 */ {4, s_7_12, -1, 1, 0},
    /* 13 */ {5, s_7_13, 12, -1, 0}};

static const symbol s_8_0[1] = {'i'};
static const symbol s_8_1[1] = {'j'};

static const struct among a_8[2] = {
    /*  0 */ {1, s_8_0, -1, -1, 0},
    /*  1 */ {1, s_8_1, -1, -1, 0}};

static const symbol s_9_0[3] = {'m', 'm', 'a'};
static const symbol s_9_1[4] = {'i', 'm', 'm', 'a'};

static const struct among a_9[2] = {
    /*  0 */ {3, s_9_0, -1, 1, 0},
    /*  1 */ {4, s_9_1, 0, -1, 0}};

static const unsigned char g_AEI[] = {17, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8};

static const unsigned char g_V1[] = {17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 32};

static const unsigned char g_V2[] = {17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 32};

static const unsigned char g_particle_end[] = {17, 97, 24, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 32};

static const symbol s_0[] = {'k'};
static const symbol s_1[] = {'k', 's', 'e'};
static const symbol s_2[] = {'k', 's', 'i'};
static const symbol s_3[] = {'i'};
static const symbol s_4[] = {'a'};
static const symbol s_5[] = {'e'};
static const symbol s_6[] = {'i'};
static const symbol s_7[] = {'o'};
static const symbol s_8[] = {0xC3, 0xA4};
static const symbol s_9[] = {0xC3, 0xB6};
static const symbol s_10[] = {'i', 'e'};
static const symbol s_11[] = {'e'};
static const symbol s_12[] = {'p', 'o'};
static const symbol s_13[] = {'t'};
static const symbol s_14[] = {'p', 'o'};
static const symbol s_15[] = {'j'};
static const symbol s_16[] = {'o'};
static const symbol s_17[] = {'u'};
static const symbol s_18[] = {'o'};
static const symbol s_19[] = {'j'};

static int r_mark_regions(struct SN_env *z) {
    z->I[0] = z->l;
    z->I[1] = z->l;
    if (out_grouping_U(z, g_V1, 97, 246, 1) < 0)
        return 0; /* goto */ /* grouping V1, line 46 */
    { /* gopast */           /* non V1, line 46 */
        int ret = in_grouping_U(z, g_V1, 97, 246, 1);
        if (ret < 0)
            return 0;
        z->c += ret;
    }
    z->I[0] = z->c; /* setmark p1, line 46 */
    if (out_grouping_U(z, g_V1, 97, 246, 1) < 0)
        return 0; /* goto */ /* grouping V1, line 47 */
    { /* gopast */           /* non V1, line 47 */
        int ret = in_grouping_U(z, g_V1, 97, 246, 1);
        if (ret < 0)
            return 0;
        z->c += ret;
    }
    z->I[1] = z->c; /* setmark p2, line 47 */
    return 1;
}

static int r_R2(struct SN_env *z) {
    if (!(z->I[1] <= z->c))
        return 0;
    return 1;
}

static int r_particle_etc(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 55 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 55 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c;                        /* [, line 55 */
        among_var = find_among_b(z, a_0, 10); /* substring, line 55 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 55 */
        z->lb = mlimit;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1:
            if (in_grouping_b_U(z, g_particle_end, 97, 246, 0))
                return 0;
            break;
        case 2: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 64 */
            if (ret < 0)
                return ret;
        } break;
    }
    {
        int ret = slice_del(z); /* delete, line 66 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_possessive(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 69 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 69 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c;                       /* [, line 69 */
        among_var = find_among_b(z, a_4, 9); /* substring, line 69 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 69 */
        z->lb = mlimit;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int m2 = z->l - z->c;
            (void)m2; /* not, line 72 */
            if (!(eq_s_b(z, 1, s_0)))
                goto lab0;
            return 0;
        lab0:
            z->c = z->l - m2;
        }
            {
                int ret = slice_del(z); /* delete, line 72 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 2: {
            int ret = slice_del(z); /* delete, line 74 */
            if (ret < 0)
                return ret;
        }
            z->ket = z->c; /* [, line 74 */
            if (!(eq_s_b(z, 3, s_1)))
                return 0;
            z->bra = z->c; /* ], line 74 */
            {
                int ret = slice_from_s(z, 3, s_2); /* <-, line 74 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 3: {
            int ret = slice_del(z); /* delete, line 78 */
            if (ret < 0)
                return ret;
        } break;
        case 4:
            if (z->c - 1 <= z->lb || z->p[z->c - 1] != 97)
                return 0;
            if (!(find_among_b(z, a_1, 6)))
                return 0; /* among, line 81 */
            {
                int ret = slice_del(z); /* delete, line 81 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 5:
            if (z->c - 2 <= z->lb || z->p[z->c - 1] != 164)
                return 0;
            if (!(find_among_b(z, a_2, 6)))
                return 0; /* among, line 83 */
            {
                int ret = slice_del(z); /* delete, line 84 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 6:
            if (z->c - 2 <= z->lb || z->p[z->c - 1] != 101)
                return 0;
            if (!(find_among_b(z, a_3, 2)))
                return 0; /* among, line 86 */
            {
                int ret = slice_del(z); /* delete, line 86 */
                if (ret < 0)
                    return ret;
            }
            break;
    }
    return 1;
}

static int r_LONG(struct SN_env *z) {
    if (!(find_among_b(z, a_5, 7)))
        return 0; /* among, line 91 */
    return 1;
}

static int r_VI(struct SN_env *z) {
    if (!(eq_s_b(z, 1, s_3)))
        return 0;
    if (in_grouping_b_U(z, g_V2, 97, 246, 0))
        return 0;
    return 1;
}

static int r_case_ending(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 96 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 96 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c;                        /* [, line 96 */
        among_var = find_among_b(z, a_6, 30); /* substring, line 96 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 96 */
        z->lb = mlimit;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1:
            if (!(eq_s_b(z, 1, s_4)))
                return 0;
            break;
        case 2:
            if (!(eq_s_b(z, 1, s_5)))
                return 0;
            break;
        case 3:
            if (!(eq_s_b(z, 1, s_6)))
                return 0;
            break;
        case 4:
            if (!(eq_s_b(z, 1, s_7)))
                return 0;
            break;
        case 5:
            if (!(eq_s_b(z, 2, s_8)))
                return 0;
            break;
        case 6:
            if (!(eq_s_b(z, 2, s_9)))
                return 0;
            break;
        case 7: {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 111 */
            {
                int m2 = z->l - z->c;
                (void)m2; /* and, line 113 */
                {
                    int m3 = z->l - z->c;
                    (void)m3; /* or, line 112 */
                    {
                        int ret = r_LONG(z);
                        if (ret == 0)
                            goto lab2; /* call LONG, line 111 */
                        if (ret < 0)
                            return ret;
                    }
                    goto lab1;
                lab2:
                    z->c = z->l - m3;
                    if (!(eq_s_b(z, 2, s_10))) {
                        z->c = z->l - m_keep;
                        goto lab0;
                    }
                }
            lab1:
                z->c = z->l - m2;
                {
                    int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
                    if (ret < 0) {
                        z->c = z->l - m_keep;
                        goto lab0;
                    }
                    z->c = ret; /* next, line 113 */
                }
            }
            z->bra = z->c; /* ], line 113 */
        lab0:;
        } break;
        case 8:
            if (in_grouping_b_U(z, g_V1, 97, 246, 0))
                return 0;
            if (out_grouping_b_U(z, g_V1, 97, 246, 0))
                return 0;
            break;
        case 9:
            if (!(eq_s_b(z, 1, s_11)))
                return 0;
            break;
    }
    {
        int ret = slice_del(z); /* delete, line 138 */
        if (ret < 0)
            return ret;
    }
    z->B[0] = 1; /* set ending_removed, line 139 */
    return 1;
}

static int r_other_endings(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 142 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[1])
            return 0;
        z->c = z->I[1]; /* tomark, line 142 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c;                        /* [, line 142 */
        among_var = find_among_b(z, a_7, 14); /* substring, line 142 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 142 */
        z->lb = mlimit;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int m2 = z->l - z->c;
            (void)m2; /* not, line 146 */
            if (!(eq_s_b(z, 2, s_12)))
                goto lab0;
            return 0;
        lab0:
            z->c = z->l - m2;
        } break;
    }
    {
        int ret = slice_del(z); /* delete, line 151 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_i_plural(struct SN_env *z) {
    {
        int mlimit; /* setlimit, line 154 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 154 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 154 */
        if (z->c <= z->lb || (z->p[z->c - 1] != 105 && z->p[z->c - 1] != 106)) {
            z->lb = mlimit;
            return 0;
        }
        if (!(find_among_b(z, a_8, 2))) {
            z->lb = mlimit;
            return 0;
        } /* substring, line 154 */
        z->bra = z->c; /* ], line 154 */
        z->lb = mlimit;
    }
    {
        int ret = slice_del(z); /* delete, line 158 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_t_plural(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 161 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 161 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 162 */
        if (!(eq_s_b(z, 1, s_13))) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 162 */
        {
            int m_test = z->l - z->c; /* test, line 162 */
            if (in_grouping_b_U(z, g_V1, 97, 246, 0)) {
                z->lb = mlimit;
                return 0;
            }
            z->c = z->l - m_test;
        }
        {
            int ret = slice_del(z); /* delete, line 163 */
            if (ret < 0)
                return ret;
        }
        z->lb = mlimit;
    }
    {
        int mlimit; /* setlimit, line 165 */
        int m2 = z->l - z->c;
        (void)m2;
        if (z->c < z->I[1])
            return 0;
        z->c = z->I[1]; /* tomark, line 165 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m2;
        z->ket = z->c; /* [, line 165 */
        if (z->c - 2 <= z->lb || z->p[z->c - 1] != 97) {
            z->lb = mlimit;
            return 0;
        }
        among_var = find_among_b(z, a_9, 2); /* substring, line 165 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 165 */
        z->lb = mlimit;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int m3 = z->l - z->c;
            (void)m3; /* not, line 167 */
            if (!(eq_s_b(z, 2, s_14)))
                goto lab0;
            return 0;
        lab0:
            z->c = z->l - m3;
        } break;
    }
    {
        int ret = slice_del(z); /* delete, line 170 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_tidy(struct SN_env *z) {
    {
        int mlimit; /* setlimit, line 173 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 173 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        {
            int m2 = z->l - z->c;
            (void)m2; /* do, line 174 */
            {
                int m3 = z->l - z->c;
                (void)m3; /* and, line 174 */
                {
                    int ret = r_LONG(z);
                    if (ret == 0)
                        goto lab0; /* call LONG, line 174 */
                    if (ret < 0)
                        return ret;
                }
                z->c = z->l - m3;
                z->ket = z->c; /* [, line 174 */
                {
                    int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
                    if (ret < 0)
                        goto lab0;
                    z->c = ret; /* next, line 174 */
                }
                z->bra = z->c; /* ], line 174 */
                {
                    int ret = slice_del(z); /* delete, line 174 */
                    if (ret < 0)
                        return ret;
                }
            }
        lab0:
            z->c = z->l - m2;
        }
        {
            int m4 = z->l - z->c;
            (void)m4;      /* do, line 175 */
            z->ket = z->c; /* [, line 175 */
            if (in_grouping_b_U(z, g_AEI, 97, 228, 0))
                goto lab1;
            z->bra = z->c; /* ], line 175 */
            if (out_grouping_b_U(z, g_V1, 97, 246, 0))
                goto lab1;
            {
                int ret = slice_del(z); /* delete, line 175 */
                if (ret < 0)
                    return ret;
            }
        lab1:
            z->c = z->l - m4;
        }
        {
            int m5 = z->l - z->c;
            (void)m5;      /* do, line 176 */
            z->ket = z->c; /* [, line 176 */
            if (!(eq_s_b(z, 1, s_15)))
                goto lab2;
            z->bra = z->c; /* ], line 176 */
            {
                int m6 = z->l - z->c;
                (void)m6; /* or, line 176 */
                if (!(eq_s_b(z, 1, s_16)))
                    goto lab4;
                goto lab3;
            lab4:
                z->c = z->l - m6;
                if (!(eq_s_b(z, 1, s_17)))
                    goto lab2;
            }
        lab3: {
            int ret = slice_del(z); /* delete, line 176 */
            if (ret < 0)
                return ret;
        }
        lab2:
            z->c = z->l - m5;
        }
        {
            int m7 = z->l - z->c;
            (void)m7;      /* do, line 177 */
            z->ket = z->c; /* [, line 177 */
            if (!(eq_s_b(z, 1, s_18)))
                goto lab5;
            z->bra = z->c; /* ], line 177 */
            if (!(eq_s_b(z, 1, s_19)))
                goto lab5;
            {
                int ret = slice_del(z); /* delete, line 177 */
                if (ret < 0)
                    return ret;
            }
        lab5:
            z->c = z->l - m7;
        }
        z->lb = mlimit;
    }
    if (in_grouping_b_U(z, g_V1, 97, 246, 1) < 0)
        return 0; /* goto */ /* non V1, line 179 */
    z->ket = z->c;           /* [, line 179 */
    {
        int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
        if (ret < 0)
            return 0;
        z->c = ret; /* next, line 179 */
    }
    z->bra = z->c;                  /* ], line 179 */
    z->S[0] = slice_to(z, z->S[0]); /* -> x, line 179 */
    if (z->S[0] == 0)
        return -1; /* -> x, line 179 */
    if (!(eq_v_b(z, z->S[0])))
        return 0; /* name x, line 179 */
    {
        int ret = slice_del(z); /* delete, line 179 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

extern int finnish_UTF_8_stem(struct SN_env *z) {
    {
        int c1 = z->c; /* do, line 185 */
        {
            int ret = r_mark_regions(z);
            if (ret == 0)
                goto lab0; /* call mark_regions, line 185 */
            if (ret < 0)
                return ret;
        }
    lab0:
        z->c = c1;
    }
    z->B[0] = 0; /* unset ending_removed, line 186 */
    z->lb = z->c;
    z->c = z->l; /* backwards, line 187 */

    {
        int m2 = z->l - z->c;
        (void)m2; /* do, line 188 */
        {
            int ret = r_particle_etc(z);
            if (ret == 0)
                goto lab1; /* call particle_etc, line 188 */
            if (ret < 0)
                return ret;
        }
    lab1:
        z->c = z->l - m2;
    }
    {
        int m3 = z->l - z->c;
        (void)m3; /* do, line 189 */
        {
            int ret = r_possessive(z);
            if (ret == 0)
                goto lab2; /* call possessive, line 189 */
            if (ret < 0)
                return ret;
        }
    lab2:
        z->c = z->l - m3;
    }
    {
        int m4 = z->l - z->c;
        (void)m4; /* do, line 190 */
        {
            int ret = r_case_ending(z);
            if (ret == 0)
                goto lab3; /* call case_ending, line 190 */
            if (ret < 0)
                return ret;
        }
    lab3:
        z->c = z->l - m4;
    }
    {
        int m5 = z->l - z->c;
        (void)m5; /* do, line 191 */
        {
            int ret = r_other_endings(z);
            if (ret == 0)
                goto lab4; /* call other_endings, line 191 */
            if (ret < 0)
                return ret;
        }
    lab4:
        z->c = z->l - m5;
    }
    {
        int m6 = z->l - z->c;
        (void)m6; /* or, line 192 */
        if (!(z->B[0]))
            goto lab6; /* Boolean test ending_removed, line 192 */
        {
            int m7 = z->l - z->c;
            (void)m7; /* do, line 192 */
            {
                int ret = r_i_plural(z);
                if (ret == 0)
                    goto lab7; /* call i_plural, line 192 */
                if (ret < 0)
                    return ret;
            }
        lab7:
            z->c = z->l - m7;
        }
        goto lab5;
    lab6:
        z->c = z->l - m6;
        {
            int m8 = z->l - z->c;
            (void)m8; /* do, line 192 */
            {
                int ret = r_t_plural(z);
                if (ret == 0)
                    goto lab8; /* call t_plural, line 192 */
                if (ret < 0)
                    return ret;
            }
        lab8:
            z->c = z->l - m8;
        }
    }
lab5: {
    int m9 = z->l - z->c;
    (void)m9; /* do, line 193 */
    {
        int ret = r_tidy(z);
        if (ret == 0)
            goto lab9; /* call tidy, line 193 */
        if (ret < 0)
            return ret;
    }
lab9:
    z->c = z->l - m9;
}
    z->c = z->lb;
    return 1;
}

extern struct SN_env *finnish_UTF_8_create_env(void) { return SN_create_env(1, 2, 1); }

extern void finnish_UTF_8_close_env(struct SN_env *z) { SN_close_env(z, 1); }
