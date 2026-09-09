
/* This file was generated automatically by the Snowball to ANSI C compiler */

#include "header.h"

#ifdef __cplusplus
extern "C" {
#endif
extern int spanish_UTF_8_stem(struct SN_env *z);
#ifdef __cplusplus
}
#endif
static int r_residual_suffix(struct SN_env *z);
static int r_verb_suffix(struct SN_env *z);
static int r_y_verb_suffix(struct SN_env *z);
static int r_standard_suffix(struct SN_env *z);
static int r_attached_pronoun(struct SN_env *z);
static int r_R2(struct SN_env *z);
static int r_R1(struct SN_env *z);
static int r_RV(struct SN_env *z);
static int r_mark_regions(struct SN_env *z);
static int r_postlude(struct SN_env *z);
#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *spanish_UTF_8_create_env(void);
extern void spanish_UTF_8_close_env(struct SN_env *z);

#ifdef __cplusplus
}
#endif
static const symbol s_0_1[2] = {0xC3, 0xA1};
static const symbol s_0_2[2] = {0xC3, 0xA9};
static const symbol s_0_3[2] = {0xC3, 0xAD};
static const symbol s_0_4[2] = {0xC3, 0xB3};
static const symbol s_0_5[2] = {0xC3, 0xBA};

static const struct among a_0[6] = {
    /*  0 */ {0, 0, -1, 6, 0},
    /*  1 */ {2, s_0_1, 0, 1, 0},
    /*  2 */ {2, s_0_2, 0, 2, 0},
    /*  3 */ {2, s_0_3, 0, 3, 0},
    /*  4 */ {2, s_0_4, 0, 4, 0},
    /*  5 */ {2, s_0_5, 0, 5, 0}};

static const symbol s_1_0[2] = {'l', 'a'};
static const symbol s_1_1[4] = {'s', 'e', 'l', 'a'};
static const symbol s_1_2[2] = {'l', 'e'};
static const symbol s_1_3[2] = {'m', 'e'};
static const symbol s_1_4[2] = {'s', 'e'};
static const symbol s_1_5[2] = {'l', 'o'};
static const symbol s_1_6[4] = {'s', 'e', 'l', 'o'};
static const symbol s_1_7[3] = {'l', 'a', 's'};
static const symbol s_1_8[5] = {'s', 'e', 'l', 'a', 's'};
static const symbol s_1_9[3] = {'l', 'e', 's'};
static const symbol s_1_10[3] = {'l', 'o', 's'};
static const symbol s_1_11[5] = {'s', 'e', 'l', 'o', 's'};
static const symbol s_1_12[3] = {'n', 'o', 's'};

static const struct among a_1[13] = {
    /*  0 */ {2, s_1_0, -1, -1, 0},
    /*  1 */ {4, s_1_1, 0, -1, 0},
    /*  2 */ {2, s_1_2, -1, -1, 0},
    /*  3 */ {2, s_1_3, -1, -1, 0},
    /*  4 */ {2, s_1_4, -1, -1, 0},
    /*  5 */ {2, s_1_5, -1, -1, 0},
    /*  6 */ {4, s_1_6, 5, -1, 0},
    /*  7 */ {3, s_1_7, -1, -1, 0},
    /*  8 */ {5, s_1_8, 7, -1, 0},
    /*  9 */ {3, s_1_9, -1, -1, 0},
    /* 10 */ {3, s_1_10, -1, -1, 0},
    /* 11 */ {5, s_1_11, 10, -1, 0},
    /* 12 */ {3, s_1_12, -1, -1, 0}};

static const symbol s_2_0[4] = {'a', 'n', 'd', 'o'};
static const symbol s_2_1[5] = {'i', 'e', 'n', 'd', 'o'};
static const symbol s_2_2[5] = {'y', 'e', 'n', 'd', 'o'};
static const symbol s_2_3[5] = {0xC3, 0xA1, 'n', 'd', 'o'};
static const symbol s_2_4[6] = {'i', 0xC3, 0xA9, 'n', 'd', 'o'};
static const symbol s_2_5[2] = {'a', 'r'};
static const symbol s_2_6[2] = {'e', 'r'};
static const symbol s_2_7[2] = {'i', 'r'};
static const symbol s_2_8[3] = {0xC3, 0xA1, 'r'};
static const symbol s_2_9[3] = {0xC3, 0xA9, 'r'};
static const symbol s_2_10[3] = {0xC3, 0xAD, 'r'};

static const struct among a_2[11] = {
    /*  0 */ {4, s_2_0, -1, 6, 0},
    /*  1 */ {5, s_2_1, -1, 6, 0},
    /*  2 */ {5, s_2_2, -1, 7, 0},
    /*  3 */ {5, s_2_3, -1, 2, 0},
    /*  4 */ {6, s_2_4, -1, 1, 0},
    /*  5 */ {2, s_2_5, -1, 6, 0},
    /*  6 */ {2, s_2_6, -1, 6, 0},
    /*  7 */ {2, s_2_7, -1, 6, 0},
    /*  8 */ {3, s_2_8, -1, 3, 0},
    /*  9 */ {3, s_2_9, -1, 4, 0},
    /* 10 */ {3, s_2_10, -1, 5, 0}};

static const symbol s_3_0[2] = {'i', 'c'};
static const symbol s_3_1[2] = {'a', 'd'};
static const symbol s_3_2[2] = {'o', 's'};
static const symbol s_3_3[2] = {'i', 'v'};

static const struct among a_3[4] = {
    /*  0 */ {2, s_3_0, -1, -1, 0},
    /*  1 */ {2, s_3_1, -1, -1, 0},
    /*  2 */ {2, s_3_2, -1, -1, 0},
    /*  3 */ {2, s_3_3, -1, 1, 0}};

static const symbol s_4_0[4] = {'a', 'b', 'l', 'e'};
static const symbol s_4_1[4] = {'i', 'b', 'l', 'e'};
static const symbol s_4_2[4] = {'a', 'n', 't', 'e'};

static const struct among a_4[3] = {
    /*  0 */ {4, s_4_0, -1, 1, 0},
    /*  1 */ {4, s_4_1, -1, 1, 0},
    /*  2 */ {4, s_4_2, -1, 1, 0}};

static const symbol s_5_0[2] = {'i', 'c'};
static const symbol s_5_1[4] = {'a', 'b', 'i', 'l'};
static const symbol s_5_2[2] = {'i', 'v'};

static const struct among a_5[3] = {
    /*  0 */ {2, s_5_0, -1, 1, 0},
    /*  1 */ {4, s_5_1, -1, 1, 0},
    /*  2 */ {2, s_5_2, -1, 1, 0}};

static const symbol s_6_0[3] = {'i', 'c', 'a'};
static const symbol s_6_1[5] = {'a', 'n', 'c', 'i', 'a'};
static const symbol s_6_2[5] = {'e', 'n', 'c', 'i', 'a'};
static const symbol s_6_3[5] = {'a', 'd', 'o', 'r', 'a'};
static const symbol s_6_4[3] = {'o', 's', 'a'};
static const symbol s_6_5[4] = {'i', 's', 't', 'a'};
static const symbol s_6_6[3] = {'i', 'v', 'a'};
static const symbol s_6_7[4] = {'a', 'n', 'z', 'a'};
static const symbol s_6_8[6] = {'l', 'o', 'g', 0xC3, 0xAD, 'a'};
static const symbol s_6_9[4] = {'i', 'd', 'a', 'd'};
static const symbol s_6_10[4] = {'a', 'b', 'l', 'e'};
static const symbol s_6_11[4] = {'i', 'b', 'l', 'e'};
static const symbol s_6_12[4] = {'a', 'n', 't', 'e'};
static const symbol s_6_13[5] = {'m', 'e', 'n', 't', 'e'};
static const symbol s_6_14[6] = {'a', 'm', 'e', 'n', 't', 'e'};
static const symbol s_6_15[6] = {'a', 'c', 'i', 0xC3, 0xB3, 'n'};
static const symbol s_6_16[6] = {'u', 'c', 'i', 0xC3, 0xB3, 'n'};
static const symbol s_6_17[3] = {'i', 'c', 'o'};
static const symbol s_6_18[4] = {'i', 's', 'm', 'o'};
static const symbol s_6_19[3] = {'o', 's', 'o'};
static const symbol s_6_20[7] = {'a', 'm', 'i', 'e', 'n', 't', 'o'};
static const symbol s_6_21[7] = {'i', 'm', 'i', 'e', 'n', 't', 'o'};
static const symbol s_6_22[3] = {'i', 'v', 'o'};
static const symbol s_6_23[4] = {'a', 'd', 'o', 'r'};
static const symbol s_6_24[4] = {'i', 'c', 'a', 's'};
static const symbol s_6_25[6] = {'a', 'n', 'c', 'i', 'a', 's'};
static const symbol s_6_26[6] = {'e', 'n', 'c', 'i', 'a', 's'};
static const symbol s_6_27[6] = {'a', 'd', 'o', 'r', 'a', 's'};
static const symbol s_6_28[4] = {'o', 's', 'a', 's'};
static const symbol s_6_29[5] = {'i', 's', 't', 'a', 's'};
static const symbol s_6_30[4] = {'i', 'v', 'a', 's'};
static const symbol s_6_31[5] = {'a', 'n', 'z', 'a', 's'};
static const symbol s_6_32[7] = {'l', 'o', 'g', 0xC3, 0xAD, 'a', 's'};
static const symbol s_6_33[6] = {'i', 'd', 'a', 'd', 'e', 's'};
static const symbol s_6_34[5] = {'a', 'b', 'l', 'e', 's'};
static const symbol s_6_35[5] = {'i', 'b', 'l', 'e', 's'};
static const symbol s_6_36[7] = {'a', 'c', 'i', 'o', 'n', 'e', 's'};
static const symbol s_6_37[7] = {'u', 'c', 'i', 'o', 'n', 'e', 's'};
static const symbol s_6_38[6] = {'a', 'd', 'o', 'r', 'e', 's'};
static const symbol s_6_39[5] = {'a', 'n', 't', 'e', 's'};
static const symbol s_6_40[4] = {'i', 'c', 'o', 's'};
static const symbol s_6_41[5] = {'i', 's', 'm', 'o', 's'};
static const symbol s_6_42[4] = {'o', 's', 'o', 's'};
static const symbol s_6_43[8] = {'a', 'm', 'i', 'e', 'n', 't', 'o', 's'};
static const symbol s_6_44[8] = {'i', 'm', 'i', 'e', 'n', 't', 'o', 's'};
static const symbol s_6_45[4] = {'i', 'v', 'o', 's'};

static const struct among a_6[46] = {
    /*  0 */ {3, s_6_0, -1, 1, 0},
    /*  1 */ {5, s_6_1, -1, 2, 0},
    /*  2 */ {5, s_6_2, -1, 5, 0},
    /*  3 */ {5, s_6_3, -1, 2, 0},
    /*  4 */ {3, s_6_4, -1, 1, 0},
    /*  5 */ {4, s_6_5, -1, 1, 0},
    /*  6 */ {3, s_6_6, -1, 9, 0},
    /*  7 */ {4, s_6_7, -1, 1, 0},
    /*  8 */ {6, s_6_8, -1, 3, 0},
    /*  9 */ {4, s_6_9, -1, 8, 0},
    /* 10 */ {4, s_6_10, -1, 1, 0},
    /* 11 */ {4, s_6_11, -1, 1, 0},
    /* 12 */ {4, s_6_12, -1, 2, 0},
    /* 13 */ {5, s_6_13, -1, 7, 0},
    /* 14 */ {6, s_6_14, 13, 6, 0},
    /* 15 */ {6, s_6_15, -1, 2, 0},
    /* 16 */ {6, s_6_16, -1, 4, 0},
    /* 17 */ {3, s_6_17, -1, 1, 0},
    /* 18 */ {4, s_6_18, -1, 1, 0},
    /* 19 */ {3, s_6_19, -1, 1, 0},
    /* 20 */ {7, s_6_20, -1, 1, 0},
    /* 21 */ {7, s_6_21, -1, 1, 0},
    /* 22 */ {3, s_6_22, -1, 9, 0},
    /* 23 */ {4, s_6_23, -1, 2, 0},
    /* 24 */ {4, s_6_24, -1, 1, 0},
    /* 25 */ {6, s_6_25, -1, 2, 0},
    /* 26 */ {6, s_6_26, -1, 5, 0},
    /* 27 */ {6, s_6_27, -1, 2, 0},
    /* 28 */ {4, s_6_28, -1, 1, 0},
    /* 29 */ {5, s_6_29, -1, 1, 0},
    /* 30 */ {4, s_6_30, -1, 9, 0},
    /* 31 */ {5, s_6_31, -1, 1, 0},
    /* 32 */ {7, s_6_32, -1, 3, 0},
    /* 33 */ {6, s_6_33, -1, 8, 0},
    /* 34 */ {5, s_6_34, -1, 1, 0},
    /* 35 */ {5, s_6_35, -1, 1, 0},
    /* 36 */ {7, s_6_36, -1, 2, 0},
    /* 37 */ {7, s_6_37, -1, 4, 0},
    /* 38 */ {6, s_6_38, -1, 2, 0},
    /* 39 */ {5, s_6_39, -1, 2, 0},
    /* 40 */ {4, s_6_40, -1, 1, 0},
    /* 41 */ {5, s_6_41, -1, 1, 0},
    /* 42 */ {4, s_6_42, -1, 1, 0},
    /* 43 */ {8, s_6_43, -1, 1, 0},
    /* 44 */ {8, s_6_44, -1, 1, 0},
    /* 45 */ {4, s_6_45, -1, 9, 0}};

static const symbol s_7_0[2] = {'y', 'a'};
static const symbol s_7_1[2] = {'y', 'e'};
static const symbol s_7_2[3] = {'y', 'a', 'n'};
static const symbol s_7_3[3] = {'y', 'e', 'n'};
static const symbol s_7_4[5] = {'y', 'e', 'r', 'o', 'n'};
static const symbol s_7_5[5] = {'y', 'e', 'n', 'd', 'o'};
static const symbol s_7_6[2] = {'y', 'o'};
static const symbol s_7_7[3] = {'y', 'a', 's'};
static const symbol s_7_8[3] = {'y', 'e', 's'};
static const symbol s_7_9[4] = {'y', 'a', 'i', 's'};
static const symbol s_7_10[5] = {'y', 'a', 'm', 'o', 's'};
static const symbol s_7_11[3] = {'y', 0xC3, 0xB3};

static const struct among a_7[12] = {
    /*  0 */ {2, s_7_0, -1, 1, 0},
    /*  1 */ {2, s_7_1, -1, 1, 0},
    /*  2 */ {3, s_7_2, -1, 1, 0},
    /*  3 */ {3, s_7_3, -1, 1, 0},
    /*  4 */ {5, s_7_4, -1, 1, 0},
    /*  5 */ {5, s_7_5, -1, 1, 0},
    /*  6 */ {2, s_7_6, -1, 1, 0},
    /*  7 */ {3, s_7_7, -1, 1, 0},
    /*  8 */ {3, s_7_8, -1, 1, 0},
    /*  9 */ {4, s_7_9, -1, 1, 0},
    /* 10 */ {5, s_7_10, -1, 1, 0},
    /* 11 */ {3, s_7_11, -1, 1, 0}};

static const symbol s_8_0[3] = {'a', 'b', 'a'};
static const symbol s_8_1[3] = {'a', 'd', 'a'};
static const symbol s_8_2[3] = {'i', 'd', 'a'};
static const symbol s_8_3[3] = {'a', 'r', 'a'};
static const symbol s_8_4[4] = {'i', 'e', 'r', 'a'};
static const symbol s_8_5[3] = {0xC3, 0xAD, 'a'};
static const symbol s_8_6[5] = {'a', 'r', 0xC3, 0xAD, 'a'};
static const symbol s_8_7[5] = {'e', 'r', 0xC3, 0xAD, 'a'};
static const symbol s_8_8[5] = {'i', 'r', 0xC3, 0xAD, 'a'};
static const symbol s_8_9[2] = {'a', 'd'};
static const symbol s_8_10[2] = {'e', 'd'};
static const symbol s_8_11[2] = {'i', 'd'};
static const symbol s_8_12[3] = {'a', 's', 'e'};
static const symbol s_8_13[4] = {'i', 'e', 's', 'e'};
static const symbol s_8_14[4] = {'a', 's', 't', 'e'};
static const symbol s_8_15[4] = {'i', 's', 't', 'e'};
static const symbol s_8_16[2] = {'a', 'n'};
static const symbol s_8_17[4] = {'a', 'b', 'a', 'n'};
static const symbol s_8_18[4] = {'a', 'r', 'a', 'n'};
static const symbol s_8_19[5] = {'i', 'e', 'r', 'a', 'n'};
static const symbol s_8_20[4] = {0xC3, 0xAD, 'a', 'n'};
static const symbol s_8_21[6] = {'a', 'r', 0xC3, 0xAD, 'a', 'n'};
static const symbol s_8_22[6] = {'e', 'r', 0xC3, 0xAD, 'a', 'n'};
static const symbol s_8_23[6] = {'i', 'r', 0xC3, 0xAD, 'a', 'n'};
static const symbol s_8_24[2] = {'e', 'n'};
static const symbol s_8_25[4] = {'a', 's', 'e', 'n'};
static const symbol s_8_26[5] = {'i', 'e', 's', 'e', 'n'};
static const symbol s_8_27[4] = {'a', 'r', 'o', 'n'};
static const symbol s_8_28[5] = {'i', 'e', 'r', 'o', 'n'};
static const symbol s_8_29[5] = {'a', 'r', 0xC3, 0xA1, 'n'};
static const symbol s_8_30[5] = {'e', 'r', 0xC3, 0xA1, 'n'};
static const symbol s_8_31[5] = {'i', 'r', 0xC3, 0xA1, 'n'};
static const symbol s_8_32[3] = {'a', 'd', 'o'};
static const symbol s_8_33[3] = {'i', 'd', 'o'};
static const symbol s_8_34[4] = {'a', 'n', 'd', 'o'};
static const symbol s_8_35[5] = {'i', 'e', 'n', 'd', 'o'};
static const symbol s_8_36[2] = {'a', 'r'};
static const symbol s_8_37[2] = {'e', 'r'};
static const symbol s_8_38[2] = {'i', 'r'};
static const symbol s_8_39[2] = {'a', 's'};
static const symbol s_8_40[4] = {'a', 'b', 'a', 's'};
static const symbol s_8_41[4] = {'a', 'd', 'a', 's'};
static const symbol s_8_42[4] = {'i', 'd', 'a', 's'};
static const symbol s_8_43[4] = {'a', 'r', 'a', 's'};
static const symbol s_8_44[5] = {'i', 'e', 'r', 'a', 's'};
static const symbol s_8_45[4] = {0xC3, 0xAD, 'a', 's'};
static const symbol s_8_46[6] = {'a', 'r', 0xC3, 0xAD, 'a', 's'};
static const symbol s_8_47[6] = {'e', 'r', 0xC3, 0xAD, 'a', 's'};
static const symbol s_8_48[6] = {'i', 'r', 0xC3, 0xAD, 'a', 's'};
static const symbol s_8_49[2] = {'e', 's'};
static const symbol s_8_50[4] = {'a', 's', 'e', 's'};
static const symbol s_8_51[5] = {'i', 'e', 's', 'e', 's'};
static const symbol s_8_52[5] = {'a', 'b', 'a', 'i', 's'};
static const symbol s_8_53[5] = {'a', 'r', 'a', 'i', 's'};
static const symbol s_8_54[6] = {'i', 'e', 'r', 'a', 'i', 's'};
static const symbol s_8_55[5] = {0xC3, 0xAD, 'a', 'i', 's'};
static const symbol s_8_56[7] = {'a', 'r', 0xC3, 0xAD, 'a', 'i', 's'};
static const symbol s_8_57[7] = {'e', 'r', 0xC3, 0xAD, 'a', 'i', 's'};
static const symbol s_8_58[7] = {'i', 'r', 0xC3, 0xAD, 'a', 'i', 's'};
static const symbol s_8_59[5] = {'a', 's', 'e', 'i', 's'};
static const symbol s_8_60[6] = {'i', 'e', 's', 'e', 'i', 's'};
static const symbol s_8_61[6] = {'a', 's', 't', 'e', 'i', 's'};
static const symbol s_8_62[6] = {'i', 's', 't', 'e', 'i', 's'};
static const symbol s_8_63[4] = {0xC3, 0xA1, 'i', 's'};
static const symbol s_8_64[4] = {0xC3, 0xA9, 'i', 's'};
static const symbol s_8_65[6] = {'a', 'r', 0xC3, 0xA9, 'i', 's'};
static const symbol s_8_66[6] = {'e', 'r', 0xC3, 0xA9, 'i', 's'};
static const symbol s_8_67[6] = {'i', 'r', 0xC3, 0xA9, 'i', 's'};
static const symbol s_8_68[4] = {'a', 'd', 'o', 's'};
static const symbol s_8_69[4] = {'i', 'd', 'o', 's'};
static const symbol s_8_70[4] = {'a', 'm', 'o', 's'};
static const symbol s_8_71[7] = {0xC3, 0xA1, 'b', 'a', 'm', 'o', 's'};
static const symbol s_8_72[7] = {0xC3, 0xA1, 'r', 'a', 'm', 'o', 's'};
static const symbol s_8_73[8] = {'i', 0xC3, 0xA9, 'r', 'a', 'm', 'o', 's'};
static const symbol s_8_74[6] = {0xC3, 0xAD, 'a', 'm', 'o', 's'};
static const symbol s_8_75[8] = {'a', 'r', 0xC3, 0xAD, 'a', 'm', 'o', 's'};
static const symbol s_8_76[8] = {'e', 'r', 0xC3, 0xAD, 'a', 'm', 'o', 's'};
static const symbol s_8_77[8] = {'i', 'r', 0xC3, 0xAD, 'a', 'm', 'o', 's'};
static const symbol s_8_78[4] = {'e', 'm', 'o', 's'};
static const symbol s_8_79[6] = {'a', 'r', 'e', 'm', 'o', 's'};
static const symbol s_8_80[6] = {'e', 'r', 'e', 'm', 'o', 's'};
static const symbol s_8_81[6] = {'i', 'r', 'e', 'm', 'o', 's'};
static const symbol s_8_82[7] = {0xC3, 0xA1, 's', 'e', 'm', 'o', 's'};
static const symbol s_8_83[8] = {'i', 0xC3, 0xA9, 's', 'e', 'm', 'o', 's'};
static const symbol s_8_84[4] = {'i', 'm', 'o', 's'};
static const symbol s_8_85[5] = {'a', 'r', 0xC3, 0xA1, 's'};
static const symbol s_8_86[5] = {'e', 'r', 0xC3, 0xA1, 's'};
static const symbol s_8_87[5] = {'i', 'r', 0xC3, 0xA1, 's'};
static const symbol s_8_88[3] = {0xC3, 0xAD, 's'};
static const symbol s_8_89[4] = {'a', 'r', 0xC3, 0xA1};
static const symbol s_8_90[4] = {'e', 'r', 0xC3, 0xA1};
static const symbol s_8_91[4] = {'i', 'r', 0xC3, 0xA1};
static const symbol s_8_92[4] = {'a', 'r', 0xC3, 0xA9};
static const symbol s_8_93[4] = {'e', 'r', 0xC3, 0xA9};
static const symbol s_8_94[4] = {'i', 'r', 0xC3, 0xA9};
static const symbol s_8_95[3] = {'i', 0xC3, 0xB3};

static const struct among a_8[96] = {
    /*  0 */ {3, s_8_0, -1, 2, 0},
    /*  1 */ {3, s_8_1, -1, 2, 0},
    /*  2 */ {3, s_8_2, -1, 2, 0},
    /*  3 */ {3, s_8_3, -1, 2, 0},
    /*  4 */ {4, s_8_4, -1, 2, 0},
    /*  5 */ {3, s_8_5, -1, 2, 0},
    /*  6 */ {5, s_8_6, 5, 2, 0},
    /*  7 */ {5, s_8_7, 5, 2, 0},
    /*  8 */ {5, s_8_8, 5, 2, 0},
    /*  9 */ {2, s_8_9, -1, 2, 0},
    /* 10 */ {2, s_8_10, -1, 2, 0},
    /* 11 */ {2, s_8_11, -1, 2, 0},
    /* 12 */ {3, s_8_12, -1, 2, 0},
    /* 13 */ {4, s_8_13, -1, 2, 0},
    /* 14 */ {4, s_8_14, -1, 2, 0},
    /* 15 */ {4, s_8_15, -1, 2, 0},
    /* 16 */ {2, s_8_16, -1, 2, 0},
    /* 17 */ {4, s_8_17, 16, 2, 0},
    /* 18 */ {4, s_8_18, 16, 2, 0},
    /* 19 */ {5, s_8_19, 16, 2, 0},
    /* 20 */ {4, s_8_20, 16, 2, 0},
    /* 21 */ {6, s_8_21, 20, 2, 0},
    /* 22 */ {6, s_8_22, 20, 2, 0},
    /* 23 */ {6, s_8_23, 20, 2, 0},
    /* 24 */ {2, s_8_24, -1, 1, 0},
    /* 25 */ {4, s_8_25, 24, 2, 0},
    /* 26 */ {5, s_8_26, 24, 2, 0},
    /* 27 */ {4, s_8_27, -1, 2, 0},
    /* 28 */ {5, s_8_28, -1, 2, 0},
    /* 29 */ {5, s_8_29, -1, 2, 0},
    /* 30 */ {5, s_8_30, -1, 2, 0},
    /* 31 */ {5, s_8_31, -1, 2, 0},
    /* 32 */ {3, s_8_32, -1, 2, 0},
    /* 33 */ {3, s_8_33, -1, 2, 0},
    /* 34 */ {4, s_8_34, -1, 2, 0},
    /* 35 */ {5, s_8_35, -1, 2, 0},
    /* 36 */ {2, s_8_36, -1, 2, 0},
    /* 37 */ {2, s_8_37, -1, 2, 0},
    /* 38 */ {2, s_8_38, -1, 2, 0},
    /* 39 */ {2, s_8_39, -1, 2, 0},
    /* 40 */ {4, s_8_40, 39, 2, 0},
    /* 41 */ {4, s_8_41, 39, 2, 0},
    /* 42 */ {4, s_8_42, 39, 2, 0},
    /* 43 */ {4, s_8_43, 39, 2, 0},
    /* 44 */ {5, s_8_44, 39, 2, 0},
    /* 45 */ {4, s_8_45, 39, 2, 0},
    /* 46 */ {6, s_8_46, 45, 2, 0},
    /* 47 */ {6, s_8_47, 45, 2, 0},
    /* 48 */ {6, s_8_48, 45, 2, 0},
    /* 49 */ {2, s_8_49, -1, 1, 0},
    /* 50 */ {4, s_8_50, 49, 2, 0},
    /* 51 */ {5, s_8_51, 49, 2, 0},
    /* 52 */ {5, s_8_52, -1, 2, 0},
    /* 53 */ {5, s_8_53, -1, 2, 0},
    /* 54 */ {6, s_8_54, -1, 2, 0},
    /* 55 */ {5, s_8_55, -1, 2, 0},
    /* 56 */ {7, s_8_56, 55, 2, 0},
    /* 57 */ {7, s_8_57, 55, 2, 0},
    /* 58 */ {7, s_8_58, 55, 2, 0},
    /* 59 */ {5, s_8_59, -1, 2, 0},
    /* 60 */ {6, s_8_60, -1, 2, 0},
    /* 61 */ {6, s_8_61, -1, 2, 0},
    /* 62 */ {6, s_8_62, -1, 2, 0},
    /* 63 */ {4, s_8_63, -1, 2, 0},
    /* 64 */ {4, s_8_64, -1, 1, 0},
    /* 65 */ {6, s_8_65, 64, 2, 0},
    /* 66 */ {6, s_8_66, 64, 2, 0},
    /* 67 */ {6, s_8_67, 64, 2, 0},
    /* 68 */ {4, s_8_68, -1, 2, 0},
    /* 69 */ {4, s_8_69, -1, 2, 0},
    /* 70 */ {4, s_8_70, -1, 2, 0},
    /* 71 */ {7, s_8_71, 70, 2, 0},
    /* 72 */ {7, s_8_72, 70, 2, 0},
    /* 73 */ {8, s_8_73, 70, 2, 0},
    /* 74 */ {6, s_8_74, 70, 2, 0},
    /* 75 */ {8, s_8_75, 74, 2, 0},
    /* 76 */ {8, s_8_76, 74, 2, 0},
    /* 77 */ {8, s_8_77, 74, 2, 0},
    /* 78 */ {4, s_8_78, -1, 1, 0},
    /* 79 */ {6, s_8_79, 78, 2, 0},
    /* 80 */ {6, s_8_80, 78, 2, 0},
    /* 81 */ {6, s_8_81, 78, 2, 0},
    /* 82 */ {7, s_8_82, 78, 2, 0},
    /* 83 */ {8, s_8_83, 78, 2, 0},
    /* 84 */ {4, s_8_84, -1, 2, 0},
    /* 85 */ {5, s_8_85, -1, 2, 0},
    /* 86 */ {5, s_8_86, -1, 2, 0},
    /* 87 */ {5, s_8_87, -1, 2, 0},
    /* 88 */ {3, s_8_88, -1, 2, 0},
    /* 89 */ {4, s_8_89, -1, 2, 0},
    /* 90 */ {4, s_8_90, -1, 2, 0},
    /* 91 */ {4, s_8_91, -1, 2, 0},
    /* 92 */ {4, s_8_92, -1, 2, 0},
    /* 93 */ {4, s_8_93, -1, 2, 0},
    /* 94 */ {4, s_8_94, -1, 2, 0},
    /* 95 */ {3, s_8_95, -1, 2, 0}};

static const symbol s_9_0[1] = {'a'};
static const symbol s_9_1[1] = {'e'};
static const symbol s_9_2[1] = {'o'};
static const symbol s_9_3[2] = {'o', 's'};
static const symbol s_9_4[2] = {0xC3, 0xA1};
static const symbol s_9_5[2] = {0xC3, 0xA9};
static const symbol s_9_6[2] = {0xC3, 0xAD};
static const symbol s_9_7[2] = {0xC3, 0xB3};

static const struct among a_9[8] = {
    /*  0 */ {1, s_9_0, -1, 1, 0},
    /*  1 */ {1, s_9_1, -1, 2, 0},
    /*  2 */ {1, s_9_2, -1, 1, 0},
    /*  3 */ {2, s_9_3, -1, 1, 0},
    /*  4 */ {2, s_9_4, -1, 1, 0},
    /*  5 */ {2, s_9_5, -1, 2, 0},
    /*  6 */ {2, s_9_6, -1, 1, 0},
    /*  7 */ {2, s_9_7, -1, 1, 0}};

static const unsigned char g_v[] = {17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 17, 4, 10};

static const symbol s_0[] = {'a'};
static const symbol s_1[] = {'e'};
static const symbol s_2[] = {'i'};
static const symbol s_3[] = {'o'};
static const symbol s_4[] = {'u'};
static const symbol s_5[] = {'i', 'e', 'n', 'd', 'o'};
static const symbol s_6[] = {'a', 'n', 'd', 'o'};
static const symbol s_7[] = {'a', 'r'};
static const symbol s_8[] = {'e', 'r'};
static const symbol s_9[] = {'i', 'r'};
static const symbol s_10[] = {'u'};
static const symbol s_11[] = {'i', 'c'};
static const symbol s_12[] = {'l', 'o', 'g'};
static const symbol s_13[] = {'u'};
static const symbol s_14[] = {'e', 'n', 't', 'e'};
static const symbol s_15[] = {'a', 't'};
static const symbol s_16[] = {'a', 't'};
static const symbol s_17[] = {'u'};
static const symbol s_18[] = {'u'};
static const symbol s_19[] = {'g'};
static const symbol s_20[] = {'u'};
static const symbol s_21[] = {'g'};

static int r_mark_regions(struct SN_env *z) {
    z->I[0] = z->l;
    z->I[1] = z->l;
    z->I[2] = z->l;
    {
        int c1 = z->c; /* do, line 37 */
        {
            int c2 = z->c; /* or, line 39 */
            if (in_grouping_U(z, g_v, 97, 252, 0))
                goto lab2;
            {
                int c3 = z->c; /* or, line 38 */
                if (out_grouping_U(z, g_v, 97, 252, 0))
                    goto lab4;
                { /* gopast */ /* grouping v, line 38 */
                    int ret = out_grouping_U(z, g_v, 97, 252, 1);
                    if (ret < 0)
                        goto lab4;
                    z->c += ret;
                }
                goto lab3;
            lab4:
                z->c = c3;
                if (in_grouping_U(z, g_v, 97, 252, 0))
                    goto lab2;
                { /* gopast */ /* non v, line 38 */
                    int ret = in_grouping_U(z, g_v, 97, 252, 1);
                    if (ret < 0)
                        goto lab2;
                    z->c += ret;
                }
            }
        lab3:
            goto lab1;
        lab2:
            z->c = c2;
            if (out_grouping_U(z, g_v, 97, 252, 0))
                goto lab0;
            {
                int c4 = z->c; /* or, line 40 */
                if (out_grouping_U(z, g_v, 97, 252, 0))
                    goto lab6;
                { /* gopast */ /* grouping v, line 40 */
                    int ret = out_grouping_U(z, g_v, 97, 252, 1);
                    if (ret < 0)
                        goto lab6;
                    z->c += ret;
                }
                goto lab5;
            lab6:
                z->c = c4;
                if (in_grouping_U(z, g_v, 97, 252, 0))
                    goto lab0;
                {
                    int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                    if (ret < 0)
                        goto lab0;
                    z->c = ret; /* next, line 40 */
                }
            }
        lab5:;
        }
    lab1:
        z->I[0] = z->c; /* setmark pV, line 41 */
    lab0:
        z->c = c1;
    }
    {
        int c5 = z->c; /* do, line 43 */
        { /* gopast */ /* grouping v, line 44 */
            int ret = out_grouping_U(z, g_v, 97, 252, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        { /* gopast */ /* non v, line 44 */
            int ret = in_grouping_U(z, g_v, 97, 252, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        z->I[1] = z->c; /* setmark p1, line 44 */
        { /* gopast */  /* grouping v, line 45 */
            int ret = out_grouping_U(z, g_v, 97, 252, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        { /* gopast */ /* non v, line 45 */
            int ret = in_grouping_U(z, g_v, 97, 252, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        z->I[2] = z->c; /* setmark p2, line 45 */
    lab7:
        z->c = c5;
    }
    return 1;
}

static int r_postlude(struct SN_env *z) {
    int among_var;
    while (1) { /* repeat, line 49 */
        int c1 = z->c;
        z->bra = z->c; /* [, line 50 */
        if (z->c + 1 >= z->l || z->p[z->c + 1] >> 5 != 5 || !((67641858 >> (z->p[z->c + 1] & 0x1f)) & 1))
            among_var = 6;
        else
            among_var = find_among(z, a_0, 6); /* substring, line 50 */
        if (!(among_var))
            goto lab0;
        z->ket = z->c; /* ], line 50 */
        switch (among_var) {
            case 0:
                goto lab0;
            case 1: {
                int ret = slice_from_s(z, 1, s_0); /* <-, line 51 */
                if (ret < 0)
                    return ret;
            } break;
            case 2: {
                int ret = slice_from_s(z, 1, s_1); /* <-, line 52 */
                if (ret < 0)
                    return ret;
            } break;
            case 3: {
                int ret = slice_from_s(z, 1, s_2); /* <-, line 53 */
                if (ret < 0)
                    return ret;
            } break;
            case 4: {
                int ret = slice_from_s(z, 1, s_3); /* <-, line 54 */
                if (ret < 0)
                    return ret;
            } break;
            case 5: {
                int ret = slice_from_s(z, 1, s_4); /* <-, line 55 */
                if (ret < 0)
                    return ret;
            } break;
            case 6: {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    goto lab0;
                z->c = ret; /* next, line 57 */
            } break;
        }
        continue;
    lab0:
        z->c = c1;
        break;
    }
    return 1;
}

static int r_RV(struct SN_env *z) {
    if (!(z->I[0] <= z->c))
        return 0;
    return 1;
}

static int r_R1(struct SN_env *z) {
    if (!(z->I[1] <= z->c))
        return 0;
    return 1;
}

static int r_R2(struct SN_env *z) {
    if (!(z->I[2] <= z->c))
        return 0;
    return 1;
}

static int r_attached_pronoun(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 68 */
    if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((557090 >> (z->p[z->c - 1] & 0x1f)) & 1))
        return 0;
    if (!(find_among_b(z, a_1, 13)))
        return 0;  /* substring, line 68 */
    z->bra = z->c; /* ], line 68 */
    if (z->c - 1 <= z->lb || (z->p[z->c - 1] != 111 && z->p[z->c - 1] != 114))
        return 0;
    among_var = find_among_b(z, a_2, 11); /* substring, line 72 */
    if (!(among_var))
        return 0;
    {
        int ret = r_RV(z);
        if (ret == 0)
            return 0; /* call RV, line 72 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1:
            z->bra = z->c; /* ], line 73 */
            {
                int ret = slice_from_s(z, 5, s_5); /* <-, line 73 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 2:
            z->bra = z->c; /* ], line 74 */
            {
                int ret = slice_from_s(z, 4, s_6); /* <-, line 74 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 3:
            z->bra = z->c; /* ], line 75 */
            {
                int ret = slice_from_s(z, 2, s_7); /* <-, line 75 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 4:
            z->bra = z->c; /* ], line 76 */
            {
                int ret = slice_from_s(z, 2, s_8); /* <-, line 76 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 5:
            z->bra = z->c; /* ], line 77 */
            {
                int ret = slice_from_s(z, 2, s_9); /* <-, line 77 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 6: {
            int ret = slice_del(z); /* delete, line 81 */
            if (ret < 0)
                return ret;
        } break;
        case 7:
            if (!(eq_s_b(z, 1, s_10)))
                return 0;
            {
                int ret = slice_del(z); /* delete, line 82 */
                if (ret < 0)
                    return ret;
            }
            break;
    }
    return 1;
}

static int r_standard_suffix(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 87 */
    if (z->c - 2 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((835634 >> (z->p[z->c - 1] & 0x1f)) & 1))
        return 0;
    among_var = find_among_b(z, a_6, 46); /* substring, line 87 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 87 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 99 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 99 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 2: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 105 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 105 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 106 */
                z->ket = z->c;                                /* [, line 106 */
                if (!(eq_s_b(z, 2, s_11))) {
                    z->c = z->l - m_keep;
                    goto lab0;
                }
                z->bra = z->c; /* ], line 106 */
                {
                    int ret = r_R2(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab0;
                    } /* call R2, line 106 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int ret = slice_del(z); /* delete, line 106 */
                    if (ret < 0)
                        return ret;
                }
            lab0:;
            }
            break;
        case 3: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 111 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_from_s(z, 3, s_12); /* <-, line 111 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 4: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 115 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_from_s(z, 1, s_13); /* <-, line 115 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 5: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 119 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_from_s(z, 4, s_14); /* <-, line 119 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 6: {
            int ret = r_R1(z);
            if (ret == 0)
                return 0; /* call R1, line 123 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 123 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 124 */
                z->ket = z->c;                                /* [, line 125 */
                if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((4718616 >> (z->p[z->c - 1] & 0x1f)) & 1)) {
                    z->c = z->l - m_keep;
                    goto lab1;
                }
                among_var = find_among_b(z, a_3, 4); /* substring, line 125 */
                if (!(among_var)) {
                    z->c = z->l - m_keep;
                    goto lab1;
                }
                z->bra = z->c; /* ], line 125 */
                {
                    int ret = r_R2(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab1;
                    } /* call R2, line 125 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int ret = slice_del(z); /* delete, line 125 */
                    if (ret < 0)
                        return ret;
                }
                switch (among_var) {
                    case 0: {
                        z->c = z->l - m_keep;
                        goto lab1;
                    }
                    case 1:
                        z->ket = z->c; /* [, line 126 */
                        if (!(eq_s_b(z, 2, s_15))) {
                            z->c = z->l - m_keep;
                            goto lab1;
                        }
                        z->bra = z->c; /* ], line 126 */
                        {
                            int ret = r_R2(z);
                            if (ret == 0) {
                                z->c = z->l - m_keep;
                                goto lab1;
                            } /* call R2, line 126 */
                            if (ret < 0)
                                return ret;
                        }
                        {
                            int ret = slice_del(z); /* delete, line 126 */
                            if (ret < 0)
                                return ret;
                        }
                        break;
                }
            lab1:;
            }
            break;
        case 7: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 135 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 135 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 136 */
                z->ket = z->c;                                /* [, line 137 */
                if (z->c - 3 <= z->lb || z->p[z->c - 1] != 101) {
                    z->c = z->l - m_keep;
                    goto lab2;
                }
                among_var = find_among_b(z, a_4, 3); /* substring, line 137 */
                if (!(among_var)) {
                    z->c = z->l - m_keep;
                    goto lab2;
                }
                z->bra = z->c; /* ], line 137 */
                switch (among_var) {
                    case 0: {
                        z->c = z->l - m_keep;
                        goto lab2;
                    }
                    case 1: {
                        int ret = r_R2(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab2;
                        } /* call R2, line 140 */
                        if (ret < 0)
                            return ret;
                    }
                        {
                            int ret = slice_del(z); /* delete, line 140 */
                            if (ret < 0)
                                return ret;
                        }
                        break;
                }
            lab2:;
            }
            break;
        case 8: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 147 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 147 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 148 */
                z->ket = z->c;                                /* [, line 149 */
                if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((4198408 >> (z->p[z->c - 1] & 0x1f)) & 1)) {
                    z->c = z->l - m_keep;
                    goto lab3;
                }
                among_var = find_among_b(z, a_5, 3); /* substring, line 149 */
                if (!(among_var)) {
                    z->c = z->l - m_keep;
                    goto lab3;
                }
                z->bra = z->c; /* ], line 149 */
                switch (among_var) {
                    case 0: {
                        z->c = z->l - m_keep;
                        goto lab3;
                    }
                    case 1: {
                        int ret = r_R2(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab3;
                        } /* call R2, line 152 */
                        if (ret < 0)
                            return ret;
                    }
                        {
                            int ret = slice_del(z); /* delete, line 152 */
                            if (ret < 0)
                                return ret;
                        }
                        break;
                }
            lab3:;
            }
            break;
        case 9: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 159 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 159 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 160 */
                z->ket = z->c;                                /* [, line 161 */
                if (!(eq_s_b(z, 2, s_16))) {
                    z->c = z->l - m_keep;
                    goto lab4;
                }
                z->bra = z->c; /* ], line 161 */
                {
                    int ret = r_R2(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab4;
                    } /* call R2, line 161 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int ret = slice_del(z); /* delete, line 161 */
                    if (ret < 0)
                        return ret;
                }
            lab4:;
            }
            break;
    }
    return 1;
}

static int r_y_verb_suffix(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 168 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 168 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c;                        /* [, line 168 */
        among_var = find_among_b(z, a_7, 12); /* substring, line 168 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 168 */
        z->lb = mlimit;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1:
            if (!(eq_s_b(z, 1, s_17)))
                return 0;
            {
                int ret = slice_del(z); /* delete, line 171 */
                if (ret < 0)
                    return ret;
            }
            break;
    }
    return 1;
}

static int r_verb_suffix(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 176 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 176 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c;                        /* [, line 176 */
        among_var = find_among_b(z, a_8, 96); /* substring, line 176 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 176 */
        z->lb = mlimit;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 179 */
            if (!(eq_s_b(z, 1, s_18))) {
                z->c = z->l - m_keep;
                goto lab0;
            }
            {
                int m_test = z->l - z->c; /* test, line 179 */
                if (!(eq_s_b(z, 1, s_19))) {
                    z->c = z->l - m_keep;
                    goto lab0;
                }
                z->c = z->l - m_test;
            }
        lab0:;
        }
            z->bra = z->c; /* ], line 179 */
            {
                int ret = slice_del(z); /* delete, line 179 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 2: {
            int ret = slice_del(z); /* delete, line 200 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_residual_suffix(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                       /* [, line 205 */
    among_var = find_among_b(z, a_9, 8); /* substring, line 205 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 205 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = r_RV(z);
            if (ret == 0)
                return 0; /* call RV, line 208 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 208 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 2: {
            int ret = r_RV(z);
            if (ret == 0)
                return 0; /* call RV, line 210 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 210 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 210 */
                z->ket = z->c;                                /* [, line 210 */
                if (!(eq_s_b(z, 1, s_20))) {
                    z->c = z->l - m_keep;
                    goto lab0;
                }
                z->bra = z->c; /* ], line 210 */
                {
                    int m_test = z->l - z->c; /* test, line 210 */
                    if (!(eq_s_b(z, 1, s_21))) {
                        z->c = z->l - m_keep;
                        goto lab0;
                    }
                    z->c = z->l - m_test;
                }
                {
                    int ret = r_RV(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab0;
                    } /* call RV, line 210 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int ret = slice_del(z); /* delete, line 210 */
                    if (ret < 0)
                        return ret;
                }
            lab0:;
            }
            break;
    }
    return 1;
}

extern int spanish_UTF_8_stem(struct SN_env *z) {
    {
        int c1 = z->c; /* do, line 216 */
        {
            int ret = r_mark_regions(z);
            if (ret == 0)
                goto lab0; /* call mark_regions, line 216 */
            if (ret < 0)
                return ret;
        }
    lab0:
        z->c = c1;
    }
    z->lb = z->c;
    z->c = z->l; /* backwards, line 217 */

    {
        int m2 = z->l - z->c;
        (void)m2; /* do, line 218 */
        {
            int ret = r_attached_pronoun(z);
            if (ret == 0)
                goto lab1; /* call attached_pronoun, line 218 */
            if (ret < 0)
                return ret;
        }
    lab1:
        z->c = z->l - m2;
    }
    {
        int m3 = z->l - z->c;
        (void)m3; /* do, line 219 */
        {
            int m4 = z->l - z->c;
            (void)m4; /* or, line 219 */
            {
                int ret = r_standard_suffix(z);
                if (ret == 0)
                    goto lab4; /* call standard_suffix, line 219 */
                if (ret < 0)
                    return ret;
            }
            goto lab3;
        lab4:
            z->c = z->l - m4;
            {
                int ret = r_y_verb_suffix(z);
                if (ret == 0)
                    goto lab5; /* call y_verb_suffix, line 220 */
                if (ret < 0)
                    return ret;
            }
            goto lab3;
        lab5:
            z->c = z->l - m4;
            {
                int ret = r_verb_suffix(z);
                if (ret == 0)
                    goto lab2; /* call verb_suffix, line 221 */
                if (ret < 0)
                    return ret;
            }
        }
    lab3:
    lab2:
        z->c = z->l - m3;
    }
    {
        int m5 = z->l - z->c;
        (void)m5; /* do, line 223 */
        {
            int ret = r_residual_suffix(z);
            if (ret == 0)
                goto lab6; /* call residual_suffix, line 223 */
            if (ret < 0)
                return ret;
        }
    lab6:
        z->c = z->l - m5;
    }
    z->c = z->lb;
    {
        int c6 = z->c; /* do, line 225 */
        {
            int ret = r_postlude(z);
            if (ret == 0)
                goto lab7; /* call postlude, line 225 */
            if (ret < 0)
                return ret;
        }
    lab7:
        z->c = c6;
    }
    return 1;
}

extern struct SN_env *spanish_UTF_8_create_env(void) { return SN_create_env(0, 3, 0); }

extern void spanish_UTF_8_close_env(struct SN_env *z) { SN_close_env(z, 0); }
