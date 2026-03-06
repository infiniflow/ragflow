
/* This file was generated automatically by the Snowball to ANSI C compiler */

#include "header.h"

#ifdef __cplusplus
extern "C" {
#endif
extern int portuguese_UTF_8_stem(struct SN_env *z);
#ifdef __cplusplus
}
#endif
static int r_residual_form(struct SN_env *z);
static int r_residual_suffix(struct SN_env *z);
static int r_verb_suffix(struct SN_env *z);
static int r_standard_suffix(struct SN_env *z);
static int r_R2(struct SN_env *z);
static int r_R1(struct SN_env *z);
static int r_RV(struct SN_env *z);
static int r_mark_regions(struct SN_env *z);
static int r_postlude(struct SN_env *z);
static int r_prelude(struct SN_env *z);
#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *portuguese_UTF_8_create_env(void);
extern void portuguese_UTF_8_close_env(struct SN_env *z);

#ifdef __cplusplus
}
#endif
static const symbol s_0_1[2] = {0xC3, 0xA3};
static const symbol s_0_2[2] = {0xC3, 0xB5};

static const struct among a_0[3] = {
    /*  0 */ {0, 0, -1, 3, 0},
    /*  1 */ {2, s_0_1, 0, 1, 0},
    /*  2 */ {2, s_0_2, 0, 2, 0}};

static const symbol s_1_1[2] = {'a', '~'};
static const symbol s_1_2[2] = {'o', '~'};

static const struct among a_1[3] = {
    /*  0 */ {0, 0, -1, 3, 0},
    /*  1 */ {2, s_1_1, 0, 1, 0},
    /*  2 */ {2, s_1_2, 0, 2, 0}};

static const symbol s_2_0[2] = {'i', 'c'};
static const symbol s_2_1[2] = {'a', 'd'};
static const symbol s_2_2[2] = {'o', 's'};
static const symbol s_2_3[2] = {'i', 'v'};

static const struct among a_2[4] = {
    /*  0 */ {2, s_2_0, -1, -1, 0},
    /*  1 */ {2, s_2_1, -1, -1, 0},
    /*  2 */ {2, s_2_2, -1, -1, 0},
    /*  3 */ {2, s_2_3, -1, 1, 0}};

static const symbol s_3_0[4] = {'a', 'n', 't', 'e'};
static const symbol s_3_1[4] = {'a', 'v', 'e', 'l'};
static const symbol s_3_2[5] = {0xC3, 0xAD, 'v', 'e', 'l'};

static const struct among a_3[3] = {
    /*  0 */ {4, s_3_0, -1, 1, 0},
    /*  1 */ {4, s_3_1, -1, 1, 0},
    /*  2 */ {5, s_3_2, -1, 1, 0}};

static const symbol s_4_0[2] = {'i', 'c'};
static const symbol s_4_1[4] = {'a', 'b', 'i', 'l'};
static const symbol s_4_2[2] = {'i', 'v'};

static const struct among a_4[3] = {
    /*  0 */ {2, s_4_0, -1, 1, 0},
    /*  1 */ {4, s_4_1, -1, 1, 0},
    /*  2 */ {2, s_4_2, -1, 1, 0}};

static const symbol s_5_0[3] = {'i', 'c', 'a'};
static const symbol s_5_1[6] = {0xC3, 0xA2, 'n', 'c', 'i', 'a'};
static const symbol s_5_2[6] = {0xC3, 0xAA, 'n', 'c', 'i', 'a'};
static const symbol s_5_3[3] = {'i', 'r', 'a'};
static const symbol s_5_4[5] = {'a', 'd', 'o', 'r', 'a'};
static const symbol s_5_5[3] = {'o', 's', 'a'};
static const symbol s_5_6[4] = {'i', 's', 't', 'a'};
static const symbol s_5_7[3] = {'i', 'v', 'a'};
static const symbol s_5_8[3] = {'e', 'z', 'a'};
static const symbol s_5_9[6] = {'l', 'o', 'g', 0xC3, 0xAD, 'a'};
static const symbol s_5_10[5] = {'i', 'd', 'a', 'd', 'e'};
static const symbol s_5_11[4] = {'a', 'n', 't', 'e'};
static const symbol s_5_12[5] = {'m', 'e', 'n', 't', 'e'};
static const symbol s_5_13[6] = {'a', 'm', 'e', 'n', 't', 'e'};
static const symbol s_5_14[5] = {0xC3, 0xA1, 'v', 'e', 'l'};
static const symbol s_5_15[5] = {0xC3, 0xAD, 'v', 'e', 'l'};
static const symbol s_5_16[6] = {'u', 'c', 'i', 0xC3, 0xB3, 'n'};
static const symbol s_5_17[3] = {'i', 'c', 'o'};
static const symbol s_5_18[4] = {'i', 's', 'm', 'o'};
static const symbol s_5_19[3] = {'o', 's', 'o'};
static const symbol s_5_20[6] = {'a', 'm', 'e', 'n', 't', 'o'};
static const symbol s_5_21[6] = {'i', 'm', 'e', 'n', 't', 'o'};
static const symbol s_5_22[3] = {'i', 'v', 'o'};
static const symbol s_5_23[6] = {'a', 0xC3, 0xA7, 'a', '~', 'o'};
static const symbol s_5_24[4] = {'a', 'd', 'o', 'r'};
static const symbol s_5_25[4] = {'i', 'c', 'a', 's'};
static const symbol s_5_26[7] = {0xC3, 0xAA, 'n', 'c', 'i', 'a', 's'};
static const symbol s_5_27[4] = {'i', 'r', 'a', 's'};
static const symbol s_5_28[6] = {'a', 'd', 'o', 'r', 'a', 's'};
static const symbol s_5_29[4] = {'o', 's', 'a', 's'};
static const symbol s_5_30[5] = {'i', 's', 't', 'a', 's'};
static const symbol s_5_31[4] = {'i', 'v', 'a', 's'};
static const symbol s_5_32[4] = {'e', 'z', 'a', 's'};
static const symbol s_5_33[7] = {'l', 'o', 'g', 0xC3, 0xAD, 'a', 's'};
static const symbol s_5_34[6] = {'i', 'd', 'a', 'd', 'e', 's'};
static const symbol s_5_35[7] = {'u', 'c', 'i', 'o', 'n', 'e', 's'};
static const symbol s_5_36[6] = {'a', 'd', 'o', 'r', 'e', 's'};
static const symbol s_5_37[5] = {'a', 'n', 't', 'e', 's'};
static const symbol s_5_38[7] = {'a', 0xC3, 0xA7, 'o', '~', 'e', 's'};
static const symbol s_5_39[4] = {'i', 'c', 'o', 's'};
static const symbol s_5_40[5] = {'i', 's', 'm', 'o', 's'};
static const symbol s_5_41[4] = {'o', 's', 'o', 's'};
static const symbol s_5_42[7] = {'a', 'm', 'e', 'n', 't', 'o', 's'};
static const symbol s_5_43[7] = {'i', 'm', 'e', 'n', 't', 'o', 's'};
static const symbol s_5_44[4] = {'i', 'v', 'o', 's'};

static const struct among a_5[45] = {
    /*  0 */ {3, s_5_0, -1, 1, 0},
    /*  1 */ {6, s_5_1, -1, 1, 0},
    /*  2 */ {6, s_5_2, -1, 4, 0},
    /*  3 */ {3, s_5_3, -1, 9, 0},
    /*  4 */ {5, s_5_4, -1, 1, 0},
    /*  5 */ {3, s_5_5, -1, 1, 0},
    /*  6 */ {4, s_5_6, -1, 1, 0},
    /*  7 */ {3, s_5_7, -1, 8, 0},
    /*  8 */ {3, s_5_8, -1, 1, 0},
    /*  9 */ {6, s_5_9, -1, 2, 0},
    /* 10 */ {5, s_5_10, -1, 7, 0},
    /* 11 */ {4, s_5_11, -1, 1, 0},
    /* 12 */ {5, s_5_12, -1, 6, 0},
    /* 13 */ {6, s_5_13, 12, 5, 0},
    /* 14 */ {5, s_5_14, -1, 1, 0},
    /* 15 */ {5, s_5_15, -1, 1, 0},
    /* 16 */ {6, s_5_16, -1, 3, 0},
    /* 17 */ {3, s_5_17, -1, 1, 0},
    /* 18 */ {4, s_5_18, -1, 1, 0},
    /* 19 */ {3, s_5_19, -1, 1, 0},
    /* 20 */ {6, s_5_20, -1, 1, 0},
    /* 21 */ {6, s_5_21, -1, 1, 0},
    /* 22 */ {3, s_5_22, -1, 8, 0},
    /* 23 */ {6, s_5_23, -1, 1, 0},
    /* 24 */ {4, s_5_24, -1, 1, 0},
    /* 25 */ {4, s_5_25, -1, 1, 0},
    /* 26 */ {7, s_5_26, -1, 4, 0},
    /* 27 */ {4, s_5_27, -1, 9, 0},
    /* 28 */ {6, s_5_28, -1, 1, 0},
    /* 29 */ {4, s_5_29, -1, 1, 0},
    /* 30 */ {5, s_5_30, -1, 1, 0},
    /* 31 */ {4, s_5_31, -1, 8, 0},
    /* 32 */ {4, s_5_32, -1, 1, 0},
    /* 33 */ {7, s_5_33, -1, 2, 0},
    /* 34 */ {6, s_5_34, -1, 7, 0},
    /* 35 */ {7, s_5_35, -1, 3, 0},
    /* 36 */ {6, s_5_36, -1, 1, 0},
    /* 37 */ {5, s_5_37, -1, 1, 0},
    /* 38 */ {7, s_5_38, -1, 1, 0},
    /* 39 */ {4, s_5_39, -1, 1, 0},
    /* 40 */ {5, s_5_40, -1, 1, 0},
    /* 41 */ {4, s_5_41, -1, 1, 0},
    /* 42 */ {7, s_5_42, -1, 1, 0},
    /* 43 */ {7, s_5_43, -1, 1, 0},
    /* 44 */ {4, s_5_44, -1, 8, 0}};

static const symbol s_6_0[3] = {'a', 'd', 'a'};
static const symbol s_6_1[3] = {'i', 'd', 'a'};
static const symbol s_6_2[2] = {'i', 'a'};
static const symbol s_6_3[4] = {'a', 'r', 'i', 'a'};
static const symbol s_6_4[4] = {'e', 'r', 'i', 'a'};
static const symbol s_6_5[4] = {'i', 'r', 'i', 'a'};
static const symbol s_6_6[3] = {'a', 'r', 'a'};
static const symbol s_6_7[3] = {'e', 'r', 'a'};
static const symbol s_6_8[3] = {'i', 'r', 'a'};
static const symbol s_6_9[3] = {'a', 'v', 'a'};
static const symbol s_6_10[4] = {'a', 's', 's', 'e'};
static const symbol s_6_11[4] = {'e', 's', 's', 'e'};
static const symbol s_6_12[4] = {'i', 's', 's', 'e'};
static const symbol s_6_13[4] = {'a', 's', 't', 'e'};
static const symbol s_6_14[4] = {'e', 's', 't', 'e'};
static const symbol s_6_15[4] = {'i', 's', 't', 'e'};
static const symbol s_6_16[2] = {'e', 'i'};
static const symbol s_6_17[4] = {'a', 'r', 'e', 'i'};
static const symbol s_6_18[4] = {'e', 'r', 'e', 'i'};
static const symbol s_6_19[4] = {'i', 'r', 'e', 'i'};
static const symbol s_6_20[2] = {'a', 'm'};
static const symbol s_6_21[3] = {'i', 'a', 'm'};
static const symbol s_6_22[5] = {'a', 'r', 'i', 'a', 'm'};
static const symbol s_6_23[5] = {'e', 'r', 'i', 'a', 'm'};
static const symbol s_6_24[5] = {'i', 'r', 'i', 'a', 'm'};
static const symbol s_6_25[4] = {'a', 'r', 'a', 'm'};
static const symbol s_6_26[4] = {'e', 'r', 'a', 'm'};
static const symbol s_6_27[4] = {'i', 'r', 'a', 'm'};
static const symbol s_6_28[4] = {'a', 'v', 'a', 'm'};
static const symbol s_6_29[2] = {'e', 'm'};
static const symbol s_6_30[4] = {'a', 'r', 'e', 'm'};
static const symbol s_6_31[4] = {'e', 'r', 'e', 'm'};
static const symbol s_6_32[4] = {'i', 'r', 'e', 'm'};
static const symbol s_6_33[5] = {'a', 's', 's', 'e', 'm'};
static const symbol s_6_34[5] = {'e', 's', 's', 'e', 'm'};
static const symbol s_6_35[5] = {'i', 's', 's', 'e', 'm'};
static const symbol s_6_36[3] = {'a', 'd', 'o'};
static const symbol s_6_37[3] = {'i', 'd', 'o'};
static const symbol s_6_38[4] = {'a', 'n', 'd', 'o'};
static const symbol s_6_39[4] = {'e', 'n', 'd', 'o'};
static const symbol s_6_40[4] = {'i', 'n', 'd', 'o'};
static const symbol s_6_41[5] = {'a', 'r', 'a', '~', 'o'};
static const symbol s_6_42[5] = {'e', 'r', 'a', '~', 'o'};
static const symbol s_6_43[5] = {'i', 'r', 'a', '~', 'o'};
static const symbol s_6_44[2] = {'a', 'r'};
static const symbol s_6_45[2] = {'e', 'r'};
static const symbol s_6_46[2] = {'i', 'r'};
static const symbol s_6_47[2] = {'a', 's'};
static const symbol s_6_48[4] = {'a', 'd', 'a', 's'};
static const symbol s_6_49[4] = {'i', 'd', 'a', 's'};
static const symbol s_6_50[3] = {'i', 'a', 's'};
static const symbol s_6_51[5] = {'a', 'r', 'i', 'a', 's'};
static const symbol s_6_52[5] = {'e', 'r', 'i', 'a', 's'};
static const symbol s_6_53[5] = {'i', 'r', 'i', 'a', 's'};
static const symbol s_6_54[4] = {'a', 'r', 'a', 's'};
static const symbol s_6_55[4] = {'e', 'r', 'a', 's'};
static const symbol s_6_56[4] = {'i', 'r', 'a', 's'};
static const symbol s_6_57[4] = {'a', 'v', 'a', 's'};
static const symbol s_6_58[2] = {'e', 's'};
static const symbol s_6_59[5] = {'a', 'r', 'd', 'e', 's'};
static const symbol s_6_60[5] = {'e', 'r', 'd', 'e', 's'};
static const symbol s_6_61[5] = {'i', 'r', 'd', 'e', 's'};
static const symbol s_6_62[4] = {'a', 'r', 'e', 's'};
static const symbol s_6_63[4] = {'e', 'r', 'e', 's'};
static const symbol s_6_64[4] = {'i', 'r', 'e', 's'};
static const symbol s_6_65[5] = {'a', 's', 's', 'e', 's'};
static const symbol s_6_66[5] = {'e', 's', 's', 'e', 's'};
static const symbol s_6_67[5] = {'i', 's', 's', 'e', 's'};
static const symbol s_6_68[5] = {'a', 's', 't', 'e', 's'};
static const symbol s_6_69[5] = {'e', 's', 't', 'e', 's'};
static const symbol s_6_70[5] = {'i', 's', 't', 'e', 's'};
static const symbol s_6_71[2] = {'i', 's'};
static const symbol s_6_72[3] = {'a', 'i', 's'};
static const symbol s_6_73[3] = {'e', 'i', 's'};
static const symbol s_6_74[5] = {'a', 'r', 'e', 'i', 's'};
static const symbol s_6_75[5] = {'e', 'r', 'e', 'i', 's'};
static const symbol s_6_76[5] = {'i', 'r', 'e', 'i', 's'};
static const symbol s_6_77[6] = {0xC3, 0xA1, 'r', 'e', 'i', 's'};
static const symbol s_6_78[6] = {0xC3, 0xA9, 'r', 'e', 'i', 's'};
static const symbol s_6_79[6] = {0xC3, 0xAD, 'r', 'e', 'i', 's'};
static const symbol s_6_80[7] = {0xC3, 0xA1, 's', 's', 'e', 'i', 's'};
static const symbol s_6_81[7] = {0xC3, 0xA9, 's', 's', 'e', 'i', 's'};
static const symbol s_6_82[7] = {0xC3, 0xAD, 's', 's', 'e', 'i', 's'};
static const symbol s_6_83[6] = {0xC3, 0xA1, 'v', 'e', 'i', 's'};
static const symbol s_6_84[5] = {0xC3, 0xAD, 'e', 'i', 's'};
static const symbol s_6_85[7] = {'a', 'r', 0xC3, 0xAD, 'e', 'i', 's'};
static const symbol s_6_86[7] = {'e', 'r', 0xC3, 0xAD, 'e', 'i', 's'};
static const symbol s_6_87[7] = {'i', 'r', 0xC3, 0xAD, 'e', 'i', 's'};
static const symbol s_6_88[4] = {'a', 'd', 'o', 's'};
static const symbol s_6_89[4] = {'i', 'd', 'o', 's'};
static const symbol s_6_90[4] = {'a', 'm', 'o', 's'};
static const symbol s_6_91[7] = {0xC3, 0xA1, 'r', 'a', 'm', 'o', 's'};
static const symbol s_6_92[7] = {0xC3, 0xA9, 'r', 'a', 'm', 'o', 's'};
static const symbol s_6_93[7] = {0xC3, 0xAD, 'r', 'a', 'm', 'o', 's'};
static const symbol s_6_94[7] = {0xC3, 0xA1, 'v', 'a', 'm', 'o', 's'};
static const symbol s_6_95[6] = {0xC3, 0xAD, 'a', 'm', 'o', 's'};
static const symbol s_6_96[8] = {'a', 'r', 0xC3, 0xAD, 'a', 'm', 'o', 's'};
static const symbol s_6_97[8] = {'e', 'r', 0xC3, 0xAD, 'a', 'm', 'o', 's'};
static const symbol s_6_98[8] = {'i', 'r', 0xC3, 0xAD, 'a', 'm', 'o', 's'};
static const symbol s_6_99[4] = {'e', 'm', 'o', 's'};
static const symbol s_6_100[6] = {'a', 'r', 'e', 'm', 'o', 's'};
static const symbol s_6_101[6] = {'e', 'r', 'e', 'm', 'o', 's'};
static const symbol s_6_102[6] = {'i', 'r', 'e', 'm', 'o', 's'};
static const symbol s_6_103[8] = {0xC3, 0xA1, 's', 's', 'e', 'm', 'o', 's'};
static const symbol s_6_104[8] = {0xC3, 0xAA, 's', 's', 'e', 'm', 'o', 's'};
static const symbol s_6_105[8] = {0xC3, 0xAD, 's', 's', 'e', 'm', 'o', 's'};
static const symbol s_6_106[4] = {'i', 'm', 'o', 's'};
static const symbol s_6_107[5] = {'a', 'r', 'm', 'o', 's'};
static const symbol s_6_108[5] = {'e', 'r', 'm', 'o', 's'};
static const symbol s_6_109[5] = {'i', 'r', 'm', 'o', 's'};
static const symbol s_6_110[5] = {0xC3, 0xA1, 'm', 'o', 's'};
static const symbol s_6_111[5] = {'a', 'r', 0xC3, 0xA1, 's'};
static const symbol s_6_112[5] = {'e', 'r', 0xC3, 0xA1, 's'};
static const symbol s_6_113[5] = {'i', 'r', 0xC3, 0xA1, 's'};
static const symbol s_6_114[2] = {'e', 'u'};
static const symbol s_6_115[2] = {'i', 'u'};
static const symbol s_6_116[2] = {'o', 'u'};
static const symbol s_6_117[4] = {'a', 'r', 0xC3, 0xA1};
static const symbol s_6_118[4] = {'e', 'r', 0xC3, 0xA1};
static const symbol s_6_119[4] = {'i', 'r', 0xC3, 0xA1};

static const struct among a_6[120] = {
    /*  0 */ {3, s_6_0, -1, 1, 0},
    /*  1 */ {3, s_6_1, -1, 1, 0},
    /*  2 */ {2, s_6_2, -1, 1, 0},
    /*  3 */ {4, s_6_3, 2, 1, 0},
    /*  4 */ {4, s_6_4, 2, 1, 0},
    /*  5 */ {4, s_6_5, 2, 1, 0},
    /*  6 */ {3, s_6_6, -1, 1, 0},
    /*  7 */ {3, s_6_7, -1, 1, 0},
    /*  8 */ {3, s_6_8, -1, 1, 0},
    /*  9 */ {3, s_6_9, -1, 1, 0},
    /* 10 */ {4, s_6_10, -1, 1, 0},
    /* 11 */ {4, s_6_11, -1, 1, 0},
    /* 12 */ {4, s_6_12, -1, 1, 0},
    /* 13 */ {4, s_6_13, -1, 1, 0},
    /* 14 */ {4, s_6_14, -1, 1, 0},
    /* 15 */ {4, s_6_15, -1, 1, 0},
    /* 16 */ {2, s_6_16, -1, 1, 0},
    /* 17 */ {4, s_6_17, 16, 1, 0},
    /* 18 */ {4, s_6_18, 16, 1, 0},
    /* 19 */ {4, s_6_19, 16, 1, 0},
    /* 20 */ {2, s_6_20, -1, 1, 0},
    /* 21 */ {3, s_6_21, 20, 1, 0},
    /* 22 */ {5, s_6_22, 21, 1, 0},
    /* 23 */ {5, s_6_23, 21, 1, 0},
    /* 24 */ {5, s_6_24, 21, 1, 0},
    /* 25 */ {4, s_6_25, 20, 1, 0},
    /* 26 */ {4, s_6_26, 20, 1, 0},
    /* 27 */ {4, s_6_27, 20, 1, 0},
    /* 28 */ {4, s_6_28, 20, 1, 0},
    /* 29 */ {2, s_6_29, -1, 1, 0},
    /* 30 */ {4, s_6_30, 29, 1, 0},
    /* 31 */ {4, s_6_31, 29, 1, 0},
    /* 32 */ {4, s_6_32, 29, 1, 0},
    /* 33 */ {5, s_6_33, 29, 1, 0},
    /* 34 */ {5, s_6_34, 29, 1, 0},
    /* 35 */ {5, s_6_35, 29, 1, 0},
    /* 36 */ {3, s_6_36, -1, 1, 0},
    /* 37 */ {3, s_6_37, -1, 1, 0},
    /* 38 */ {4, s_6_38, -1, 1, 0},
    /* 39 */ {4, s_6_39, -1, 1, 0},
    /* 40 */ {4, s_6_40, -1, 1, 0},
    /* 41 */ {5, s_6_41, -1, 1, 0},
    /* 42 */ {5, s_6_42, -1, 1, 0},
    /* 43 */ {5, s_6_43, -1, 1, 0},
    /* 44 */ {2, s_6_44, -1, 1, 0},
    /* 45 */ {2, s_6_45, -1, 1, 0},
    /* 46 */ {2, s_6_46, -1, 1, 0},
    /* 47 */ {2, s_6_47, -1, 1, 0},
    /* 48 */ {4, s_6_48, 47, 1, 0},
    /* 49 */ {4, s_6_49, 47, 1, 0},
    /* 50 */ {3, s_6_50, 47, 1, 0},
    /* 51 */ {5, s_6_51, 50, 1, 0},
    /* 52 */ {5, s_6_52, 50, 1, 0},
    /* 53 */ {5, s_6_53, 50, 1, 0},
    /* 54 */ {4, s_6_54, 47, 1, 0},
    /* 55 */ {4, s_6_55, 47, 1, 0},
    /* 56 */ {4, s_6_56, 47, 1, 0},
    /* 57 */ {4, s_6_57, 47, 1, 0},
    /* 58 */ {2, s_6_58, -1, 1, 0},
    /* 59 */ {5, s_6_59, 58, 1, 0},
    /* 60 */ {5, s_6_60, 58, 1, 0},
    /* 61 */ {5, s_6_61, 58, 1, 0},
    /* 62 */ {4, s_6_62, 58, 1, 0},
    /* 63 */ {4, s_6_63, 58, 1, 0},
    /* 64 */ {4, s_6_64, 58, 1, 0},
    /* 65 */ {5, s_6_65, 58, 1, 0},
    /* 66 */ {5, s_6_66, 58, 1, 0},
    /* 67 */ {5, s_6_67, 58, 1, 0},
    /* 68 */ {5, s_6_68, 58, 1, 0},
    /* 69 */ {5, s_6_69, 58, 1, 0},
    /* 70 */ {5, s_6_70, 58, 1, 0},
    /* 71 */ {2, s_6_71, -1, 1, 0},
    /* 72 */ {3, s_6_72, 71, 1, 0},
    /* 73 */ {3, s_6_73, 71, 1, 0},
    /* 74 */ {5, s_6_74, 73, 1, 0},
    /* 75 */ {5, s_6_75, 73, 1, 0},
    /* 76 */ {5, s_6_76, 73, 1, 0},
    /* 77 */ {6, s_6_77, 73, 1, 0},
    /* 78 */ {6, s_6_78, 73, 1, 0},
    /* 79 */ {6, s_6_79, 73, 1, 0},
    /* 80 */ {7, s_6_80, 73, 1, 0},
    /* 81 */ {7, s_6_81, 73, 1, 0},
    /* 82 */ {7, s_6_82, 73, 1, 0},
    /* 83 */ {6, s_6_83, 73, 1, 0},
    /* 84 */ {5, s_6_84, 73, 1, 0},
    /* 85 */ {7, s_6_85, 84, 1, 0},
    /* 86 */ {7, s_6_86, 84, 1, 0},
    /* 87 */ {7, s_6_87, 84, 1, 0},
    /* 88 */ {4, s_6_88, -1, 1, 0},
    /* 89 */ {4, s_6_89, -1, 1, 0},
    /* 90 */ {4, s_6_90, -1, 1, 0},
    /* 91 */ {7, s_6_91, 90, 1, 0},
    /* 92 */ {7, s_6_92, 90, 1, 0},
    /* 93 */ {7, s_6_93, 90, 1, 0},
    /* 94 */ {7, s_6_94, 90, 1, 0},
    /* 95 */ {6, s_6_95, 90, 1, 0},
    /* 96 */ {8, s_6_96, 95, 1, 0},
    /* 97 */ {8, s_6_97, 95, 1, 0},
    /* 98 */ {8, s_6_98, 95, 1, 0},
    /* 99 */ {4, s_6_99, -1, 1, 0},
    /*100 */ {6, s_6_100, 99, 1, 0},
    /*101 */ {6, s_6_101, 99, 1, 0},
    /*102 */ {6, s_6_102, 99, 1, 0},
    /*103 */ {8, s_6_103, 99, 1, 0},
    /*104 */ {8, s_6_104, 99, 1, 0},
    /*105 */ {8, s_6_105, 99, 1, 0},
    /*106 */ {4, s_6_106, -1, 1, 0},
    /*107 */ {5, s_6_107, -1, 1, 0},
    /*108 */ {5, s_6_108, -1, 1, 0},
    /*109 */ {5, s_6_109, -1, 1, 0},
    /*110 */ {5, s_6_110, -1, 1, 0},
    /*111 */ {5, s_6_111, -1, 1, 0},
    /*112 */ {5, s_6_112, -1, 1, 0},
    /*113 */ {5, s_6_113, -1, 1, 0},
    /*114 */ {2, s_6_114, -1, 1, 0},
    /*115 */ {2, s_6_115, -1, 1, 0},
    /*116 */ {2, s_6_116, -1, 1, 0},
    /*117 */ {4, s_6_117, -1, 1, 0},
    /*118 */ {4, s_6_118, -1, 1, 0},
    /*119 */ {4, s_6_119, -1, 1, 0}};

static const symbol s_7_0[1] = {'a'};
static const symbol s_7_1[1] = {'i'};
static const symbol s_7_2[1] = {'o'};
static const symbol s_7_3[2] = {'o', 's'};
static const symbol s_7_4[2] = {0xC3, 0xA1};
static const symbol s_7_5[2] = {0xC3, 0xAD};
static const symbol s_7_6[2] = {0xC3, 0xB3};

static const struct among a_7[7] = {
    /*  0 */ {1, s_7_0, -1, 1, 0},
    /*  1 */ {1, s_7_1, -1, 1, 0},
    /*  2 */ {1, s_7_2, -1, 1, 0},
    /*  3 */ {2, s_7_3, -1, 1, 0},
    /*  4 */ {2, s_7_4, -1, 1, 0},
    /*  5 */ {2, s_7_5, -1, 1, 0},
    /*  6 */ {2, s_7_6, -1, 1, 0}};

static const symbol s_8_0[1] = {'e'};
static const symbol s_8_1[2] = {0xC3, 0xA7};
static const symbol s_8_2[2] = {0xC3, 0xA9};
static const symbol s_8_3[2] = {0xC3, 0xAA};

static const struct among a_8[4] = {
    /*  0 */ {1, s_8_0, -1, 1, 0},
    /*  1 */ {2, s_8_1, -1, 2, 0},
    /*  2 */ {2, s_8_2, -1, 1, 0},
    /*  3 */ {2, s_8_3, -1, 1, 0}};

static const unsigned char g_v[] = {17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 19, 12, 2};

static const symbol s_0[] = {'a', '~'};
static const symbol s_1[] = {'o', '~'};
static const symbol s_2[] = {0xC3, 0xA3};
static const symbol s_3[] = {0xC3, 0xB5};
static const symbol s_4[] = {'l', 'o', 'g'};
static const symbol s_5[] = {'u'};
static const symbol s_6[] = {'e', 'n', 't', 'e'};
static const symbol s_7[] = {'a', 't'};
static const symbol s_8[] = {'a', 't'};
static const symbol s_9[] = {'e'};
static const symbol s_10[] = {'i', 'r'};
static const symbol s_11[] = {'u'};
static const symbol s_12[] = {'g'};
static const symbol s_13[] = {'i'};
static const symbol s_14[] = {'c'};
static const symbol s_15[] = {'c'};
static const symbol s_16[] = {'i'};
static const symbol s_17[] = {'c'};

static int r_prelude(struct SN_env *z) {
    int among_var;
    while (1) { /* repeat, line 36 */
        int c1 = z->c;
        z->bra = z->c; /* [, line 37 */
        if (z->c + 1 >= z->l || (z->p[z->c + 1] != 163 && z->p[z->c + 1] != 181))
            among_var = 3;
        else
            among_var = find_among(z, a_0, 3); /* substring, line 37 */
        if (!(among_var))
            goto lab0;
        z->ket = z->c; /* ], line 37 */
        switch (among_var) {
            case 0:
                goto lab0;
            case 1: {
                int ret = slice_from_s(z, 2, s_0); /* <-, line 38 */
                if (ret < 0)
                    return ret;
            } break;
            case 2: {
                int ret = slice_from_s(z, 2, s_1); /* <-, line 39 */
                if (ret < 0)
                    return ret;
            } break;
            case 3: {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    goto lab0;
                z->c = ret; /* next, line 40 */
            } break;
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
        int c1 = z->c; /* do, line 50 */
        {
            int c2 = z->c; /* or, line 52 */
            if (in_grouping_U(z, g_v, 97, 250, 0))
                goto lab2;
            {
                int c3 = z->c; /* or, line 51 */
                if (out_grouping_U(z, g_v, 97, 250, 0))
                    goto lab4;
                { /* gopast */ /* grouping v, line 51 */
                    int ret = out_grouping_U(z, g_v, 97, 250, 1);
                    if (ret < 0)
                        goto lab4;
                    z->c += ret;
                }
                goto lab3;
            lab4:
                z->c = c3;
                if (in_grouping_U(z, g_v, 97, 250, 0))
                    goto lab2;
                { /* gopast */ /* non v, line 51 */
                    int ret = in_grouping_U(z, g_v, 97, 250, 1);
                    if (ret < 0)
                        goto lab2;
                    z->c += ret;
                }
            }
        lab3:
            goto lab1;
        lab2:
            z->c = c2;
            if (out_grouping_U(z, g_v, 97, 250, 0))
                goto lab0;
            {
                int c4 = z->c; /* or, line 53 */
                if (out_grouping_U(z, g_v, 97, 250, 0))
                    goto lab6;
                { /* gopast */ /* grouping v, line 53 */
                    int ret = out_grouping_U(z, g_v, 97, 250, 1);
                    if (ret < 0)
                        goto lab6;
                    z->c += ret;
                }
                goto lab5;
            lab6:
                z->c = c4;
                if (in_grouping_U(z, g_v, 97, 250, 0))
                    goto lab0;
                {
                    int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                    if (ret < 0)
                        goto lab0;
                    z->c = ret; /* next, line 53 */
                }
            }
        lab5:;
        }
    lab1:
        z->I[0] = z->c; /* setmark pV, line 54 */
    lab0:
        z->c = c1;
    }
    {
        int c5 = z->c; /* do, line 56 */
        { /* gopast */ /* grouping v, line 57 */
            int ret = out_grouping_U(z, g_v, 97, 250, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        { /* gopast */ /* non v, line 57 */
            int ret = in_grouping_U(z, g_v, 97, 250, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        z->I[1] = z->c; /* setmark p1, line 57 */
        { /* gopast */  /* grouping v, line 58 */
            int ret = out_grouping_U(z, g_v, 97, 250, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        { /* gopast */ /* non v, line 58 */
            int ret = in_grouping_U(z, g_v, 97, 250, 1);
            if (ret < 0)
                goto lab7;
            z->c += ret;
        }
        z->I[2] = z->c; /* setmark p2, line 58 */
    lab7:
        z->c = c5;
    }
    return 1;
}

static int r_postlude(struct SN_env *z) {
    int among_var;
    while (1) { /* repeat, line 62 */
        int c1 = z->c;
        z->bra = z->c; /* [, line 63 */
        if (z->c + 1 >= z->l || z->p[z->c + 1] != 126)
            among_var = 3;
        else
            among_var = find_among(z, a_1, 3); /* substring, line 63 */
        if (!(among_var))
            goto lab0;
        z->ket = z->c; /* ], line 63 */
        switch (among_var) {
            case 0:
                goto lab0;
            case 1: {
                int ret = slice_from_s(z, 2, s_2); /* <-, line 64 */
                if (ret < 0)
                    return ret;
            } break;
            case 2: {
                int ret = slice_from_s(z, 2, s_3); /* <-, line 65 */
                if (ret < 0)
                    return ret;
            } break;
            case 3: {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    goto lab0;
                z->c = ret; /* next, line 66 */
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

static int r_standard_suffix(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 77 */
    if (z->c - 2 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((839714 >> (z->p[z->c - 1] & 0x1f)) & 1))
        return 0;
    among_var = find_among_b(z, a_5, 45); /* substring, line 77 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 77 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 93 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 93 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 2: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 98 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_from_s(z, 3, s_4); /* <-, line 98 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 3: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 102 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_from_s(z, 1, s_5); /* <-, line 102 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 4: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 106 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_from_s(z, 4, s_6); /* <-, line 106 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 5: {
            int ret = r_R1(z);
            if (ret == 0)
                return 0; /* call R1, line 110 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 110 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 111 */
                z->ket = z->c;                                /* [, line 112 */
                if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((4718616 >> (z->p[z->c - 1] & 0x1f)) & 1)) {
                    z->c = z->l - m_keep;
                    goto lab0;
                }
                among_var = find_among_b(z, a_2, 4); /* substring, line 112 */
                if (!(among_var)) {
                    z->c = z->l - m_keep;
                    goto lab0;
                }
                z->bra = z->c; /* ], line 112 */
                {
                    int ret = r_R2(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab0;
                    } /* call R2, line 112 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int ret = slice_del(z); /* delete, line 112 */
                    if (ret < 0)
                        return ret;
                }
                switch (among_var) {
                    case 0: {
                        z->c = z->l - m_keep;
                        goto lab0;
                    }
                    case 1:
                        z->ket = z->c; /* [, line 113 */
                        if (!(eq_s_b(z, 2, s_7))) {
                            z->c = z->l - m_keep;
                            goto lab0;
                        }
                        z->bra = z->c; /* ], line 113 */
                        {
                            int ret = r_R2(z);
                            if (ret == 0) {
                                z->c = z->l - m_keep;
                                goto lab0;
                            } /* call R2, line 113 */
                            if (ret < 0)
                                return ret;
                        }
                        {
                            int ret = slice_del(z); /* delete, line 113 */
                            if (ret < 0)
                                return ret;
                        }
                        break;
                }
            lab0:;
            }
            break;
        case 6: {
            int ret = r_R2(z);
            if (ret == 0)
                return 0; /* call R2, line 122 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 122 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 123 */
                z->ket = z->c;                                /* [, line 124 */
                if (z->c - 3 <= z->lb || (z->p[z->c - 1] != 101 && z->p[z->c - 1] != 108)) {
                    z->c = z->l - m_keep;
                    goto lab1;
                }
                among_var = find_among_b(z, a_3, 3); /* substring, line 124 */
                if (!(among_var)) {
                    z->c = z->l - m_keep;
                    goto lab1;
                }
                z->bra = z->c; /* ], line 124 */
                switch (among_var) {
                    case 0: {
                        z->c = z->l - m_keep;
                        goto lab1;
                    }
                    case 1: {
                        int ret = r_R2(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab1;
                        } /* call R2, line 127 */
                        if (ret < 0)
                            return ret;
                    }
                        {
                            int ret = slice_del(z); /* delete, line 127 */
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
                return 0; /* call R2, line 134 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 134 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 135 */
                z->ket = z->c;                                /* [, line 136 */
                if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((4198408 >> (z->p[z->c - 1] & 0x1f)) & 1)) {
                    z->c = z->l - m_keep;
                    goto lab2;
                }
                among_var = find_among_b(z, a_4, 3); /* substring, line 136 */
                if (!(among_var)) {
                    z->c = z->l - m_keep;
                    goto lab2;
                }
                z->bra = z->c; /* ], line 136 */
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
                        } /* call R2, line 139 */
                        if (ret < 0)
                            return ret;
                    }
                        {
                            int ret = slice_del(z); /* delete, line 139 */
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
                return 0; /* call R2, line 146 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 146 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 147 */
                z->ket = z->c;                                /* [, line 148 */
                if (!(eq_s_b(z, 2, s_8))) {
                    z->c = z->l - m_keep;
                    goto lab3;
                }
                z->bra = z->c; /* ], line 148 */
                {
                    int ret = r_R2(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab3;
                    } /* call R2, line 148 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int ret = slice_del(z); /* delete, line 148 */
                    if (ret < 0)
                        return ret;
                }
            lab3:;
            }
            break;
        case 9: {
            int ret = r_RV(z);
            if (ret == 0)
                return 0; /* call RV, line 153 */
            if (ret < 0)
                return ret;
        }
            if (!(eq_s_b(z, 1, s_9)))
                return 0;
            {
                int ret = slice_from_s(z, 2, s_10); /* <-, line 154 */
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
        int mlimit; /* setlimit, line 159 */
        int m1 = z->l - z->c;
        (void)m1;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 159 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m1;
        z->ket = z->c;                         /* [, line 160 */
        among_var = find_among_b(z, a_6, 120); /* substring, line 160 */
        if (!(among_var)) {
            z->lb = mlimit;
            return 0;
        }
        z->bra = z->c; /* ], line 160 */
        switch (among_var) {
            case 0: {
                z->lb = mlimit;
                return 0;
            }
            case 1: {
                int ret = slice_del(z); /* delete, line 179 */
                if (ret < 0)
                    return ret;
            } break;
        }
        z->lb = mlimit;
    }
    return 1;
}

static int r_residual_suffix(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                       /* [, line 184 */
    among_var = find_among_b(z, a_7, 7); /* substring, line 184 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 184 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = r_RV(z);
            if (ret == 0)
                return 0; /* call RV, line 187 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 187 */
                if (ret < 0)
                    return ret;
            }
            break;
    }
    return 1;
}

static int r_residual_form(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                       /* [, line 192 */
    among_var = find_among_b(z, a_8, 4); /* substring, line 192 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 192 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = r_RV(z);
            if (ret == 0)
                return 0; /* call RV, line 194 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 194 */
                if (ret < 0)
                    return ret;
            }
            z->ket = z->c; /* [, line 194 */
            {
                int m1 = z->l - z->c;
                (void)m1; /* or, line 194 */
                if (!(eq_s_b(z, 1, s_11)))
                    goto lab1;
                z->bra = z->c; /* ], line 194 */
                {
                    int m_test = z->l - z->c; /* test, line 194 */
                    if (!(eq_s_b(z, 1, s_12)))
                        goto lab1;
                    z->c = z->l - m_test;
                }
                goto lab0;
            lab1:
                z->c = z->l - m1;
                if (!(eq_s_b(z, 1, s_13)))
                    return 0;
                z->bra = z->c; /* ], line 195 */
                {
                    int m_test = z->l - z->c; /* test, line 195 */
                    if (!(eq_s_b(z, 1, s_14)))
                        return 0;
                    z->c = z->l - m_test;
                }
            }
        lab0: {
            int ret = r_RV(z);
            if (ret == 0)
                return 0; /* call RV, line 195 */
            if (ret < 0)
                return ret;
        }
            {
                int ret = slice_del(z); /* delete, line 195 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 2: {
            int ret = slice_from_s(z, 1, s_15); /* <-, line 196 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

extern int portuguese_UTF_8_stem(struct SN_env *z) {
    {
        int c1 = z->c; /* do, line 202 */
        {
            int ret = r_prelude(z);
            if (ret == 0)
                goto lab0; /* call prelude, line 202 */
            if (ret < 0)
                return ret;
        }
    lab0:
        z->c = c1;
    }
    {
        int c2 = z->c; /* do, line 203 */
        {
            int ret = r_mark_regions(z);
            if (ret == 0)
                goto lab1; /* call mark_regions, line 203 */
            if (ret < 0)
                return ret;
        }
    lab1:
        z->c = c2;
    }
    z->lb = z->c;
    z->c = z->l; /* backwards, line 204 */

    {
        int m3 = z->l - z->c;
        (void)m3; /* do, line 205 */
        {
            int m4 = z->l - z->c;
            (void)m4; /* or, line 209 */
            {
                int m5 = z->l - z->c;
                (void)m5; /* and, line 207 */
                {
                    int m6 = z->l - z->c;
                    (void)m6; /* or, line 206 */
                    {
                        int ret = r_standard_suffix(z);
                        if (ret == 0)
                            goto lab6; /* call standard_suffix, line 206 */
                        if (ret < 0)
                            return ret;
                    }
                    goto lab5;
                lab6:
                    z->c = z->l - m6;
                    {
                        int ret = r_verb_suffix(z);
                        if (ret == 0)
                            goto lab4; /* call verb_suffix, line 206 */
                        if (ret < 0)
                            return ret;
                    }
                }
            lab5:
                z->c = z->l - m5;
                {
                    int m7 = z->l - z->c;
                    (void)m7;      /* do, line 207 */
                    z->ket = z->c; /* [, line 207 */
                    if (!(eq_s_b(z, 1, s_16)))
                        goto lab7;
                    z->bra = z->c; /* ], line 207 */
                    {
                        int m_test = z->l - z->c; /* test, line 207 */
                        if (!(eq_s_b(z, 1, s_17)))
                            goto lab7;
                        z->c = z->l - m_test;
                    }
                    {
                        int ret = r_RV(z);
                        if (ret == 0)
                            goto lab7; /* call RV, line 207 */
                        if (ret < 0)
                            return ret;
                    }
                    {
                        int ret = slice_del(z); /* delete, line 207 */
                        if (ret < 0)
                            return ret;
                    }
                lab7:
                    z->c = z->l - m7;
                }
            }
            goto lab3;
        lab4:
            z->c = z->l - m4;
            {
                int ret = r_residual_suffix(z);
                if (ret == 0)
                    goto lab2; /* call residual_suffix, line 209 */
                if (ret < 0)
                    return ret;
            }
        }
    lab3:
    lab2:
        z->c = z->l - m3;
    }
    {
        int m8 = z->l - z->c;
        (void)m8; /* do, line 211 */
        {
            int ret = r_residual_form(z);
            if (ret == 0)
                goto lab8; /* call residual_form, line 211 */
            if (ret < 0)
                return ret;
        }
    lab8:
        z->c = z->l - m8;
    }
    z->c = z->lb;
    {
        int c9 = z->c; /* do, line 213 */
        {
            int ret = r_postlude(z);
            if (ret == 0)
                goto lab9; /* call postlude, line 213 */
            if (ret < 0)
                return ret;
        }
    lab9:
        z->c = c9;
    }
    return 1;
}

extern struct SN_env *portuguese_UTF_8_create_env(void) { return SN_create_env(0, 3, 0); }

extern void portuguese_UTF_8_close_env(struct SN_env *z) { SN_close_env(z, 0); }
