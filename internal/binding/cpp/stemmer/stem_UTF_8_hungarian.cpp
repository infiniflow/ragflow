
/* This file was generated automatically by the Snowball to ANSI C compiler */

#include "header.h"

#ifdef __cplusplus
extern "C" {
#endif
extern int hungarian_UTF_8_stem(struct SN_env *z);
#ifdef __cplusplus
}
#endif
static int r_double(struct SN_env *z);
static int r_undouble(struct SN_env *z);
static int r_factive(struct SN_env *z);
static int r_instrum(struct SN_env *z);
static int r_plur_owner(struct SN_env *z);
static int r_sing_owner(struct SN_env *z);
static int r_owned(struct SN_env *z);
static int r_plural(struct SN_env *z);
static int r_case_other(struct SN_env *z);
static int r_case_special(struct SN_env *z);
static int r_case(struct SN_env *z);
static int r_v_ending(struct SN_env *z);
static int r_R1(struct SN_env *z);
static int r_mark_regions(struct SN_env *z);
#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *hungarian_UTF_8_create_env(void);
extern void hungarian_UTF_8_close_env(struct SN_env *z);

#ifdef __cplusplus
}
#endif
static const symbol s_0_0[2] = {'c', 's'};
static const symbol s_0_1[3] = {'d', 'z', 's'};
static const symbol s_0_2[2] = {'g', 'y'};
static const symbol s_0_3[2] = {'l', 'y'};
static const symbol s_0_4[2] = {'n', 'y'};
static const symbol s_0_5[2] = {'s', 'z'};
static const symbol s_0_6[2] = {'t', 'y'};
static const symbol s_0_7[2] = {'z', 's'};

static const struct among a_0[8] = {
    /*  0 */ {2, s_0_0, -1, -1, 0},
    /*  1 */ {3, s_0_1, -1, -1, 0},
    /*  2 */ {2, s_0_2, -1, -1, 0},
    /*  3 */ {2, s_0_3, -1, -1, 0},
    /*  4 */ {2, s_0_4, -1, -1, 0},
    /*  5 */ {2, s_0_5, -1, -1, 0},
    /*  6 */ {2, s_0_6, -1, -1, 0},
    /*  7 */ {2, s_0_7, -1, -1, 0}};

static const symbol s_1_0[2] = {0xC3, 0xA1};
static const symbol s_1_1[2] = {0xC3, 0xA9};

static const struct among a_1[2] = {
    /*  0 */ {2, s_1_0, -1, 1, 0},
    /*  1 */ {2, s_1_1, -1, 2, 0}};

static const symbol s_2_0[2] = {'b', 'b'};
static const symbol s_2_1[2] = {'c', 'c'};
static const symbol s_2_2[2] = {'d', 'd'};
static const symbol s_2_3[2] = {'f', 'f'};
static const symbol s_2_4[2] = {'g', 'g'};
static const symbol s_2_5[2] = {'j', 'j'};
static const symbol s_2_6[2] = {'k', 'k'};
static const symbol s_2_7[2] = {'l', 'l'};
static const symbol s_2_8[2] = {'m', 'm'};
static const symbol s_2_9[2] = {'n', 'n'};
static const symbol s_2_10[2] = {'p', 'p'};
static const symbol s_2_11[2] = {'r', 'r'};
static const symbol s_2_12[3] = {'c', 'c', 's'};
static const symbol s_2_13[2] = {'s', 's'};
static const symbol s_2_14[3] = {'z', 'z', 's'};
static const symbol s_2_15[2] = {'t', 't'};
static const symbol s_2_16[2] = {'v', 'v'};
static const symbol s_2_17[3] = {'g', 'g', 'y'};
static const symbol s_2_18[3] = {'l', 'l', 'y'};
static const symbol s_2_19[3] = {'n', 'n', 'y'};
static const symbol s_2_20[3] = {'t', 't', 'y'};
static const symbol s_2_21[3] = {'s', 's', 'z'};
static const symbol s_2_22[2] = {'z', 'z'};

static const struct among a_2[23] = {
    /*  0 */ {2, s_2_0, -1, -1, 0},
    /*  1 */ {2, s_2_1, -1, -1, 0},
    /*  2 */ {2, s_2_2, -1, -1, 0},
    /*  3 */ {2, s_2_3, -1, -1, 0},
    /*  4 */ {2, s_2_4, -1, -1, 0},
    /*  5 */ {2, s_2_5, -1, -1, 0},
    /*  6 */ {2, s_2_6, -1, -1, 0},
    /*  7 */ {2, s_2_7, -1, -1, 0},
    /*  8 */ {2, s_2_8, -1, -1, 0},
    /*  9 */ {2, s_2_9, -1, -1, 0},
    /* 10 */ {2, s_2_10, -1, -1, 0},
    /* 11 */ {2, s_2_11, -1, -1, 0},
    /* 12 */ {3, s_2_12, -1, -1, 0},
    /* 13 */ {2, s_2_13, -1, -1, 0},
    /* 14 */ {3, s_2_14, -1, -1, 0},
    /* 15 */ {2, s_2_15, -1, -1, 0},
    /* 16 */ {2, s_2_16, -1, -1, 0},
    /* 17 */ {3, s_2_17, -1, -1, 0},
    /* 18 */ {3, s_2_18, -1, -1, 0},
    /* 19 */ {3, s_2_19, -1, -1, 0},
    /* 20 */ {3, s_2_20, -1, -1, 0},
    /* 21 */ {3, s_2_21, -1, -1, 0},
    /* 22 */ {2, s_2_22, -1, -1, 0}};

static const symbol s_3_0[2] = {'a', 'l'};
static const symbol s_3_1[2] = {'e', 'l'};

static const struct among a_3[2] = {
    /*  0 */ {2, s_3_0, -1, 1, 0},
    /*  1 */ {2, s_3_1, -1, 2, 0}};

static const symbol s_4_0[2] = {'b', 'a'};
static const symbol s_4_1[2] = {'r', 'a'};
static const symbol s_4_2[2] = {'b', 'e'};
static const symbol s_4_3[2] = {'r', 'e'};
static const symbol s_4_4[2] = {'i', 'g'};
static const symbol s_4_5[3] = {'n', 'a', 'k'};
static const symbol s_4_6[3] = {'n', 'e', 'k'};
static const symbol s_4_7[3] = {'v', 'a', 'l'};
static const symbol s_4_8[3] = {'v', 'e', 'l'};
static const symbol s_4_9[2] = {'u', 'l'};
static const symbol s_4_10[4] = {'n', 0xC3, 0xA1, 'l'};
static const symbol s_4_11[4] = {'n', 0xC3, 0xA9, 'l'};
static const symbol s_4_12[4] = {'b', 0xC3, 0xB3, 'l'};
static const symbol s_4_13[4] = {'r', 0xC3, 0xB3, 'l'};
static const symbol s_4_14[4] = {'t', 0xC3, 0xB3, 'l'};
static const symbol s_4_15[4] = {'b', 0xC3, 0xB5, 'l'};
static const symbol s_4_16[4] = {'r', 0xC3, 0xB5, 'l'};
static const symbol s_4_17[4] = {'t', 0xC3, 0xB5, 'l'};
static const symbol s_4_18[3] = {0xC3, 0xBC, 'l'};
static const symbol s_4_19[1] = {'n'};
static const symbol s_4_20[2] = {'a', 'n'};
static const symbol s_4_21[3] = {'b', 'a', 'n'};
static const symbol s_4_22[2] = {'e', 'n'};
static const symbol s_4_23[3] = {'b', 'e', 'n'};
static const symbol s_4_24[7] = {'k', 0xC3, 0xA9, 'p', 'p', 'e', 'n'};
static const symbol s_4_25[2] = {'o', 'n'};
static const symbol s_4_26[3] = {0xC3, 0xB6, 'n'};
static const symbol s_4_27[5] = {'k', 0xC3, 0xA9, 'p', 'p'};
static const symbol s_4_28[3] = {'k', 'o', 'r'};
static const symbol s_4_29[1] = {'t'};
static const symbol s_4_30[2] = {'a', 't'};
static const symbol s_4_31[2] = {'e', 't'};
static const symbol s_4_32[5] = {'k', 0xC3, 0xA9, 'n', 't'};
static const symbol s_4_33[7] = {'a', 'n', 'k', 0xC3, 0xA9, 'n', 't'};
static const symbol s_4_34[7] = {'e', 'n', 'k', 0xC3, 0xA9, 'n', 't'};
static const symbol s_4_35[7] = {'o', 'n', 'k', 0xC3, 0xA9, 'n', 't'};
static const symbol s_4_36[2] = {'o', 't'};
static const symbol s_4_37[4] = {0xC3, 0xA9, 'r', 't'};
static const symbol s_4_38[3] = {0xC3, 0xB6, 't'};
static const symbol s_4_39[3] = {'h', 'e', 'z'};
static const symbol s_4_40[3] = {'h', 'o', 'z'};
static const symbol s_4_41[4] = {'h', 0xC3, 0xB6, 'z'};
static const symbol s_4_42[3] = {'v', 0xC3, 0xA1};
static const symbol s_4_43[3] = {'v', 0xC3, 0xA9};

static const struct among a_4[44] = {
    /*  0 */ {2, s_4_0, -1, -1, 0},
    /*  1 */ {2, s_4_1, -1, -1, 0},
    /*  2 */ {2, s_4_2, -1, -1, 0},
    /*  3 */ {2, s_4_3, -1, -1, 0},
    /*  4 */ {2, s_4_4, -1, -1, 0},
    /*  5 */ {3, s_4_5, -1, -1, 0},
    /*  6 */ {3, s_4_6, -1, -1, 0},
    /*  7 */ {3, s_4_7, -1, -1, 0},
    /*  8 */ {3, s_4_8, -1, -1, 0},
    /*  9 */ {2, s_4_9, -1, -1, 0},
    /* 10 */ {4, s_4_10, -1, -1, 0},
    /* 11 */ {4, s_4_11, -1, -1, 0},
    /* 12 */ {4, s_4_12, -1, -1, 0},
    /* 13 */ {4, s_4_13, -1, -1, 0},
    /* 14 */ {4, s_4_14, -1, -1, 0},
    /* 15 */ {4, s_4_15, -1, -1, 0},
    /* 16 */ {4, s_4_16, -1, -1, 0},
    /* 17 */ {4, s_4_17, -1, -1, 0},
    /* 18 */ {3, s_4_18, -1, -1, 0},
    /* 19 */ {1, s_4_19, -1, -1, 0},
    /* 20 */ {2, s_4_20, 19, -1, 0},
    /* 21 */ {3, s_4_21, 20, -1, 0},
    /* 22 */ {2, s_4_22, 19, -1, 0},
    /* 23 */ {3, s_4_23, 22, -1, 0},
    /* 24 */ {7, s_4_24, 22, -1, 0},
    /* 25 */ {2, s_4_25, 19, -1, 0},
    /* 26 */ {3, s_4_26, 19, -1, 0},
    /* 27 */ {5, s_4_27, -1, -1, 0},
    /* 28 */ {3, s_4_28, -1, -1, 0},
    /* 29 */ {1, s_4_29, -1, -1, 0},
    /* 30 */ {2, s_4_30, 29, -1, 0},
    /* 31 */ {2, s_4_31, 29, -1, 0},
    /* 32 */ {5, s_4_32, 29, -1, 0},
    /* 33 */ {7, s_4_33, 32, -1, 0},
    /* 34 */ {7, s_4_34, 32, -1, 0},
    /* 35 */ {7, s_4_35, 32, -1, 0},
    /* 36 */ {2, s_4_36, 29, -1, 0},
    /* 37 */ {4, s_4_37, 29, -1, 0},
    /* 38 */ {3, s_4_38, 29, -1, 0},
    /* 39 */ {3, s_4_39, -1, -1, 0},
    /* 40 */ {3, s_4_40, -1, -1, 0},
    /* 41 */ {4, s_4_41, -1, -1, 0},
    /* 42 */ {3, s_4_42, -1, -1, 0},
    /* 43 */ {3, s_4_43, -1, -1, 0}};

static const symbol s_5_0[3] = {0xC3, 0xA1, 'n'};
static const symbol s_5_1[3] = {0xC3, 0xA9, 'n'};
static const symbol s_5_2[8] = {0xC3, 0xA1, 'n', 'k', 0xC3, 0xA9, 'n', 't'};

static const struct among a_5[3] = {
    /*  0 */ {3, s_5_0, -1, 2, 0},
    /*  1 */ {3, s_5_1, -1, 1, 0},
    /*  2 */ {8, s_5_2, -1, 3, 0}};

static const symbol s_6_0[4] = {'s', 't', 'u', 'l'};
static const symbol s_6_1[5] = {'a', 's', 't', 'u', 'l'};
static const symbol s_6_2[6] = {0xC3, 0xA1, 's', 't', 'u', 'l'};
static const symbol s_6_3[5] = {'s', 't', 0xC3, 0xBC, 'l'};
static const symbol s_6_4[6] = {'e', 's', 't', 0xC3, 0xBC, 'l'};
static const symbol s_6_5[7] = {0xC3, 0xA9, 's', 't', 0xC3, 0xBC, 'l'};

static const struct among a_6[6] = {
    /*  0 */ {4, s_6_0, -1, 2, 0},
    /*  1 */ {5, s_6_1, 0, 1, 0},
    /*  2 */ {6, s_6_2, 0, 3, 0},
    /*  3 */ {5, s_6_3, -1, 2, 0},
    /*  4 */ {6, s_6_4, 3, 1, 0},
    /*  5 */ {7, s_6_5, 3, 4, 0}};

static const symbol s_7_0[2] = {0xC3, 0xA1};
static const symbol s_7_1[2] = {0xC3, 0xA9};

static const struct among a_7[2] = {
    /*  0 */ {2, s_7_0, -1, 1, 0},
    /*  1 */ {2, s_7_1, -1, 2, 0}};

static const symbol s_8_0[1] = {'k'};
static const symbol s_8_1[2] = {'a', 'k'};
static const symbol s_8_2[2] = {'e', 'k'};
static const symbol s_8_3[2] = {'o', 'k'};
static const symbol s_8_4[3] = {0xC3, 0xA1, 'k'};
static const symbol s_8_5[3] = {0xC3, 0xA9, 'k'};
static const symbol s_8_6[3] = {0xC3, 0xB6, 'k'};

static const struct among a_8[7] = {
    /*  0 */ {1, s_8_0, -1, 7, 0},
    /*  1 */ {2, s_8_1, 0, 4, 0},
    /*  2 */ {2, s_8_2, 0, 6, 0},
    /*  3 */ {2, s_8_3, 0, 5, 0},
    /*  4 */ {3, s_8_4, 0, 1, 0},
    /*  5 */ {3, s_8_5, 0, 2, 0},
    /*  6 */ {3, s_8_6, 0, 3, 0}};

static const symbol s_9_0[3] = {0xC3, 0xA9, 'i'};
static const symbol s_9_1[5] = {0xC3, 0xA1, 0xC3, 0xA9, 'i'};
static const symbol s_9_2[5] = {0xC3, 0xA9, 0xC3, 0xA9, 'i'};
static const symbol s_9_3[2] = {0xC3, 0xA9};
static const symbol s_9_4[3] = {'k', 0xC3, 0xA9};
static const symbol s_9_5[4] = {'a', 'k', 0xC3, 0xA9};
static const symbol s_9_6[4] = {'e', 'k', 0xC3, 0xA9};
static const symbol s_9_7[4] = {'o', 'k', 0xC3, 0xA9};
static const symbol s_9_8[5] = {0xC3, 0xA1, 'k', 0xC3, 0xA9};
static const symbol s_9_9[5] = {0xC3, 0xA9, 'k', 0xC3, 0xA9};
static const symbol s_9_10[5] = {0xC3, 0xB6, 'k', 0xC3, 0xA9};
static const symbol s_9_11[4] = {0xC3, 0xA9, 0xC3, 0xA9};

static const struct among a_9[12] = {
    /*  0 */ {3, s_9_0, -1, 7, 0},
    /*  1 */ {5, s_9_1, 0, 6, 0},
    /*  2 */ {5, s_9_2, 0, 5, 0},
    /*  3 */ {2, s_9_3, -1, 9, 0},
    /*  4 */ {3, s_9_4, 3, 4, 0},
    /*  5 */ {4, s_9_5, 4, 1, 0},
    /*  6 */ {4, s_9_6, 4, 1, 0},
    /*  7 */ {4, s_9_7, 4, 1, 0},
    /*  8 */ {5, s_9_8, 4, 3, 0},
    /*  9 */ {5, s_9_9, 4, 2, 0},
    /* 10 */ {5, s_9_10, 4, 1, 0},
    /* 11 */ {4, s_9_11, 3, 8, 0}};

static const symbol s_10_0[1] = {'a'};
static const symbol s_10_1[2] = {'j', 'a'};
static const symbol s_10_2[1] = {'d'};
static const symbol s_10_3[2] = {'a', 'd'};
static const symbol s_10_4[2] = {'e', 'd'};
static const symbol s_10_5[2] = {'o', 'd'};
static const symbol s_10_6[3] = {0xC3, 0xA1, 'd'};
static const symbol s_10_7[3] = {0xC3, 0xA9, 'd'};
static const symbol s_10_8[3] = {0xC3, 0xB6, 'd'};
static const symbol s_10_9[1] = {'e'};
static const symbol s_10_10[2] = {'j', 'e'};
static const symbol s_10_11[2] = {'n', 'k'};
static const symbol s_10_12[3] = {'u', 'n', 'k'};
static const symbol s_10_13[4] = {0xC3, 0xA1, 'n', 'k'};
static const symbol s_10_14[4] = {0xC3, 0xA9, 'n', 'k'};
static const symbol s_10_15[4] = {0xC3, 0xBC, 'n', 'k'};
static const symbol s_10_16[2] = {'u', 'k'};
static const symbol s_10_17[3] = {'j', 'u', 'k'};
static const symbol s_10_18[5] = {0xC3, 0xA1, 'j', 'u', 'k'};
static const symbol s_10_19[3] = {0xC3, 0xBC, 'k'};
static const symbol s_10_20[4] = {'j', 0xC3, 0xBC, 'k'};
static const symbol s_10_21[6] = {0xC3, 0xA9, 'j', 0xC3, 0xBC, 'k'};
static const symbol s_10_22[1] = {'m'};
static const symbol s_10_23[2] = {'a', 'm'};
static const symbol s_10_24[2] = {'e', 'm'};
static const symbol s_10_25[2] = {'o', 'm'};
static const symbol s_10_26[3] = {0xC3, 0xA1, 'm'};
static const symbol s_10_27[3] = {0xC3, 0xA9, 'm'};
static const symbol s_10_28[1] = {'o'};
static const symbol s_10_29[2] = {0xC3, 0xA1};
static const symbol s_10_30[2] = {0xC3, 0xA9};

static const struct among a_10[31] = {
    /*  0 */ {1, s_10_0, -1, 18, 0},
    /*  1 */ {2, s_10_1, 0, 17, 0},
    /*  2 */ {1, s_10_2, -1, 16, 0},
    /*  3 */ {2, s_10_3, 2, 13, 0},
    /*  4 */ {2, s_10_4, 2, 13, 0},
    /*  5 */ {2, s_10_5, 2, 13, 0},
    /*  6 */ {3, s_10_6, 2, 14, 0},
    /*  7 */ {3, s_10_7, 2, 15, 0},
    /*  8 */ {3, s_10_8, 2, 13, 0},
    /*  9 */ {1, s_10_9, -1, 18, 0},
    /* 10 */ {2, s_10_10, 9, 17, 0},
    /* 11 */ {2, s_10_11, -1, 4, 0},
    /* 12 */ {3, s_10_12, 11, 1, 0},
    /* 13 */ {4, s_10_13, 11, 2, 0},
    /* 14 */ {4, s_10_14, 11, 3, 0},
    /* 15 */ {4, s_10_15, 11, 1, 0},
    /* 16 */ {2, s_10_16, -1, 8, 0},
    /* 17 */ {3, s_10_17, 16, 7, 0},
    /* 18 */ {5, s_10_18, 17, 5, 0},
    /* 19 */ {3, s_10_19, -1, 8, 0},
    /* 20 */ {4, s_10_20, 19, 7, 0},
    /* 21 */ {6, s_10_21, 20, 6, 0},
    /* 22 */ {1, s_10_22, -1, 12, 0},
    /* 23 */ {2, s_10_23, 22, 9, 0},
    /* 24 */ {2, s_10_24, 22, 9, 0},
    /* 25 */ {2, s_10_25, 22, 9, 0},
    /* 26 */ {3, s_10_26, 22, 10, 0},
    /* 27 */ {3, s_10_27, 22, 11, 0},
    /* 28 */ {1, s_10_28, -1, 18, 0},
    /* 29 */ {2, s_10_29, -1, 19, 0},
    /* 30 */ {2, s_10_30, -1, 20, 0}};

static const symbol s_11_0[2] = {'i', 'd'};
static const symbol s_11_1[3] = {'a', 'i', 'd'};
static const symbol s_11_2[4] = {'j', 'a', 'i', 'd'};
static const symbol s_11_3[3] = {'e', 'i', 'd'};
static const symbol s_11_4[4] = {'j', 'e', 'i', 'd'};
static const symbol s_11_5[4] = {0xC3, 0xA1, 'i', 'd'};
static const symbol s_11_6[4] = {0xC3, 0xA9, 'i', 'd'};
static const symbol s_11_7[1] = {'i'};
static const symbol s_11_8[2] = {'a', 'i'};
static const symbol s_11_9[3] = {'j', 'a', 'i'};
static const symbol s_11_10[2] = {'e', 'i'};
static const symbol s_11_11[3] = {'j', 'e', 'i'};
static const symbol s_11_12[3] = {0xC3, 0xA1, 'i'};
static const symbol s_11_13[3] = {0xC3, 0xA9, 'i'};
static const symbol s_11_14[4] = {'i', 't', 'e', 'k'};
static const symbol s_11_15[5] = {'e', 'i', 't', 'e', 'k'};
static const symbol s_11_16[6] = {'j', 'e', 'i', 't', 'e', 'k'};
static const symbol s_11_17[6] = {0xC3, 0xA9, 'i', 't', 'e', 'k'};
static const symbol s_11_18[2] = {'i', 'k'};
static const symbol s_11_19[3] = {'a', 'i', 'k'};
static const symbol s_11_20[4] = {'j', 'a', 'i', 'k'};
static const symbol s_11_21[3] = {'e', 'i', 'k'};
static const symbol s_11_22[4] = {'j', 'e', 'i', 'k'};
static const symbol s_11_23[4] = {0xC3, 0xA1, 'i', 'k'};
static const symbol s_11_24[4] = {0xC3, 0xA9, 'i', 'k'};
static const symbol s_11_25[3] = {'i', 'n', 'k'};
static const symbol s_11_26[4] = {'a', 'i', 'n', 'k'};
static const symbol s_11_27[5] = {'j', 'a', 'i', 'n', 'k'};
static const symbol s_11_28[4] = {'e', 'i', 'n', 'k'};
static const symbol s_11_29[5] = {'j', 'e', 'i', 'n', 'k'};
static const symbol s_11_30[5] = {0xC3, 0xA1, 'i', 'n', 'k'};
static const symbol s_11_31[5] = {0xC3, 0xA9, 'i', 'n', 'k'};
static const symbol s_11_32[5] = {'a', 'i', 't', 'o', 'k'};
static const symbol s_11_33[6] = {'j', 'a', 'i', 't', 'o', 'k'};
static const symbol s_11_34[6] = {0xC3, 0xA1, 'i', 't', 'o', 'k'};
static const symbol s_11_35[2] = {'i', 'm'};
static const symbol s_11_36[3] = {'a', 'i', 'm'};
static const symbol s_11_37[4] = {'j', 'a', 'i', 'm'};
static const symbol s_11_38[3] = {'e', 'i', 'm'};
static const symbol s_11_39[4] = {'j', 'e', 'i', 'm'};
static const symbol s_11_40[4] = {0xC3, 0xA1, 'i', 'm'};
static const symbol s_11_41[4] = {0xC3, 0xA9, 'i', 'm'};

static const struct among a_11[42] = {
    /*  0 */ {2, s_11_0, -1, 10, 0},
    /*  1 */ {3, s_11_1, 0, 9, 0},
    /*  2 */ {4, s_11_2, 1, 6, 0},
    /*  3 */ {3, s_11_3, 0, 9, 0},
    /*  4 */ {4, s_11_4, 3, 6, 0},
    /*  5 */ {4, s_11_5, 0, 7, 0},
    /*  6 */ {4, s_11_6, 0, 8, 0},
    /*  7 */ {1, s_11_7, -1, 15, 0},
    /*  8 */ {2, s_11_8, 7, 14, 0},
    /*  9 */ {3, s_11_9, 8, 11, 0},
    /* 10 */ {2, s_11_10, 7, 14, 0},
    /* 11 */ {3, s_11_11, 10, 11, 0},
    /* 12 */ {3, s_11_12, 7, 12, 0},
    /* 13 */ {3, s_11_13, 7, 13, 0},
    /* 14 */ {4, s_11_14, -1, 24, 0},
    /* 15 */ {5, s_11_15, 14, 21, 0},
    /* 16 */ {6, s_11_16, 15, 20, 0},
    /* 17 */ {6, s_11_17, 14, 23, 0},
    /* 18 */ {2, s_11_18, -1, 29, 0},
    /* 19 */ {3, s_11_19, 18, 26, 0},
    /* 20 */ {4, s_11_20, 19, 25, 0},
    /* 21 */ {3, s_11_21, 18, 26, 0},
    /* 22 */ {4, s_11_22, 21, 25, 0},
    /* 23 */ {4, s_11_23, 18, 27, 0},
    /* 24 */ {4, s_11_24, 18, 28, 0},
    /* 25 */ {3, s_11_25, -1, 20, 0},
    /* 26 */ {4, s_11_26, 25, 17, 0},
    /* 27 */ {5, s_11_27, 26, 16, 0},
    /* 28 */ {4, s_11_28, 25, 17, 0},
    /* 29 */ {5, s_11_29, 28, 16, 0},
    /* 30 */ {5, s_11_30, 25, 18, 0},
    /* 31 */ {5, s_11_31, 25, 19, 0},
    /* 32 */ {5, s_11_32, -1, 21, 0},
    /* 33 */ {6, s_11_33, 32, 20, 0},
    /* 34 */ {6, s_11_34, -1, 22, 0},
    /* 35 */ {2, s_11_35, -1, 5, 0},
    /* 36 */ {3, s_11_36, 35, 4, 0},
    /* 37 */ {4, s_11_37, 36, 1, 0},
    /* 38 */ {3, s_11_38, 35, 4, 0},
    /* 39 */ {4, s_11_39, 38, 1, 0},
    /* 40 */ {4, s_11_40, 35, 2, 0},
    /* 41 */ {4, s_11_41, 35, 3, 0}};

static const unsigned char g_v[] = {17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 17, 52, 14};

static const symbol s_0[] = {'a'};
static const symbol s_1[] = {'e'};
static const symbol s_2[] = {'e'};
static const symbol s_3[] = {'a'};
static const symbol s_4[] = {'a'};
static const symbol s_5[] = {'a'};
static const symbol s_6[] = {'e'};
static const symbol s_7[] = {'a'};
static const symbol s_8[] = {'e'};
static const symbol s_9[] = {'e'};
static const symbol s_10[] = {'a'};
static const symbol s_11[] = {'e'};
static const symbol s_12[] = {'a'};
static const symbol s_13[] = {'e'};
static const symbol s_14[] = {'a'};
static const symbol s_15[] = {'e'};
static const symbol s_16[] = {'a'};
static const symbol s_17[] = {'e'};
static const symbol s_18[] = {'a'};
static const symbol s_19[] = {'e'};
static const symbol s_20[] = {'a'};
static const symbol s_21[] = {'e'};
static const symbol s_22[] = {'a'};
static const symbol s_23[] = {'e'};
static const symbol s_24[] = {'a'};
static const symbol s_25[] = {'e'};
static const symbol s_26[] = {'a'};
static const symbol s_27[] = {'e'};
static const symbol s_28[] = {'a'};
static const symbol s_29[] = {'e'};
static const symbol s_30[] = {'a'};
static const symbol s_31[] = {'e'};
static const symbol s_32[] = {'a'};
static const symbol s_33[] = {'e'};
static const symbol s_34[] = {'a'};
static const symbol s_35[] = {'e'};

static int r_mark_regions(struct SN_env *z) {
    z->I[0] = z->l;
    {
        int c1 = z->c; /* or, line 51 */
        if (in_grouping_U(z, g_v, 97, 252, 0))
            goto lab1;
        if (in_grouping_U(z, g_v, 97, 252, 1) < 0)
            goto lab1; /* goto */ /* non v, line 48 */
        {
            int c2 = z->c; /* or, line 49 */
            if (z->c + 1 >= z->l || z->p[z->c + 1] >> 5 != 3 || !((101187584 >> (z->p[z->c + 1] & 0x1f)) & 1))
                goto lab3;
            if (!(find_among(z, a_0, 8)))
                goto lab3; /* among, line 49 */
            goto lab2;
        lab3:
            z->c = c2;
            {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    goto lab1;
                z->c = ret; /* next, line 49 */
            }
        }
    lab2:
        z->I[0] = z->c; /* setmark p1, line 50 */
        goto lab0;
    lab1:
        z->c = c1;
        if (out_grouping_U(z, g_v, 97, 252, 0))
            return 0;
        { /* gopast */ /* grouping v, line 53 */
            int ret = out_grouping_U(z, g_v, 97, 252, 1);
            if (ret < 0)
                return 0;
            z->c += ret;
        }
        z->I[0] = z->c; /* setmark p1, line 53 */
    }
lab0:
    return 1;
}

static int r_R1(struct SN_env *z) {
    if (!(z->I[0] <= z->c))
        return 0;
    return 1;
}

static int r_v_ending(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 61 */
    if (z->c - 1 <= z->lb || (z->p[z->c - 1] != 161 && z->p[z->c - 1] != 169))
        return 0;
    among_var = find_among_b(z, a_1, 2); /* substring, line 61 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 61 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 61 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_from_s(z, 1, s_0); /* <-, line 62 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_from_s(z, 1, s_1); /* <-, line 63 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_double(struct SN_env *z) {
    {
        int m_test = z->l - z->c; /* test, line 68 */
        if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((106790108 >> (z->p[z->c - 1] & 0x1f)) & 1))
            return 0;
        if (!(find_among_b(z, a_2, 23)))
            return 0; /* among, line 68 */
        z->c = z->l - m_test;
    }
    return 1;
}

static int r_undouble(struct SN_env *z) {
    {
        int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
        if (ret < 0)
            return 0;
        z->c = ret; /* next, line 73 */
    }
    z->ket = z->c; /* [, line 73 */
    {
        int ret = skip_utf8(z->p, z->c, z->lb, z->l, -1);
        if (ret < 0)
            return 0;
        z->c = ret; /* hop, line 73 */
    }
    z->bra = z->c; /* ], line 73 */
    {
        int ret = slice_del(z); /* delete, line 73 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_instrum(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 77 */
    if (z->c - 1 <= z->lb || z->p[z->c - 1] != 108)
        return 0;
    among_var = find_among_b(z, a_3, 2); /* substring, line 77 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 77 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 77 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = r_double(z);
            if (ret == 0)
                return 0; /* call double, line 78 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = r_double(z);
            if (ret == 0)
                return 0; /* call double, line 79 */
            if (ret < 0)
                return ret;
        } break;
    }
    {
        int ret = slice_del(z); /* delete, line 81 */
        if (ret < 0)
            return ret;
    }
    {
        int ret = r_undouble(z);
        if (ret == 0)
            return 0; /* call undouble, line 82 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_case(struct SN_env *z) {
    z->ket = z->c; /* [, line 87 */
    if (!(find_among_b(z, a_4, 44)))
        return 0;  /* substring, line 87 */
    z->bra = z->c; /* ], line 87 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 87 */
        if (ret < 0)
            return ret;
    }
    {
        int ret = slice_del(z); /* delete, line 111 */
        if (ret < 0)
            return ret;
    }
    {
        int ret = r_v_ending(z);
        if (ret == 0)
            return 0; /* call v_ending, line 112 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_case_special(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 116 */
    if (z->c - 2 <= z->lb || (z->p[z->c - 1] != 110 && z->p[z->c - 1] != 116))
        return 0;
    among_var = find_among_b(z, a_5, 3); /* substring, line 116 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 116 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 116 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_from_s(z, 1, s_2); /* <-, line 117 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_from_s(z, 1, s_3); /* <-, line 118 */
            if (ret < 0)
                return ret;
        } break;
        case 3: {
            int ret = slice_from_s(z, 1, s_4); /* <-, line 119 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_case_other(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 124 */
    if (z->c - 3 <= z->lb || z->p[z->c - 1] != 108)
        return 0;
    among_var = find_among_b(z, a_6, 6); /* substring, line 124 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 124 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 124 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 125 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_del(z); /* delete, line 126 */
            if (ret < 0)
                return ret;
        } break;
        case 3: {
            int ret = slice_from_s(z, 1, s_5); /* <-, line 127 */
            if (ret < 0)
                return ret;
        } break;
        case 4: {
            int ret = slice_from_s(z, 1, s_6); /* <-, line 128 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_factive(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 133 */
    if (z->c - 1 <= z->lb || (z->p[z->c - 1] != 161 && z->p[z->c - 1] != 169))
        return 0;
    among_var = find_among_b(z, a_7, 2); /* substring, line 133 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 133 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 133 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = r_double(z);
            if (ret == 0)
                return 0; /* call double, line 134 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = r_double(z);
            if (ret == 0)
                return 0; /* call double, line 135 */
            if (ret < 0)
                return ret;
        } break;
    }
    {
        int ret = slice_del(z); /* delete, line 137 */
        if (ret < 0)
            return ret;
    }
    {
        int ret = r_undouble(z);
        if (ret == 0)
            return 0; /* call undouble, line 138 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_plural(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 142 */
    if (z->c <= z->lb || z->p[z->c - 1] != 107)
        return 0;
    among_var = find_among_b(z, a_8, 7); /* substring, line 142 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 142 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 142 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_from_s(z, 1, s_7); /* <-, line 143 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_from_s(z, 1, s_8); /* <-, line 144 */
            if (ret < 0)
                return ret;
        } break;
        case 3: {
            int ret = slice_del(z); /* delete, line 145 */
            if (ret < 0)
                return ret;
        } break;
        case 4: {
            int ret = slice_del(z); /* delete, line 146 */
            if (ret < 0)
                return ret;
        } break;
        case 5: {
            int ret = slice_del(z); /* delete, line 147 */
            if (ret < 0)
                return ret;
        } break;
        case 6: {
            int ret = slice_del(z); /* delete, line 148 */
            if (ret < 0)
                return ret;
        } break;
        case 7: {
            int ret = slice_del(z); /* delete, line 149 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_owned(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 154 */
    if (z->c - 1 <= z->lb || (z->p[z->c - 1] != 105 && z->p[z->c - 1] != 169))
        return 0;
    among_var = find_among_b(z, a_9, 12); /* substring, line 154 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 154 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 154 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 155 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_from_s(z, 1, s_9); /* <-, line 156 */
            if (ret < 0)
                return ret;
        } break;
        case 3: {
            int ret = slice_from_s(z, 1, s_10); /* <-, line 157 */
            if (ret < 0)
                return ret;
        } break;
        case 4: {
            int ret = slice_del(z); /* delete, line 158 */
            if (ret < 0)
                return ret;
        } break;
        case 5: {
            int ret = slice_from_s(z, 1, s_11); /* <-, line 159 */
            if (ret < 0)
                return ret;
        } break;
        case 6: {
            int ret = slice_from_s(z, 1, s_12); /* <-, line 160 */
            if (ret < 0)
                return ret;
        } break;
        case 7: {
            int ret = slice_del(z); /* delete, line 161 */
            if (ret < 0)
                return ret;
        } break;
        case 8: {
            int ret = slice_from_s(z, 1, s_13); /* <-, line 162 */
            if (ret < 0)
                return ret;
        } break;
        case 9: {
            int ret = slice_del(z); /* delete, line 163 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_sing_owner(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                         /* [, line 168 */
    among_var = find_among_b(z, a_10, 31); /* substring, line 168 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 168 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 168 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 169 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_from_s(z, 1, s_14); /* <-, line 170 */
            if (ret < 0)
                return ret;
        } break;
        case 3: {
            int ret = slice_from_s(z, 1, s_15); /* <-, line 171 */
            if (ret < 0)
                return ret;
        } break;
        case 4: {
            int ret = slice_del(z); /* delete, line 172 */
            if (ret < 0)
                return ret;
        } break;
        case 5: {
            int ret = slice_from_s(z, 1, s_16); /* <-, line 173 */
            if (ret < 0)
                return ret;
        } break;
        case 6: {
            int ret = slice_from_s(z, 1, s_17); /* <-, line 174 */
            if (ret < 0)
                return ret;
        } break;
        case 7: {
            int ret = slice_del(z); /* delete, line 175 */
            if (ret < 0)
                return ret;
        } break;
        case 8: {
            int ret = slice_del(z); /* delete, line 176 */
            if (ret < 0)
                return ret;
        } break;
        case 9: {
            int ret = slice_del(z); /* delete, line 177 */
            if (ret < 0)
                return ret;
        } break;
        case 10: {
            int ret = slice_from_s(z, 1, s_18); /* <-, line 178 */
            if (ret < 0)
                return ret;
        } break;
        case 11: {
            int ret = slice_from_s(z, 1, s_19); /* <-, line 179 */
            if (ret < 0)
                return ret;
        } break;
        case 12: {
            int ret = slice_del(z); /* delete, line 180 */
            if (ret < 0)
                return ret;
        } break;
        case 13: {
            int ret = slice_del(z); /* delete, line 181 */
            if (ret < 0)
                return ret;
        } break;
        case 14: {
            int ret = slice_from_s(z, 1, s_20); /* <-, line 182 */
            if (ret < 0)
                return ret;
        } break;
        case 15: {
            int ret = slice_from_s(z, 1, s_21); /* <-, line 183 */
            if (ret < 0)
                return ret;
        } break;
        case 16: {
            int ret = slice_del(z); /* delete, line 184 */
            if (ret < 0)
                return ret;
        } break;
        case 17: {
            int ret = slice_del(z); /* delete, line 185 */
            if (ret < 0)
                return ret;
        } break;
        case 18: {
            int ret = slice_del(z); /* delete, line 186 */
            if (ret < 0)
                return ret;
        } break;
        case 19: {
            int ret = slice_from_s(z, 1, s_22); /* <-, line 187 */
            if (ret < 0)
                return ret;
        } break;
        case 20: {
            int ret = slice_from_s(z, 1, s_23); /* <-, line 188 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_plur_owner(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 193 */
    if (z->c <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((10768 >> (z->p[z->c - 1] & 0x1f)) & 1))
        return 0;
    among_var = find_among_b(z, a_11, 42); /* substring, line 193 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 193 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 193 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 194 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_from_s(z, 1, s_24); /* <-, line 195 */
            if (ret < 0)
                return ret;
        } break;
        case 3: {
            int ret = slice_from_s(z, 1, s_25); /* <-, line 196 */
            if (ret < 0)
                return ret;
        } break;
        case 4: {
            int ret = slice_del(z); /* delete, line 197 */
            if (ret < 0)
                return ret;
        } break;
        case 5: {
            int ret = slice_del(z); /* delete, line 198 */
            if (ret < 0)
                return ret;
        } break;
        case 6: {
            int ret = slice_del(z); /* delete, line 199 */
            if (ret < 0)
                return ret;
        } break;
        case 7: {
            int ret = slice_from_s(z, 1, s_26); /* <-, line 200 */
            if (ret < 0)
                return ret;
        } break;
        case 8: {
            int ret = slice_from_s(z, 1, s_27); /* <-, line 201 */
            if (ret < 0)
                return ret;
        } break;
        case 9: {
            int ret = slice_del(z); /* delete, line 202 */
            if (ret < 0)
                return ret;
        } break;
        case 10: {
            int ret = slice_del(z); /* delete, line 203 */
            if (ret < 0)
                return ret;
        } break;
        case 11: {
            int ret = slice_del(z); /* delete, line 204 */
            if (ret < 0)
                return ret;
        } break;
        case 12: {
            int ret = slice_from_s(z, 1, s_28); /* <-, line 205 */
            if (ret < 0)
                return ret;
        } break;
        case 13: {
            int ret = slice_from_s(z, 1, s_29); /* <-, line 206 */
            if (ret < 0)
                return ret;
        } break;
        case 14: {
            int ret = slice_del(z); /* delete, line 207 */
            if (ret < 0)
                return ret;
        } break;
        case 15: {
            int ret = slice_del(z); /* delete, line 208 */
            if (ret < 0)
                return ret;
        } break;
        case 16: {
            int ret = slice_del(z); /* delete, line 209 */
            if (ret < 0)
                return ret;
        } break;
        case 17: {
            int ret = slice_del(z); /* delete, line 210 */
            if (ret < 0)
                return ret;
        } break;
        case 18: {
            int ret = slice_from_s(z, 1, s_30); /* <-, line 211 */
            if (ret < 0)
                return ret;
        } break;
        case 19: {
            int ret = slice_from_s(z, 1, s_31); /* <-, line 212 */
            if (ret < 0)
                return ret;
        } break;
        case 20: {
            int ret = slice_del(z); /* delete, line 214 */
            if (ret < 0)
                return ret;
        } break;
        case 21: {
            int ret = slice_del(z); /* delete, line 215 */
            if (ret < 0)
                return ret;
        } break;
        case 22: {
            int ret = slice_from_s(z, 1, s_32); /* <-, line 216 */
            if (ret < 0)
                return ret;
        } break;
        case 23: {
            int ret = slice_from_s(z, 1, s_33); /* <-, line 217 */
            if (ret < 0)
                return ret;
        } break;
        case 24: {
            int ret = slice_del(z); /* delete, line 218 */
            if (ret < 0)
                return ret;
        } break;
        case 25: {
            int ret = slice_del(z); /* delete, line 219 */
            if (ret < 0)
                return ret;
        } break;
        case 26: {
            int ret = slice_del(z); /* delete, line 220 */
            if (ret < 0)
                return ret;
        } break;
        case 27: {
            int ret = slice_from_s(z, 1, s_34); /* <-, line 221 */
            if (ret < 0)
                return ret;
        } break;
        case 28: {
            int ret = slice_from_s(z, 1, s_35); /* <-, line 222 */
            if (ret < 0)
                return ret;
        } break;
        case 29: {
            int ret = slice_del(z); /* delete, line 223 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

extern int hungarian_UTF_8_stem(struct SN_env *z) {
    {
        int c1 = z->c; /* do, line 229 */
        {
            int ret = r_mark_regions(z);
            if (ret == 0)
                goto lab0; /* call mark_regions, line 229 */
            if (ret < 0)
                return ret;
        }
    lab0:
        z->c = c1;
    }
    z->lb = z->c;
    z->c = z->l; /* backwards, line 230 */

    {
        int m2 = z->l - z->c;
        (void)m2; /* do, line 231 */
        {
            int ret = r_instrum(z);
            if (ret == 0)
                goto lab1; /* call instrum, line 231 */
            if (ret < 0)
                return ret;
        }
    lab1:
        z->c = z->l - m2;
    }
    {
        int m3 = z->l - z->c;
        (void)m3; /* do, line 232 */
        {
            int ret = r_case(z);
            if (ret == 0)
                goto lab2; /* call case, line 232 */
            if (ret < 0)
                return ret;
        }
    lab2:
        z->c = z->l - m3;
    }
    {
        int m4 = z->l - z->c;
        (void)m4; /* do, line 233 */
        {
            int ret = r_case_special(z);
            if (ret == 0)
                goto lab3; /* call case_special, line 233 */
            if (ret < 0)
                return ret;
        }
    lab3:
        z->c = z->l - m4;
    }
    {
        int m5 = z->l - z->c;
        (void)m5; /* do, line 234 */
        {
            int ret = r_case_other(z);
            if (ret == 0)
                goto lab4; /* call case_other, line 234 */
            if (ret < 0)
                return ret;
        }
    lab4:
        z->c = z->l - m5;
    }
    {
        int m6 = z->l - z->c;
        (void)m6; /* do, line 235 */
        {
            int ret = r_factive(z);
            if (ret == 0)
                goto lab5; /* call factive, line 235 */
            if (ret < 0)
                return ret;
        }
    lab5:
        z->c = z->l - m6;
    }
    {
        int m7 = z->l - z->c;
        (void)m7; /* do, line 236 */
        {
            int ret = r_owned(z);
            if (ret == 0)
                goto lab6; /* call owned, line 236 */
            if (ret < 0)
                return ret;
        }
    lab6:
        z->c = z->l - m7;
    }
    {
        int m8 = z->l - z->c;
        (void)m8; /* do, line 237 */
        {
            int ret = r_sing_owner(z);
            if (ret == 0)
                goto lab7; /* call sing_owner, line 237 */
            if (ret < 0)
                return ret;
        }
    lab7:
        z->c = z->l - m8;
    }
    {
        int m9 = z->l - z->c;
        (void)m9; /* do, line 238 */
        {
            int ret = r_plur_owner(z);
            if (ret == 0)
                goto lab8; /* call plur_owner, line 238 */
            if (ret < 0)
                return ret;
        }
    lab8:
        z->c = z->l - m9;
    }
    {
        int m10 = z->l - z->c;
        (void)m10; /* do, line 239 */
        {
            int ret = r_plural(z);
            if (ret == 0)
                goto lab9; /* call plural, line 239 */
            if (ret < 0)
                return ret;
        }
    lab9:
        z->c = z->l - m10;
    }
    z->c = z->lb;
    return 1;
}

extern struct SN_env *hungarian_UTF_8_create_env(void) { return SN_create_env(0, 1, 0); }

extern void hungarian_UTF_8_close_env(struct SN_env *z) { SN_close_env(z, 0); }
