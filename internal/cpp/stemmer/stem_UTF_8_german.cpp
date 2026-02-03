
/* This file was generated automatically by the Snowball to ANSI C compiler */

#include "header.h"

#ifdef __cplusplus
extern "C" {
#endif
extern int german_UTF_8_stem(struct SN_env *z);
#ifdef __cplusplus
}
#endif
static int r_standard_suffix(struct SN_env *z);
static int r_R2(struct SN_env *z);
static int r_R1(struct SN_env *z);
static int r_mark_regions(struct SN_env *z);
static int r_postlude(struct SN_env *z);
static int r_prelude(struct SN_env *z);
#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *german_UTF_8_create_env(void);
extern void german_UTF_8_close_env(struct SN_env *z);

#ifdef __cplusplus
}
#endif
static const symbol s_0_1[1] = {'U'};
static const symbol s_0_2[1] = {'Y'};
static const symbol s_0_3[2] = {0xC3, 0xA4};
static const symbol s_0_4[2] = {0xC3, 0xB6};
static const symbol s_0_5[2] = {0xC3, 0xBC};

static const struct among a_0[6] = {
    /*  0 */ {0, 0, -1, 6, 0},
    /*  1 */ {1, s_0_1, 0, 2, 0},
    /*  2 */ {1, s_0_2, 0, 1, 0},
    /*  3 */ {2, s_0_3, 0, 3, 0},
    /*  4 */ {2, s_0_4, 0, 4, 0},
    /*  5 */ {2, s_0_5, 0, 5, 0}};

static const symbol s_1_0[1] = {'e'};
static const symbol s_1_1[2] = {'e', 'm'};
static const symbol s_1_2[2] = {'e', 'n'};
static const symbol s_1_3[3] = {'e', 'r', 'n'};
static const symbol s_1_4[2] = {'e', 'r'};
static const symbol s_1_5[1] = {'s'};
static const symbol s_1_6[2] = {'e', 's'};

static const struct among a_1[7] = {
    /*  0 */ {1, s_1_0, -1, 1, 0},
    /*  1 */ {2, s_1_1, -1, 1, 0},
    /*  2 */ {2, s_1_2, -1, 1, 0},
    /*  3 */ {3, s_1_3, -1, 1, 0},
    /*  4 */ {2, s_1_4, -1, 1, 0},
    /*  5 */ {1, s_1_5, -1, 2, 0},
    /*  6 */ {2, s_1_6, 5, 1, 0}};

static const symbol s_2_0[2] = {'e', 'n'};
static const symbol s_2_1[2] = {'e', 'r'};
static const symbol s_2_2[2] = {'s', 't'};
static const symbol s_2_3[3] = {'e', 's', 't'};

static const struct among a_2[4] = {
    /*  0 */ {2, s_2_0, -1, 1, 0},
    /*  1 */ {2, s_2_1, -1, 1, 0},
    /*  2 */ {2, s_2_2, -1, 2, 0},
    /*  3 */ {3, s_2_3, 2, 1, 0}};

static const symbol s_3_0[2] = {'i', 'g'};
static const symbol s_3_1[4] = {'l', 'i', 'c', 'h'};

static const struct among a_3[2] = {
    /*  0 */ {2, s_3_0, -1, 1, 0},
    /*  1 */ {4, s_3_1, -1, 1, 0}};

static const symbol s_4_0[3] = {'e', 'n', 'd'};
static const symbol s_4_1[2] = {'i', 'g'};
static const symbol s_4_2[3] = {'u', 'n', 'g'};
static const symbol s_4_3[4] = {'l', 'i', 'c', 'h'};
static const symbol s_4_4[4] = {'i', 's', 'c', 'h'};
static const symbol s_4_5[2] = {'i', 'k'};
static const symbol s_4_6[4] = {'h', 'e', 'i', 't'};
static const symbol s_4_7[4] = {'k', 'e', 'i', 't'};

static const struct among a_4[8] = {
    /*  0 */ {3, s_4_0, -1, 1, 0},
    /*  1 */ {2, s_4_1, -1, 2, 0},
    /*  2 */ {3, s_4_2, -1, 1, 0},
    /*  3 */ {4, s_4_3, -1, 3, 0},
    /*  4 */ {4, s_4_4, -1, 2, 0},
    /*  5 */ {2, s_4_5, -1, 2, 0},
    /*  6 */ {4, s_4_6, -1, 3, 0},
    /*  7 */ {4, s_4_7, -1, 4, 0}};

static const unsigned char g_v[] = {17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 32, 8};

static const unsigned char g_s_ending[] = {117, 30, 5};

static const unsigned char g_st_ending[] = {117, 30, 4};

static const symbol s_0[] = {0xC3, 0x9F};
static const symbol s_1[] = {'s', 's'};
static const symbol s_2[] = {'u'};
static const symbol s_3[] = {'U'};
static const symbol s_4[] = {'y'};
static const symbol s_5[] = {'Y'};
static const symbol s_6[] = {'y'};
static const symbol s_7[] = {'u'};
static const symbol s_8[] = {'a'};
static const symbol s_9[] = {'o'};
static const symbol s_10[] = {'u'};
static const symbol s_11[] = {'i', 'g'};
static const symbol s_12[] = {'e'};
static const symbol s_13[] = {'e'};
static const symbol s_14[] = {'e', 'r'};
static const symbol s_15[] = {'e', 'n'};

static int r_prelude(struct SN_env *z) {
    {
        int c_test = z->c; /* test, line 30 */
        while (1) {        /* repeat, line 30 */
            int c1 = z->c;
            {
                int c2 = z->c; /* or, line 33 */
                z->bra = z->c; /* [, line 32 */
                if (!(eq_s(z, 2, s_0)))
                    goto lab2;
                z->ket = z->c; /* ], line 32 */
                {
                    int ret = slice_from_s(z, 2, s_1); /* <-, line 32 */
                    if (ret < 0)
                        return ret;
                }
                goto lab1;
            lab2:
                z->c = c2;
                {
                    int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                    if (ret < 0)
                        goto lab0;
                    z->c = ret; /* next, line 33 */
                }
            }
        lab1:
            continue;
        lab0:
            z->c = c1;
            break;
        }
        z->c = c_test;
    }
    while (1) { /* repeat, line 36 */
        int c3 = z->c;
        while (1) { /* goto, line 36 */
            int c4 = z->c;
            if (in_grouping_U(z, g_v, 97, 252, 0))
                goto lab4;
            z->bra = z->c; /* [, line 37 */
            {
                int c5 = z->c; /* or, line 37 */
                if (!(eq_s(z, 1, s_2)))
                    goto lab6;
                z->ket = z->c; /* ], line 37 */
                if (in_grouping_U(z, g_v, 97, 252, 0))
                    goto lab6;
                {
                    int ret = slice_from_s(z, 1, s_3); /* <-, line 37 */
                    if (ret < 0)
                        return ret;
                }
                goto lab5;
            lab6:
                z->c = c5;
                if (!(eq_s(z, 1, s_4)))
                    goto lab4;
                z->ket = z->c; /* ], line 38 */
                if (in_grouping_U(z, g_v, 97, 252, 0))
                    goto lab4;
                {
                    int ret = slice_from_s(z, 1, s_5); /* <-, line 38 */
                    if (ret < 0)
                        return ret;
                }
            }
        lab5:
            z->c = c4;
            break;
        lab4:
            z->c = c4;
            {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    goto lab3;
                z->c = ret; /* goto, line 36 */
            }
        }
        continue;
    lab3:
        z->c = c3;
        break;
    }
    return 1;
}

static int r_mark_regions(struct SN_env *z) {
    z->I[0] = z->l;
    z->I[1] = z->l;
    {
        int c_test = z->c; /* test, line 47 */
        {
            int ret = skip_utf8(z->p, z->c, 0, z->l, +3);
            if (ret < 0)
                return 0;
            z->c = ret; /* hop, line 47 */
        }
        z->I[2] = z->c; /* setmark x, line 47 */
        z->c = c_test;
    }
    { /* gopast */ /* grouping v, line 49 */
        int ret = out_grouping_U(z, g_v, 97, 252, 1);
        if (ret < 0)
            return 0;
        z->c += ret;
    }
    { /* gopast */ /* non v, line 49 */
        int ret = in_grouping_U(z, g_v, 97, 252, 1);
        if (ret < 0)
            return 0;
        z->c += ret;
    }
    z->I[0] = z->c; /* setmark p1, line 49 */
                    /* try, line 50 */
    if (!(z->I[0] < z->I[2]))
        goto lab0;
    z->I[0] = z->I[2];
lab0: { /* gopast */ /* grouping v, line 51 */
    int ret = out_grouping_U(z, g_v, 97, 252, 1);
    if (ret < 0)
        return 0;
    z->c += ret;
}
    { /* gopast */ /* non v, line 51 */
        int ret = in_grouping_U(z, g_v, 97, 252, 1);
        if (ret < 0)
            return 0;
        z->c += ret;
    }
    z->I[1] = z->c; /* setmark p2, line 51 */
    return 1;
}

static int r_postlude(struct SN_env *z) {
    int among_var;
    while (1) { /* repeat, line 55 */
        int c1 = z->c;
        z->bra = z->c;                     /* [, line 57 */
        among_var = find_among(z, a_0, 6); /* substring, line 57 */
        if (!(among_var))
            goto lab0;
        z->ket = z->c; /* ], line 57 */
        switch (among_var) {
            case 0:
                goto lab0;
            case 1: {
                int ret = slice_from_s(z, 1, s_6); /* <-, line 58 */
                if (ret < 0)
                    return ret;
            } break;
            case 2: {
                int ret = slice_from_s(z, 1, s_7); /* <-, line 59 */
                if (ret < 0)
                    return ret;
            } break;
            case 3: {
                int ret = slice_from_s(z, 1, s_8); /* <-, line 60 */
                if (ret < 0)
                    return ret;
            } break;
            case 4: {
                int ret = slice_from_s(z, 1, s_9); /* <-, line 61 */
                if (ret < 0)
                    return ret;
            } break;
            case 5: {
                int ret = slice_from_s(z, 1, s_10); /* <-, line 62 */
                if (ret < 0)
                    return ret;
            } break;
            case 6: {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    goto lab0;
                z->c = ret; /* next, line 63 */
            } break;
        }
        continue;
    lab0:
        z->c = c1;
        break;
    }
    return 1;
}

static int r_R1(struct SN_env *z) {
    if (!(z->I[0] <= z->c))
        return 0;
    return 1;
}

static int r_R2(struct SN_env *z) {
    if (!(z->I[1] <= z->c))
        return 0;
    return 1;
}

static int r_standard_suffix(struct SN_env *z) {
    int among_var;
    {
        int m1 = z->l - z->c;
        (void)m1;      /* do, line 74 */
        z->ket = z->c; /* [, line 75 */
        if (z->c <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((811040 >> (z->p[z->c - 1] & 0x1f)) & 1))
            goto lab0;
        among_var = find_among_b(z, a_1, 7); /* substring, line 75 */
        if (!(among_var))
            goto lab0;
        z->bra = z->c; /* ], line 75 */
        {
            int ret = r_R1(z);
            if (ret == 0)
                goto lab0; /* call R1, line 75 */
            if (ret < 0)
                return ret;
        }
        switch (among_var) {
            case 0:
                goto lab0;
            case 1: {
                int ret = slice_del(z); /* delete, line 77 */
                if (ret < 0)
                    return ret;
            } break;
            case 2:
                if (in_grouping_b_U(z, g_s_ending, 98, 116, 0))
                    goto lab0;
                {
                    int ret = slice_del(z); /* delete, line 80 */
                    if (ret < 0)
                        return ret;
                }
                break;
        }
    lab0:
        z->c = z->l - m1;
    }
    {
        int m2 = z->l - z->c;
        (void)m2;      /* do, line 84 */
        z->ket = z->c; /* [, line 85 */
        if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((1327104 >> (z->p[z->c - 1] & 0x1f)) & 1))
            goto lab1;
        among_var = find_among_b(z, a_2, 4); /* substring, line 85 */
        if (!(among_var))
            goto lab1;
        z->bra = z->c; /* ], line 85 */
        {
            int ret = r_R1(z);
            if (ret == 0)
                goto lab1; /* call R1, line 85 */
            if (ret < 0)
                return ret;
        }
        switch (among_var) {
            case 0:
                goto lab1;
            case 1: {
                int ret = slice_del(z); /* delete, line 87 */
                if (ret < 0)
                    return ret;
            } break;
            case 2:
                if (in_grouping_b_U(z, g_st_ending, 98, 116, 0))
                    goto lab1;
                {
                    int ret = skip_utf8(z->p, z->c, z->lb, z->l, -3);
                    if (ret < 0)
                        goto lab1;
                    z->c = ret; /* hop, line 90 */
                }
                {
                    int ret = slice_del(z); /* delete, line 90 */
                    if (ret < 0)
                        return ret;
                }
                break;
        }
    lab1:
        z->c = z->l - m2;
    }
    {
        int m3 = z->l - z->c;
        (void)m3;      /* do, line 94 */
        z->ket = z->c; /* [, line 95 */
        if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((1051024 >> (z->p[z->c - 1] & 0x1f)) & 1))
            goto lab2;
        among_var = find_among_b(z, a_4, 8); /* substring, line 95 */
        if (!(among_var))
            goto lab2;
        z->bra = z->c; /* ], line 95 */
        {
            int ret = r_R2(z);
            if (ret == 0)
                goto lab2; /* call R2, line 95 */
            if (ret < 0)
                return ret;
        }
        switch (among_var) {
            case 0:
                goto lab2;
            case 1: {
                int ret = slice_del(z); /* delete, line 97 */
                if (ret < 0)
                    return ret;
            }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 98 */
                    z->ket = z->c;                                /* [, line 98 */
                    if (!(eq_s_b(z, 2, s_11))) {
                        z->c = z->l - m_keep;
                        goto lab3;
                    }
                    z->bra = z->c; /* ], line 98 */
                    {
                        int m4 = z->l - z->c;
                        (void)m4; /* not, line 98 */
                        if (!(eq_s_b(z, 1, s_12)))
                            goto lab4;
                        {
                            z->c = z->l - m_keep;
                            goto lab3;
                        }
                    lab4:
                        z->c = z->l - m4;
                    }
                    {
                        int ret = r_R2(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab3;
                        } /* call R2, line 98 */
                        if (ret < 0)
                            return ret;
                    }
                    {
                        int ret = slice_del(z); /* delete, line 98 */
                        if (ret < 0)
                            return ret;
                    }
                lab3:;
                }
                break;
            case 2: {
                int m5 = z->l - z->c;
                (void)m5; /* not, line 101 */
                if (!(eq_s_b(z, 1, s_13)))
                    goto lab5;
                goto lab2;
            lab5:
                z->c = z->l - m5;
            }
                {
                    int ret = slice_del(z); /* delete, line 101 */
                    if (ret < 0)
                        return ret;
                }
                break;
            case 3: {
                int ret = slice_del(z); /* delete, line 104 */
                if (ret < 0)
                    return ret;
            }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 105 */
                    z->ket = z->c;                                /* [, line 106 */
                    {
                        int m6 = z->l - z->c;
                        (void)m6; /* or, line 106 */
                        if (!(eq_s_b(z, 2, s_14)))
                            goto lab8;
                        goto lab7;
                    lab8:
                        z->c = z->l - m6;
                        if (!(eq_s_b(z, 2, s_15))) {
                            z->c = z->l - m_keep;
                            goto lab6;
                        }
                    }
                lab7:
                    z->bra = z->c; /* ], line 106 */
                    {
                        int ret = r_R1(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab6;
                        } /* call R1, line 106 */
                        if (ret < 0)
                            return ret;
                    }
                    {
                        int ret = slice_del(z); /* delete, line 106 */
                        if (ret < 0)
                            return ret;
                    }
                lab6:;
                }
                break;
            case 4: {
                int ret = slice_del(z); /* delete, line 110 */
                if (ret < 0)
                    return ret;
            }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 111 */
                    z->ket = z->c;                                /* [, line 112 */
                    if (z->c - 1 <= z->lb || (z->p[z->c - 1] != 103 && z->p[z->c - 1] != 104)) {
                        z->c = z->l - m_keep;
                        goto lab9;
                    }
                    among_var = find_among_b(z, a_3, 2); /* substring, line 112 */
                    if (!(among_var)) {
                        z->c = z->l - m_keep;
                        goto lab9;
                    }
                    z->bra = z->c; /* ], line 112 */
                    {
                        int ret = r_R2(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab9;
                        } /* call R2, line 112 */
                        if (ret < 0)
                            return ret;
                    }
                    switch (among_var) {
                        case 0: {
                            z->c = z->l - m_keep;
                            goto lab9;
                        }
                        case 1: {
                            int ret = slice_del(z); /* delete, line 114 */
                            if (ret < 0)
                                return ret;
                        } break;
                    }
                lab9:;
                }
                break;
        }
    lab2:
        z->c = z->l - m3;
    }
    return 1;
}

extern int german_UTF_8_stem(struct SN_env *z) {
    {
        int c1 = z->c; /* do, line 125 */
        {
            int ret = r_prelude(z);
            if (ret == 0)
                goto lab0; /* call prelude, line 125 */
            if (ret < 0)
                return ret;
        }
    lab0:
        z->c = c1;
    }
    {
        int c2 = z->c; /* do, line 126 */
        {
            int ret = r_mark_regions(z);
            if (ret == 0)
                goto lab1; /* call mark_regions, line 126 */
            if (ret < 0)
                return ret;
        }
    lab1:
        z->c = c2;
    }
    z->lb = z->c;
    z->c = z->l; /* backwards, line 127 */

    {
        int m3 = z->l - z->c;
        (void)m3; /* do, line 128 */
        {
            int ret = r_standard_suffix(z);
            if (ret == 0)
                goto lab2; /* call standard_suffix, line 128 */
            if (ret < 0)
                return ret;
        }
    lab2:
        z->c = z->l - m3;
    }
    z->c = z->lb;
    {
        int c4 = z->c; /* do, line 129 */
        {
            int ret = r_postlude(z);
            if (ret == 0)
                goto lab3; /* call postlude, line 129 */
            if (ret < 0)
                return ret;
        }
    lab3:
        z->c = c4;
    }
    return 1;
}

extern struct SN_env *german_UTF_8_create_env(void) { return SN_create_env(0, 3, 0); }

extern void german_UTF_8_close_env(struct SN_env *z) { SN_close_env(z, 0); }
