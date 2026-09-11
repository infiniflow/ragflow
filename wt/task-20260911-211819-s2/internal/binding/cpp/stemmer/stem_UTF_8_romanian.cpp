
/* This file was generated automatically by the Snowball to ANSI C compiler */

#include "header.h"

#ifdef __cplusplus
extern "C" {
#endif
extern int romanian_UTF_8_stem(struct SN_env *z);
#ifdef __cplusplus
}
#endif
static int r_vowel_suffix(struct SN_env *z);
static int r_verb_suffix(struct SN_env *z);
static int r_combo_suffix(struct SN_env *z);
static int r_standard_suffix(struct SN_env *z);
static int r_step_0(struct SN_env *z);
static int r_R2(struct SN_env *z);
static int r_R1(struct SN_env *z);
static int r_RV(struct SN_env *z);
static int r_mark_regions(struct SN_env *z);
static int r_postlude(struct SN_env *z);
static int r_prelude(struct SN_env *z);
#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *romanian_UTF_8_create_env(void);
extern void romanian_UTF_8_close_env(struct SN_env *z);

#ifdef __cplusplus
}
#endif
static const symbol s_0_1[1] = {'I'};
static const symbol s_0_2[1] = {'U'};

static const struct among a_0[3] = {
    /*  0 */ {0, 0, -1, 3, 0},
    /*  1 */ {1, s_0_1, 0, 1, 0},
    /*  2 */ {1, s_0_2, 0, 2, 0}};

static const symbol s_1_0[2] = {'e', 'a'};
static const symbol s_1_1[5] = {'a', 0xC5, 0xA3, 'i', 'a'};
static const symbol s_1_2[3] = {'a', 'u', 'a'};
static const symbol s_1_3[3] = {'i', 'u', 'a'};
static const symbol s_1_4[5] = {'a', 0xC5, 0xA3, 'i', 'e'};
static const symbol s_1_5[3] = {'e', 'l', 'e'};
static const symbol s_1_6[3] = {'i', 'l', 'e'};
static const symbol s_1_7[4] = {'i', 'i', 'l', 'e'};
static const symbol s_1_8[3] = {'i', 'e', 'i'};
static const symbol s_1_9[4] = {'a', 't', 'e', 'i'};
static const symbol s_1_10[2] = {'i', 'i'};
static const symbol s_1_11[4] = {'u', 'l', 'u', 'i'};
static const symbol s_1_12[2] = {'u', 'l'};
static const symbol s_1_13[4] = {'e', 'l', 'o', 'r'};
static const symbol s_1_14[4] = {'i', 'l', 'o', 'r'};
static const symbol s_1_15[5] = {'i', 'i', 'l', 'o', 'r'};

static const struct among a_1[16] = {
    /*  0 */ {2, s_1_0, -1, 3, 0},
    /*  1 */ {5, s_1_1, -1, 7, 0},
    /*  2 */ {3, s_1_2, -1, 2, 0},
    /*  3 */ {3, s_1_3, -1, 4, 0},
    /*  4 */ {5, s_1_4, -1, 7, 0},
    /*  5 */ {3, s_1_5, -1, 3, 0},
    /*  6 */ {3, s_1_6, -1, 5, 0},
    /*  7 */ {4, s_1_7, 6, 4, 0},
    /*  8 */ {3, s_1_8, -1, 4, 0},
    /*  9 */ {4, s_1_9, -1, 6, 0},
    /* 10 */ {2, s_1_10, -1, 4, 0},
    /* 11 */ {4, s_1_11, -1, 1, 0},
    /* 12 */ {2, s_1_12, -1, 1, 0},
    /* 13 */ {4, s_1_13, -1, 3, 0},
    /* 14 */ {4, s_1_14, -1, 4, 0},
    /* 15 */ {5, s_1_15, 14, 4, 0}};

static const symbol s_2_0[5] = {'i', 'c', 'a', 'l', 'a'};
static const symbol s_2_1[5] = {'i', 'c', 'i', 'v', 'a'};
static const symbol s_2_2[5] = {'a', 't', 'i', 'v', 'a'};
static const symbol s_2_3[5] = {'i', 't', 'i', 'v', 'a'};
static const symbol s_2_4[5] = {'i', 'c', 'a', 'l', 'e'};
static const symbol s_2_5[7] = {'a', 0xC5, 0xA3, 'i', 'u', 'n', 'e'};
static const symbol s_2_6[7] = {'i', 0xC5, 0xA3, 'i', 'u', 'n', 'e'};
static const symbol s_2_7[6] = {'a', 't', 'o', 'a', 'r', 'e'};
static const symbol s_2_8[6] = {'i', 't', 'o', 'a', 'r', 'e'};
static const symbol s_2_9[7] = {0xC4, 0x83, 't', 'o', 'a', 'r', 'e'};
static const symbol s_2_10[7] = {'i', 'c', 'i', 't', 'a', 't', 'e'};
static const symbol s_2_11[9] = {'a', 'b', 'i', 'l', 'i', 't', 'a', 't', 'e'};
static const symbol s_2_12[9] = {'i', 'b', 'i', 'l', 'i', 't', 'a', 't', 'e'};
static const symbol s_2_13[7] = {'i', 'v', 'i', 't', 'a', 't', 'e'};
static const symbol s_2_14[5] = {'i', 'c', 'i', 'v', 'e'};
static const symbol s_2_15[5] = {'a', 't', 'i', 'v', 'e'};
static const symbol s_2_16[5] = {'i', 't', 'i', 'v', 'e'};
static const symbol s_2_17[5] = {'i', 'c', 'a', 'l', 'i'};
static const symbol s_2_18[5] = {'a', 't', 'o', 'r', 'i'};
static const symbol s_2_19[7] = {'i', 'c', 'a', 't', 'o', 'r', 'i'};
static const symbol s_2_20[5] = {'i', 't', 'o', 'r', 'i'};
static const symbol s_2_21[6] = {0xC4, 0x83, 't', 'o', 'r', 'i'};
static const symbol s_2_22[7] = {'i', 'c', 'i', 't', 'a', 't', 'i'};
static const symbol s_2_23[9] = {'a', 'b', 'i', 'l', 'i', 't', 'a', 't', 'i'};
static const symbol s_2_24[7] = {'i', 'v', 'i', 't', 'a', 't', 'i'};
static const symbol s_2_25[5] = {'i', 'c', 'i', 'v', 'i'};
static const symbol s_2_26[5] = {'a', 't', 'i', 'v', 'i'};
static const symbol s_2_27[5] = {'i', 't', 'i', 'v', 'i'};
static const symbol s_2_28[7] = {'i', 'c', 'i', 't', 0xC4, 0x83, 'i'};
static const symbol s_2_29[9] = {'a', 'b', 'i', 'l', 'i', 't', 0xC4, 0x83, 'i'};
static const symbol s_2_30[7] = {'i', 'v', 'i', 't', 0xC4, 0x83, 'i'};
static const symbol s_2_31[9] = {'i', 'c', 'i', 't', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_2_32[11] = {'a', 'b', 'i', 'l', 'i', 't', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_2_33[9] = {'i', 'v', 'i', 't', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_2_34[4] = {'i', 'c', 'a', 'l'};
static const symbol s_2_35[4] = {'a', 't', 'o', 'r'};
static const symbol s_2_36[6] = {'i', 'c', 'a', 't', 'o', 'r'};
static const symbol s_2_37[4] = {'i', 't', 'o', 'r'};
static const symbol s_2_38[5] = {0xC4, 0x83, 't', 'o', 'r'};
static const symbol s_2_39[4] = {'i', 'c', 'i', 'v'};
static const symbol s_2_40[4] = {'a', 't', 'i', 'v'};
static const symbol s_2_41[4] = {'i', 't', 'i', 'v'};
static const symbol s_2_42[6] = {'i', 'c', 'a', 'l', 0xC4, 0x83};
static const symbol s_2_43[6] = {'i', 'c', 'i', 'v', 0xC4, 0x83};
static const symbol s_2_44[6] = {'a', 't', 'i', 'v', 0xC4, 0x83};
static const symbol s_2_45[6] = {'i', 't', 'i', 'v', 0xC4, 0x83};

static const struct among a_2[46] = {
    /*  0 */ {5, s_2_0, -1, 4, 0},
    /*  1 */ {5, s_2_1, -1, 4, 0},
    /*  2 */ {5, s_2_2, -1, 5, 0},
    /*  3 */ {5, s_2_3, -1, 6, 0},
    /*  4 */ {5, s_2_4, -1, 4, 0},
    /*  5 */ {7, s_2_5, -1, 5, 0},
    /*  6 */ {7, s_2_6, -1, 6, 0},
    /*  7 */ {6, s_2_7, -1, 5, 0},
    /*  8 */ {6, s_2_8, -1, 6, 0},
    /*  9 */ {7, s_2_9, -1, 5, 0},
    /* 10 */ {7, s_2_10, -1, 4, 0},
    /* 11 */ {9, s_2_11, -1, 1, 0},
    /* 12 */ {9, s_2_12, -1, 2, 0},
    /* 13 */ {7, s_2_13, -1, 3, 0},
    /* 14 */ {5, s_2_14, -1, 4, 0},
    /* 15 */ {5, s_2_15, -1, 5, 0},
    /* 16 */ {5, s_2_16, -1, 6, 0},
    /* 17 */ {5, s_2_17, -1, 4, 0},
    /* 18 */ {5, s_2_18, -1, 5, 0},
    /* 19 */ {7, s_2_19, 18, 4, 0},
    /* 20 */ {5, s_2_20, -1, 6, 0},
    /* 21 */ {6, s_2_21, -1, 5, 0},
    /* 22 */ {7, s_2_22, -1, 4, 0},
    /* 23 */ {9, s_2_23, -1, 1, 0},
    /* 24 */ {7, s_2_24, -1, 3, 0},
    /* 25 */ {5, s_2_25, -1, 4, 0},
    /* 26 */ {5, s_2_26, -1, 5, 0},
    /* 27 */ {5, s_2_27, -1, 6, 0},
    /* 28 */ {7, s_2_28, -1, 4, 0},
    /* 29 */ {9, s_2_29, -1, 1, 0},
    /* 30 */ {7, s_2_30, -1, 3, 0},
    /* 31 */ {9, s_2_31, -1, 4, 0},
    /* 32 */ {11, s_2_32, -1, 1, 0},
    /* 33 */ {9, s_2_33, -1, 3, 0},
    /* 34 */ {4, s_2_34, -1, 4, 0},
    /* 35 */ {4, s_2_35, -1, 5, 0},
    /* 36 */ {6, s_2_36, 35, 4, 0},
    /* 37 */ {4, s_2_37, -1, 6, 0},
    /* 38 */ {5, s_2_38, -1, 5, 0},
    /* 39 */ {4, s_2_39, -1, 4, 0},
    /* 40 */ {4, s_2_40, -1, 5, 0},
    /* 41 */ {4, s_2_41, -1, 6, 0},
    /* 42 */ {6, s_2_42, -1, 4, 0},
    /* 43 */ {6, s_2_43, -1, 4, 0},
    /* 44 */ {6, s_2_44, -1, 5, 0},
    /* 45 */ {6, s_2_45, -1, 6, 0}};

static const symbol s_3_0[3] = {'i', 'c', 'a'};
static const symbol s_3_1[5] = {'a', 'b', 'i', 'l', 'a'};
static const symbol s_3_2[5] = {'i', 'b', 'i', 'l', 'a'};
static const symbol s_3_3[4] = {'o', 'a', 's', 'a'};
static const symbol s_3_4[3] = {'a', 't', 'a'};
static const symbol s_3_5[3] = {'i', 't', 'a'};
static const symbol s_3_6[4] = {'a', 'n', 't', 'a'};
static const symbol s_3_7[4] = {'i', 's', 't', 'a'};
static const symbol s_3_8[3] = {'u', 't', 'a'};
static const symbol s_3_9[3] = {'i', 'v', 'a'};
static const symbol s_3_10[2] = {'i', 'c'};
static const symbol s_3_11[3] = {'i', 'c', 'e'};
static const symbol s_3_12[5] = {'a', 'b', 'i', 'l', 'e'};
static const symbol s_3_13[5] = {'i', 'b', 'i', 'l', 'e'};
static const symbol s_3_14[4] = {'i', 's', 'm', 'e'};
static const symbol s_3_15[4] = {'i', 'u', 'n', 'e'};
static const symbol s_3_16[4] = {'o', 'a', 's', 'e'};
static const symbol s_3_17[3] = {'a', 't', 'e'};
static const symbol s_3_18[5] = {'i', 't', 'a', 't', 'e'};
static const symbol s_3_19[3] = {'i', 't', 'e'};
static const symbol s_3_20[4] = {'a', 'n', 't', 'e'};
static const symbol s_3_21[4] = {'i', 's', 't', 'e'};
static const symbol s_3_22[3] = {'u', 't', 'e'};
static const symbol s_3_23[3] = {'i', 'v', 'e'};
static const symbol s_3_24[3] = {'i', 'c', 'i'};
static const symbol s_3_25[5] = {'a', 'b', 'i', 'l', 'i'};
static const symbol s_3_26[5] = {'i', 'b', 'i', 'l', 'i'};
static const symbol s_3_27[4] = {'i', 'u', 'n', 'i'};
static const symbol s_3_28[5] = {'a', 't', 'o', 'r', 'i'};
static const symbol s_3_29[3] = {'o', 's', 'i'};
static const symbol s_3_30[3] = {'a', 't', 'i'};
static const symbol s_3_31[5] = {'i', 't', 'a', 't', 'i'};
static const symbol s_3_32[3] = {'i', 't', 'i'};
static const symbol s_3_33[4] = {'a', 'n', 't', 'i'};
static const symbol s_3_34[4] = {'i', 's', 't', 'i'};
static const symbol s_3_35[3] = {'u', 't', 'i'};
static const symbol s_3_36[5] = {'i', 0xC5, 0x9F, 't', 'i'};
static const symbol s_3_37[3] = {'i', 'v', 'i'};
static const symbol s_3_38[5] = {'i', 't', 0xC4, 0x83, 'i'};
static const symbol s_3_39[4] = {'o', 0xC5, 0x9F, 'i'};
static const symbol s_3_40[7] = {'i', 't', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_3_41[4] = {'a', 'b', 'i', 'l'};
static const symbol s_3_42[4] = {'i', 'b', 'i', 'l'};
static const symbol s_3_43[3] = {'i', 's', 'm'};
static const symbol s_3_44[4] = {'a', 't', 'o', 'r'};
static const symbol s_3_45[2] = {'o', 's'};
static const symbol s_3_46[2] = {'a', 't'};
static const symbol s_3_47[2] = {'i', 't'};
static const symbol s_3_48[3] = {'a', 'n', 't'};
static const symbol s_3_49[3] = {'i', 's', 't'};
static const symbol s_3_50[2] = {'u', 't'};
static const symbol s_3_51[2] = {'i', 'v'};
static const symbol s_3_52[4] = {'i', 'c', 0xC4, 0x83};
static const symbol s_3_53[6] = {'a', 'b', 'i', 'l', 0xC4, 0x83};
static const symbol s_3_54[6] = {'i', 'b', 'i', 'l', 0xC4, 0x83};
static const symbol s_3_55[5] = {'o', 'a', 's', 0xC4, 0x83};
static const symbol s_3_56[4] = {'a', 't', 0xC4, 0x83};
static const symbol s_3_57[4] = {'i', 't', 0xC4, 0x83};
static const symbol s_3_58[5] = {'a', 'n', 't', 0xC4, 0x83};
static const symbol s_3_59[5] = {'i', 's', 't', 0xC4, 0x83};
static const symbol s_3_60[4] = {'u', 't', 0xC4, 0x83};
static const symbol s_3_61[4] = {'i', 'v', 0xC4, 0x83};

static const struct among a_3[62] = {
    /*  0 */ {3, s_3_0, -1, 1, 0},
    /*  1 */ {5, s_3_1, -1, 1, 0},
    /*  2 */ {5, s_3_2, -1, 1, 0},
    /*  3 */ {4, s_3_3, -1, 1, 0},
    /*  4 */ {3, s_3_4, -1, 1, 0},
    /*  5 */ {3, s_3_5, -1, 1, 0},
    /*  6 */ {4, s_3_6, -1, 1, 0},
    /*  7 */ {4, s_3_7, -1, 3, 0},
    /*  8 */ {3, s_3_8, -1, 1, 0},
    /*  9 */ {3, s_3_9, -1, 1, 0},
    /* 10 */ {2, s_3_10, -1, 1, 0},
    /* 11 */ {3, s_3_11, -1, 1, 0},
    /* 12 */ {5, s_3_12, -1, 1, 0},
    /* 13 */ {5, s_3_13, -1, 1, 0},
    /* 14 */ {4, s_3_14, -1, 3, 0},
    /* 15 */ {4, s_3_15, -1, 2, 0},
    /* 16 */ {4, s_3_16, -1, 1, 0},
    /* 17 */ {3, s_3_17, -1, 1, 0},
    /* 18 */ {5, s_3_18, 17, 1, 0},
    /* 19 */ {3, s_3_19, -1, 1, 0},
    /* 20 */ {4, s_3_20, -1, 1, 0},
    /* 21 */ {4, s_3_21, -1, 3, 0},
    /* 22 */ {3, s_3_22, -1, 1, 0},
    /* 23 */ {3, s_3_23, -1, 1, 0},
    /* 24 */ {3, s_3_24, -1, 1, 0},
    /* 25 */ {5, s_3_25, -1, 1, 0},
    /* 26 */ {5, s_3_26, -1, 1, 0},
    /* 27 */ {4, s_3_27, -1, 2, 0},
    /* 28 */ {5, s_3_28, -1, 1, 0},
    /* 29 */ {3, s_3_29, -1, 1, 0},
    /* 30 */ {3, s_3_30, -1, 1, 0},
    /* 31 */ {5, s_3_31, 30, 1, 0},
    /* 32 */ {3, s_3_32, -1, 1, 0},
    /* 33 */ {4, s_3_33, -1, 1, 0},
    /* 34 */ {4, s_3_34, -1, 3, 0},
    /* 35 */ {3, s_3_35, -1, 1, 0},
    /* 36 */ {5, s_3_36, -1, 3, 0},
    /* 37 */ {3, s_3_37, -1, 1, 0},
    /* 38 */ {5, s_3_38, -1, 1, 0},
    /* 39 */ {4, s_3_39, -1, 1, 0},
    /* 40 */ {7, s_3_40, -1, 1, 0},
    /* 41 */ {4, s_3_41, -1, 1, 0},
    /* 42 */ {4, s_3_42, -1, 1, 0},
    /* 43 */ {3, s_3_43, -1, 3, 0},
    /* 44 */ {4, s_3_44, -1, 1, 0},
    /* 45 */ {2, s_3_45, -1, 1, 0},
    /* 46 */ {2, s_3_46, -1, 1, 0},
    /* 47 */ {2, s_3_47, -1, 1, 0},
    /* 48 */ {3, s_3_48, -1, 1, 0},
    /* 49 */ {3, s_3_49, -1, 3, 0},
    /* 50 */ {2, s_3_50, -1, 1, 0},
    /* 51 */ {2, s_3_51, -1, 1, 0},
    /* 52 */ {4, s_3_52, -1, 1, 0},
    /* 53 */ {6, s_3_53, -1, 1, 0},
    /* 54 */ {6, s_3_54, -1, 1, 0},
    /* 55 */ {5, s_3_55, -1, 1, 0},
    /* 56 */ {4, s_3_56, -1, 1, 0},
    /* 57 */ {4, s_3_57, -1, 1, 0},
    /* 58 */ {5, s_3_58, -1, 1, 0},
    /* 59 */ {5, s_3_59, -1, 3, 0},
    /* 60 */ {4, s_3_60, -1, 1, 0},
    /* 61 */ {4, s_3_61, -1, 1, 0}};

static const symbol s_4_0[2] = {'e', 'a'};
static const symbol s_4_1[2] = {'i', 'a'};
static const symbol s_4_2[3] = {'e', 's', 'c'};
static const symbol s_4_3[4] = {0xC4, 0x83, 's', 'c'};
static const symbol s_4_4[3] = {'i', 'n', 'd'};
static const symbol s_4_5[4] = {0xC3, 0xA2, 'n', 'd'};
static const symbol s_4_6[3] = {'a', 'r', 'e'};
static const symbol s_4_7[3] = {'e', 'r', 'e'};
static const symbol s_4_8[3] = {'i', 'r', 'e'};
static const symbol s_4_9[4] = {0xC3, 0xA2, 'r', 'e'};
static const symbol s_4_10[2] = {'s', 'e'};
static const symbol s_4_11[3] = {'a', 's', 'e'};
static const symbol s_4_12[4] = {'s', 'e', 's', 'e'};
static const symbol s_4_13[3] = {'i', 's', 'e'};
static const symbol s_4_14[3] = {'u', 's', 'e'};
static const symbol s_4_15[4] = {0xC3, 0xA2, 's', 'e'};
static const symbol s_4_16[5] = {'e', 0xC5, 0x9F, 't', 'e'};
static const symbol s_4_17[6] = {0xC4, 0x83, 0xC5, 0x9F, 't', 'e'};
static const symbol s_4_18[3] = {'e', 'z', 'e'};
static const symbol s_4_19[2] = {'a', 'i'};
static const symbol s_4_20[3] = {'e', 'a', 'i'};
static const symbol s_4_21[3] = {'i', 'a', 'i'};
static const symbol s_4_22[3] = {'s', 'e', 'i'};
static const symbol s_4_23[5] = {'e', 0xC5, 0x9F, 't', 'i'};
static const symbol s_4_24[6] = {0xC4, 0x83, 0xC5, 0x9F, 't', 'i'};
static const symbol s_4_25[2] = {'u', 'i'};
static const symbol s_4_26[3] = {'e', 'z', 'i'};
static const symbol s_4_27[4] = {'a', 0xC5, 0x9F, 'i'};
static const symbol s_4_28[5] = {'s', 'e', 0xC5, 0x9F, 'i'};
static const symbol s_4_29[6] = {'a', 's', 'e', 0xC5, 0x9F, 'i'};
static const symbol s_4_30[7] = {'s', 'e', 's', 'e', 0xC5, 0x9F, 'i'};
static const symbol s_4_31[6] = {'i', 's', 'e', 0xC5, 0x9F, 'i'};
static const symbol s_4_32[6] = {'u', 's', 'e', 0xC5, 0x9F, 'i'};
static const symbol s_4_33[7] = {0xC3, 0xA2, 's', 'e', 0xC5, 0x9F, 'i'};
static const symbol s_4_34[4] = {'i', 0xC5, 0x9F, 'i'};
static const symbol s_4_35[4] = {'u', 0xC5, 0x9F, 'i'};
static const symbol s_4_36[5] = {0xC3, 0xA2, 0xC5, 0x9F, 'i'};
static const symbol s_4_37[3] = {0xC3, 0xA2, 'i'};
static const symbol s_4_38[4] = {'a', 0xC5, 0xA3, 'i'};
static const symbol s_4_39[5] = {'e', 'a', 0xC5, 0xA3, 'i'};
static const symbol s_4_40[5] = {'i', 'a', 0xC5, 0xA3, 'i'};
static const symbol s_4_41[4] = {'e', 0xC5, 0xA3, 'i'};
static const symbol s_4_42[4] = {'i', 0xC5, 0xA3, 'i'};
static const symbol s_4_43[7] = {'a', 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_44[8] = {'s', 'e', 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_45[9] = {'a', 's', 'e', 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_46[10] = {'s', 'e', 's', 'e', 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_47[9] = {'i', 's', 'e', 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_48[9] = {'u', 's', 'e', 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_49[10] = {0xC3, 0xA2, 's', 'e', 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_50[7] = {'i', 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_51[7] = {'u', 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_52[8] = {0xC3, 0xA2, 'r', 0xC4, 0x83, 0xC5, 0xA3, 'i'};
static const symbol s_4_53[5] = {0xC3, 0xA2, 0xC5, 0xA3, 'i'};
static const symbol s_4_54[2] = {'a', 'm'};
static const symbol s_4_55[3] = {'e', 'a', 'm'};
static const symbol s_4_56[3] = {'i', 'a', 'm'};
static const symbol s_4_57[2] = {'e', 'm'};
static const symbol s_4_58[4] = {'a', 's', 'e', 'm'};
static const symbol s_4_59[5] = {'s', 'e', 's', 'e', 'm'};
static const symbol s_4_60[4] = {'i', 's', 'e', 'm'};
static const symbol s_4_61[4] = {'u', 's', 'e', 'm'};
static const symbol s_4_62[5] = {0xC3, 0xA2, 's', 'e', 'm'};
static const symbol s_4_63[2] = {'i', 'm'};
static const symbol s_4_64[3] = {0xC4, 0x83, 'm'};
static const symbol s_4_65[5] = {'a', 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_66[6] = {'s', 'e', 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_67[7] = {'a', 's', 'e', 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_68[8] = {'s', 'e', 's', 'e', 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_69[7] = {'i', 's', 'e', 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_70[7] = {'u', 's', 'e', 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_71[8] = {0xC3, 0xA2, 's', 'e', 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_72[5] = {'i', 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_73[5] = {'u', 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_74[6] = {0xC3, 0xA2, 'r', 0xC4, 0x83, 'm'};
static const symbol s_4_75[3] = {0xC3, 0xA2, 'm'};
static const symbol s_4_76[2] = {'a', 'u'};
static const symbol s_4_77[3] = {'e', 'a', 'u'};
static const symbol s_4_78[3] = {'i', 'a', 'u'};
static const symbol s_4_79[4] = {'i', 'n', 'd', 'u'};
static const symbol s_4_80[5] = {0xC3, 0xA2, 'n', 'd', 'u'};
static const symbol s_4_81[2] = {'e', 'z'};
static const symbol s_4_82[6] = {'e', 'a', 's', 'c', 0xC4, 0x83};
static const symbol s_4_83[4] = {'a', 'r', 0xC4, 0x83};
static const symbol s_4_84[5] = {'s', 'e', 'r', 0xC4, 0x83};
static const symbol s_4_85[6] = {'a', 's', 'e', 'r', 0xC4, 0x83};
static const symbol s_4_86[7] = {'s', 'e', 's', 'e', 'r', 0xC4, 0x83};
static const symbol s_4_87[6] = {'i', 's', 'e', 'r', 0xC4, 0x83};
static const symbol s_4_88[6] = {'u', 's', 'e', 'r', 0xC4, 0x83};
static const symbol s_4_89[7] = {0xC3, 0xA2, 's', 'e', 'r', 0xC4, 0x83};
static const symbol s_4_90[4] = {'i', 'r', 0xC4, 0x83};
static const symbol s_4_91[4] = {'u', 'r', 0xC4, 0x83};
static const symbol s_4_92[5] = {0xC3, 0xA2, 'r', 0xC4, 0x83};
static const symbol s_4_93[5] = {'e', 'a', 'z', 0xC4, 0x83};

static const struct among a_4[94] = {
    /*  0 */ {2, s_4_0, -1, 1, 0},
    /*  1 */ {2, s_4_1, -1, 1, 0},
    /*  2 */ {3, s_4_2, -1, 1, 0},
    /*  3 */ {4, s_4_3, -1, 1, 0},
    /*  4 */ {3, s_4_4, -1, 1, 0},
    /*  5 */ {4, s_4_5, -1, 1, 0},
    /*  6 */ {3, s_4_6, -1, 1, 0},
    /*  7 */ {3, s_4_7, -1, 1, 0},
    /*  8 */ {3, s_4_8, -1, 1, 0},
    /*  9 */ {4, s_4_9, -1, 1, 0},
    /* 10 */ {2, s_4_10, -1, 2, 0},
    /* 11 */ {3, s_4_11, 10, 1, 0},
    /* 12 */ {4, s_4_12, 10, 2, 0},
    /* 13 */ {3, s_4_13, 10, 1, 0},
    /* 14 */ {3, s_4_14, 10, 1, 0},
    /* 15 */ {4, s_4_15, 10, 1, 0},
    /* 16 */ {5, s_4_16, -1, 1, 0},
    /* 17 */ {6, s_4_17, -1, 1, 0},
    /* 18 */ {3, s_4_18, -1, 1, 0},
    /* 19 */ {2, s_4_19, -1, 1, 0},
    /* 20 */ {3, s_4_20, 19, 1, 0},
    /* 21 */ {3, s_4_21, 19, 1, 0},
    /* 22 */ {3, s_4_22, -1, 2, 0},
    /* 23 */ {5, s_4_23, -1, 1, 0},
    /* 24 */ {6, s_4_24, -1, 1, 0},
    /* 25 */ {2, s_4_25, -1, 1, 0},
    /* 26 */ {3, s_4_26, -1, 1, 0},
    /* 27 */ {4, s_4_27, -1, 1, 0},
    /* 28 */ {5, s_4_28, -1, 2, 0},
    /* 29 */ {6, s_4_29, 28, 1, 0},
    /* 30 */ {7, s_4_30, 28, 2, 0},
    /* 31 */ {6, s_4_31, 28, 1, 0},
    /* 32 */ {6, s_4_32, 28, 1, 0},
    /* 33 */ {7, s_4_33, 28, 1, 0},
    /* 34 */ {4, s_4_34, -1, 1, 0},
    /* 35 */ {4, s_4_35, -1, 1, 0},
    /* 36 */ {5, s_4_36, -1, 1, 0},
    /* 37 */ {3, s_4_37, -1, 1, 0},
    /* 38 */ {4, s_4_38, -1, 2, 0},
    /* 39 */ {5, s_4_39, 38, 1, 0},
    /* 40 */ {5, s_4_40, 38, 1, 0},
    /* 41 */ {4, s_4_41, -1, 2, 0},
    /* 42 */ {4, s_4_42, -1, 2, 0},
    /* 43 */ {7, s_4_43, -1, 1, 0},
    /* 44 */ {8, s_4_44, -1, 2, 0},
    /* 45 */ {9, s_4_45, 44, 1, 0},
    /* 46 */ {10, s_4_46, 44, 2, 0},
    /* 47 */ {9, s_4_47, 44, 1, 0},
    /* 48 */ {9, s_4_48, 44, 1, 0},
    /* 49 */ {10, s_4_49, 44, 1, 0},
    /* 50 */ {7, s_4_50, -1, 1, 0},
    /* 51 */ {7, s_4_51, -1, 1, 0},
    /* 52 */ {8, s_4_52, -1, 1, 0},
    /* 53 */ {5, s_4_53, -1, 2, 0},
    /* 54 */ {2, s_4_54, -1, 1, 0},
    /* 55 */ {3, s_4_55, 54, 1, 0},
    /* 56 */ {3, s_4_56, 54, 1, 0},
    /* 57 */ {2, s_4_57, -1, 2, 0},
    /* 58 */ {4, s_4_58, 57, 1, 0},
    /* 59 */ {5, s_4_59, 57, 2, 0},
    /* 60 */ {4, s_4_60, 57, 1, 0},
    /* 61 */ {4, s_4_61, 57, 1, 0},
    /* 62 */ {5, s_4_62, 57, 1, 0},
    /* 63 */ {2, s_4_63, -1, 2, 0},
    /* 64 */ {3, s_4_64, -1, 2, 0},
    /* 65 */ {5, s_4_65, 64, 1, 0},
    /* 66 */ {6, s_4_66, 64, 2, 0},
    /* 67 */ {7, s_4_67, 66, 1, 0},
    /* 68 */ {8, s_4_68, 66, 2, 0},
    /* 69 */ {7, s_4_69, 66, 1, 0},
    /* 70 */ {7, s_4_70, 66, 1, 0},
    /* 71 */ {8, s_4_71, 66, 1, 0},
    /* 72 */ {5, s_4_72, 64, 1, 0},
    /* 73 */ {5, s_4_73, 64, 1, 0},
    /* 74 */ {6, s_4_74, 64, 1, 0},
    /* 75 */ {3, s_4_75, -1, 2, 0},
    /* 76 */ {2, s_4_76, -1, 1, 0},
    /* 77 */ {3, s_4_77, 76, 1, 0},
    /* 78 */ {3, s_4_78, 76, 1, 0},
    /* 79 */ {4, s_4_79, -1, 1, 0},
    /* 80 */ {5, s_4_80, -1, 1, 0},
    /* 81 */ {2, s_4_81, -1, 1, 0},
    /* 82 */ {6, s_4_82, -1, 1, 0},
    /* 83 */ {4, s_4_83, -1, 1, 0},
    /* 84 */ {5, s_4_84, -1, 2, 0},
    /* 85 */ {6, s_4_85, 84, 1, 0},
    /* 86 */ {7, s_4_86, 84, 2, 0},
    /* 87 */ {6, s_4_87, 84, 1, 0},
    /* 88 */ {6, s_4_88, 84, 1, 0},
    /* 89 */ {7, s_4_89, 84, 1, 0},
    /* 90 */ {4, s_4_90, -1, 1, 0},
    /* 91 */ {4, s_4_91, -1, 1, 0},
    /* 92 */ {5, s_4_92, -1, 1, 0},
    /* 93 */ {5, s_4_93, -1, 1, 0}};

static const symbol s_5_0[1] = {'a'};
static const symbol s_5_1[1] = {'e'};
static const symbol s_5_2[2] = {'i', 'e'};
static const symbol s_5_3[1] = {'i'};
static const symbol s_5_4[2] = {0xC4, 0x83};

static const struct among a_5[5] = {
    /*  0 */ {1, s_5_0, -1, 1, 0},
    /*  1 */ {1, s_5_1, -1, 1, 0},
    /*  2 */ {2, s_5_2, 1, 1, 0},
    /*  3 */ {1, s_5_3, -1, 1, 0},
    /*  4 */ {2, s_5_4, -1, 1, 0}};

static const unsigned char g_v[] = {17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 32, 0, 0, 4};

static const symbol s_0[] = {'u'};
static const symbol s_1[] = {'U'};
static const symbol s_2[] = {'i'};
static const symbol s_3[] = {'I'};
static const symbol s_4[] = {'i'};
static const symbol s_5[] = {'u'};
static const symbol s_6[] = {'a'};
static const symbol s_7[] = {'e'};
static const symbol s_8[] = {'i'};
static const symbol s_9[] = {'a', 'b'};
static const symbol s_10[] = {'i'};
static const symbol s_11[] = {'a', 't'};
static const symbol s_12[] = {'a', 0xC5, 0xA3, 'i'};
static const symbol s_13[] = {'a', 'b', 'i', 'l'};
static const symbol s_14[] = {'i', 'b', 'i', 'l'};
static const symbol s_15[] = {'i', 'v'};
static const symbol s_16[] = {'i', 'c'};
static const symbol s_17[] = {'a', 't'};
static const symbol s_18[] = {'i', 't'};
static const symbol s_19[] = {0xC5, 0xA3};
static const symbol s_20[] = {'t'};
static const symbol s_21[] = {'i', 's', 't'};
static const symbol s_22[] = {'u'};

static int r_prelude(struct SN_env *z) {
    while (1) { /* repeat, line 32 */
        int c1 = z->c;
        while (1) { /* goto, line 32 */
            int c2 = z->c;
            if (in_grouping_U(z, g_v, 97, 259, 0))
                goto lab1;
            z->bra = z->c; /* [, line 33 */
            {
                int c3 = z->c; /* or, line 33 */
                if (!(eq_s(z, 1, s_0)))
                    goto lab3;
                z->ket = z->c; /* ], line 33 */
                if (in_grouping_U(z, g_v, 97, 259, 0))
                    goto lab3;
                {
                    int ret = slice_from_s(z, 1, s_1); /* <-, line 33 */
                    if (ret < 0)
                        return ret;
                }
                goto lab2;
            lab3:
                z->c = c3;
                if (!(eq_s(z, 1, s_2)))
                    goto lab1;
                z->ket = z->c; /* ], line 34 */
                if (in_grouping_U(z, g_v, 97, 259, 0))
                    goto lab1;
                {
                    int ret = slice_from_s(z, 1, s_3); /* <-, line 34 */
                    if (ret < 0)
                        return ret;
                }
            }
        lab2:
            z->c = c2;
            break;
        lab1:
            z->c = c2;
            {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    goto lab0;
                z->c = ret; /* goto, line 32 */
            }
        }
        continue;
    lab0:
        z->c = c1;
        break;
    }
    return 1;
}

static int r_mark_regions(struct SN_env *z) {
    z->I[0] = z->l;
    z->I[1] = z->l;
    z->I[2] = z->l;
    {
        int c1 = z->c; /* do, line 44 */
        {
            int c2 = z->c; /* or, line 46 */
            if (in_grouping_U(z, g_v, 97, 259, 0))
                goto lab2;
            {
                int c3 = z->c; /* or, line 45 */
                if (out_grouping_U(z, g_v, 97, 259, 0))
                    goto lab4;
                { /* gopast */ /* grouping v, line 45 */
                    int ret = out_grouping_U(z, g_v, 97, 259, 1);
                    if (ret < 0)
                        goto lab4;
                    z->c += ret;
                }
                goto lab3;
            lab4:
                z->c = c3;
                if (in_grouping_U(z, g_v, 97, 259, 0))
                    goto lab2;
                { /* gopast */ /* non v, line 45 */
                    int ret = in_grouping_U(z, g_v, 97, 259, 1);
                    if (ret < 0)
                        goto lab2;
                    z->c += ret;
                }
            }
        lab3:
            goto lab1;
        lab2:
            z->c = c2;
            if (out_grouping_U(z, g_v, 97, 259, 0))
                goto lab0;
            {
                int c4 = z->c; /* or, line 47 */
                if (out_grouping_U(z, g_v, 97, 259, 0))
                    goto lab6;
                { /* gopast */ /* grouping v, line 47 */
                    int ret = out_grouping_U(z, g_v, 97, 259, 1);
                    if (ret < 0)
                        goto lab6;
                    z->c += ret;
                }
                goto lab5;
            lab6:
                z->c = c4;
                if (in_grouping_U(z, g_v, 97, 259, 0))
                    goto lab0;
                {
                    int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                    if (ret < 0)
                        goto lab0;
                    z->c = ret; /* next, line 47 */
                }
            }
        lab5:;
        }
    lab1:
        z->I[0] = z->c; /* setmark pV, line 48 */
    lab0:
        z->c = c1;
    }
    {
        int c5 = z->c; /* do, line 50 */
        { /* gopast */ /* grouping v, line 51 */
            int ret = out_grouping_U(z, g_v, 97, 259, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        { /* gopast */ /* non v, line 51 */
            int ret = in_grouping_U(z, g_v, 97, 259, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        z->I[1] = z->c; /* setmark p1, line 51 */
        { /* gopast */  /* grouping v, line 52 */
            int ret = out_grouping_U(z, g_v, 97, 259, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        { /* gopast */ /* non v, line 52 */
            int ret = in_grouping_U(z, g_v, 97, 259, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        z->I[2] = z->c; /* setmark p2, line 52 */
    lab7:
        z->c = c5;
    }
    return 1;
}

static int r_postlude(struct SN_env *z) {
    int among_var;
    while (1) { /* repeat, line 56 */
        int c1 = z->c;
        z->bra = z->c; /* [, line 58 */
        if (z->c >= z->l || (z->p[z->c + 0] != 73 && z->p[z->c + 0] != 85))
            among_var = 3;
        else
            among_var = find_among(z, a_0, 3); /* substring, line 58 */
        if (!(among_var))
            goto lab0;
        z->ket = z->c; /* ], line 58 */
        switch (among_var) {
            case 0:
                goto lab0;
            case 1: {
                int ret = slice_from_s(z, 1, s_4); /* <-, line 59 */
                if (ret < 0)
                    return ret;
            } break;
            case 2: {
                int ret = slice_from_s(z, 1, s_5); /* <-, line 60 */
                if (ret < 0)
                    return ret;
            } break;
            case 3: {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    goto lab0;
                z->c = ret; /* next, line 61 */
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

static int r_step_0(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 73 */
    if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((266786 >> (z->p[z->c - 1] & 0x1f)) & 1))
        return 0;
    among_var = find_among_b(z, a_1, 16); /* substring, line 73 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 73 */
    {
        int ret = r_R1(z);
        if (ret == 0)
            return 0; /* call R1, line 73 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 75 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_from_s(z, 1, s_6); /* <-, line 77 */
            if (ret < 0)
                return ret;
        } break;
        case 3: {
            int ret = slice_from_s(z, 1, s_7); /* <-, line 79 */
            if (ret < 0)
                return ret;
        } break;
        case 4: {
            int ret = slice_from_s(z, 1, s_8); /* <-, line 81 */
            if (ret < 0)
                return ret;
        } break;
        case 5: {
            int m1 = z->l - z->c;
            (void)m1; /* not, line 83 */
            if (!(eq_s_b(z, 2, s_9)))
                goto lab0;
            return 0;
        lab0:
            z->c = z->l - m1;
        }
            {
                int ret = slice_from_s(z, 1, s_10); /* <-, line 83 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 6: {
            int ret = slice_from_s(z, 2, s_11); /* <-, line 85 */
            if (ret < 0)
                return ret;
        } break;
        case 7: {
            int ret = slice_from_s(z, 4, s_12); /* <-, line 87 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_combo_suffix(struct SN_env *z) {
    int among_var;
    {
        int m_test = z->l - z->c;             /* test, line 91 */
        z->ket = z->c;                        /* [, line 92 */
        among_var = find_among_b(z, a_2, 46); /* substring, line 92 */
        if (!(among_var))
            return 0;
        z->bra = z->c; /* ], line 92 */
        {
            int ret = r_R1(z);
            if (ret == 0)
                return 0; /* call R1, line 92 */
            if (ret < 0)
                return ret;
        }
        switch (among_var) {
            case 0:
                return 0;
            case 1: {
                int ret = slice_from_s(z, 4, s_13); /* <-, line 101 */
                if (ret < 0)
                    return ret;
            } break;
            case 2: {
                int ret = slice_from_s(z, 4, s_14); /* <-, line 104 */
                if (ret < 0)
                    return ret;
            } break;
            case 3: {
                int ret = slice_from_s(z, 2, s_15); /* <-, line 107 */
                if (ret < 0)
                    return ret;
            } break;
            case 4: {
                int ret = slice_from_s(z, 2, s_16); /* <-, line 113 */
                if (ret < 0)
                    return ret;
            } break;
            case 5: {
                int ret = slice_from_s(z, 2, s_17); /* <-, line 118 */
                if (ret < 0)
                    return ret;
            } break;
            case 6: {
                int ret = slice_from_s(z, 2, s_18); /* <-, line 122 */
                if (ret < 0)
                    return ret;
            } break;
        }
        z->B[0] = 1; /* set standard_suffix_removed, line 125 */
        z->c = z->l - m_test;
    }
    return 1;
}

static int r_standard_suffix(struct SN_env *z) {
    int among_var;
    z->B[0] = 0; /* unset standard_suffix_removed, line 130 */
    while (1) {  /* repeat, line 131 */
        int m1 = z->l - z->c;
        (void)m1;
        {
            int ret = r_combo_suffix(z);
            if (ret == 0)
                goto lab0; /* call combo_suffix, line 131 */
            if (ret < 0)
                return ret;
        }
        continue;
    lab0:
        z->c = z->l - m1;
        break;
    }
    z->ket = z->c;                        /* [, line 132 */
    among_var = find_among_b(z, a_3, 62); /* substring, line 132 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 132 */
    {
        int ret = r_R2(z);
        if (ret == 0)
            return 0; /* call R2, line 132 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 149 */
            if (ret < 0)
                return ret;
        } break;
        case 2:
            if (!(eq_s_b(z, 2, s_19)))
                return 0;
            z->bra = z->c; /* ], line 152 */
            {
                int ret = slice_from_s(z, 1, s_20); /* <-, line 152 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 3: {
            int ret = slice_from_s(z, 3, s_21); /* <-, line 156 */
            if (ret < 0)
                return ret;
        } break;
    }
    z->B[0] = 1; /* set standard_suffix_removed, line 160 */
    return 1;
}

static int r_verb_suffix(struct SN_env *z) {
    int among_var;
    {
        int mlimit; /* setlimit, line 164 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 164 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c;                        /* [, line 165 */
        among_var = find_among_b(z, a_4, 94); /* substring, line 165 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 165 */
        switch (among_var) {
            case 0: {
                z->lb = mlimit;
                return 0;
            }
            case 1: {
                int m2 = z->l - z->c;
                (void)m2; /* or, line 200 */
                if (out_grouping_b_U(z, g_v, 97, 259, 0))
                    goto lab1;
                goto lab0;
            lab1:
                z->c = z->l - m2;
                if (!(eq_s_b(z, 1, s_22))) {
                    z->lb = mlimit;
                    return 0;
                }
            }
            lab0: {
                int ret = slice_del(z); /* delete, line 200 */
                if (ret < 0)
                    return ret;
            } break;
            case 2: {
                int ret = slice_del(z); /* delete, line 214 */
                if (ret < 0)
                    return ret;
            } break;
        }
        z->lb = mlimit;
    }
    return 1;
}

static int r_vowel_suffix(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                       /* [, line 219 */
    among_var = find_among_b(z, a_5, 5); /* substring, line 219 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 219 */
    {
        int ret = r_RV(z);
        if (ret == 0)
            return 0; /* call RV, line 219 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 220 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

extern int romanian_UTF_8_stem(struct SN_env *z) {
    {
        int c1 = z->c; /* do, line 226 */
        {
            int ret = r_prelude(z);
            if (ret == 0)
                goto lab0; /* call prelude, line 226 */
            if (ret < 0)
                return ret;
        }
    lab0:
        z->c = c1;
    }
    {
        int c2 = z->c; /* do, line 227 */
        {
            int ret = r_mark_regions(z);
            if (ret == 0)
                goto lab1; /* call mark_regions, line 227 */
            if (ret < 0)
                return ret;
        }
    lab1:
        z->c = c2;
    }
    z->lb = z->c;
    z->c = z->l; /* backwards, line 228 */

    {
        int m3 = z->l - z->c;
        (void)m3; /* do, line 229 */
        {
            int ret = r_step_0(z);
            if (ret == 0)
                goto lab2; /* call step_0, line 229 */
            if (ret < 0)
                return ret;
        }
    lab2:
        z->c = z->l - m3;
    }
    {
        int m4 = z->l - z->c;
        (void)m4; /* do, line 230 */
        {
            int ret = r_standard_suffix(z);
            if (ret == 0)
                goto lab3; /* call standard_suffix, line 230 */
            if (ret < 0)
                return ret;
        }
    lab3:
        z->c = z->l - m4;
    }
    {
        int m5 = z->l - z->c;
        (void)m5; /* do, line 231 */
        {
            int m6 = z->l - z->c;
            (void)m6; /* or, line 231 */
            if (!(z->B[0]))
                goto lab6; /* Boolean test standard_suffix_removed, line 231 */
            goto lab5;
        lab6:
            z->c = z->l - m6;
            {
                int ret = r_verb_suffix(z);
                if (ret == 0)
                    goto lab4; /* call verb_suffix, line 231 */
                if (ret < 0)
                    return ret;
            }
        }
    lab5:
    lab4:
        z->c = z->l - m5;
    }
    {
        int m7 = z->l - z->c;
        (void)m7; /* do, line 232 */
        {
            int ret = r_vowel_suffix(z);
            if (ret == 0)
                goto lab7; /* call vowel_suffix, line 232 */
            if (ret < 0)
                return ret;
        }
    lab7:
        z->c = z->l - m7;
    }
    z->c = z->lb;
    {
        int c8 = z->c; /* do, line 234 */
        {
            int ret = r_postlude(z);
            if (ret == 0)
                goto lab8; /* call postlude, line 234 */
            if (ret < 0)
                return ret;
        }
    lab8:
        z->c = c8;
    }
    return 1;
}

extern struct SN_env *romanian_UTF_8_create_env(void) { return SN_create_env(0, 3, 1); }

extern void romanian_UTF_8_close_env(struct SN_env *z) { SN_close_env(z, 0); }
