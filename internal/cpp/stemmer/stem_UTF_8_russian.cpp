
/* This file was generated automatically by the Snowball to ANSI C compiler */

#include "header.h"

#ifdef __cplusplus
extern "C" {
#endif
extern int russian_UTF_8_stem(struct SN_env *z);
#ifdef __cplusplus
}
#endif
static int r_tidy_up(struct SN_env *z);
static int r_derivational(struct SN_env *z);
static int r_noun(struct SN_env *z);
static int r_verb(struct SN_env *z);
static int r_reflexive(struct SN_env *z);
static int r_adjectival(struct SN_env *z);
static int r_adjective(struct SN_env *z);
static int r_perfective_gerund(struct SN_env *z);
static int r_R2(struct SN_env *z);
static int r_mark_regions(struct SN_env *z);
#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *russian_UTF_8_create_env(void);
extern void russian_UTF_8_close_env(struct SN_env *z);

#ifdef __cplusplus
}
#endif
static const symbol s_0_0[10] = {0xD0, 0xB2, 0xD1, 0x88, 0xD0, 0xB8, 0xD1, 0x81, 0xD1, 0x8C};
static const symbol s_0_1[12] = {0xD1, 0x8B, 0xD0, 0xB2, 0xD1, 0x88, 0xD0, 0xB8, 0xD1, 0x81, 0xD1, 0x8C};
static const symbol s_0_2[12] = {0xD0, 0xB8, 0xD0, 0xB2, 0xD1, 0x88, 0xD0, 0xB8, 0xD1, 0x81, 0xD1, 0x8C};
static const symbol s_0_3[2] = {0xD0, 0xB2};
static const symbol s_0_4[4] = {0xD1, 0x8B, 0xD0, 0xB2};
static const symbol s_0_5[4] = {0xD0, 0xB8, 0xD0, 0xB2};
static const symbol s_0_6[6] = {0xD0, 0xB2, 0xD1, 0x88, 0xD0, 0xB8};
static const symbol s_0_7[8] = {0xD1, 0x8B, 0xD0, 0xB2, 0xD1, 0x88, 0xD0, 0xB8};
static const symbol s_0_8[8] = {0xD0, 0xB8, 0xD0, 0xB2, 0xD1, 0x88, 0xD0, 0xB8};

static const struct among a_0[9] = {
    /*  0 */ {10, s_0_0, -1, 1, 0},
    /*  1 */ {12, s_0_1, 0, 2, 0},
    /*  2 */ {12, s_0_2, 0, 2, 0},
    /*  3 */ {2, s_0_3, -1, 1, 0},
    /*  4 */ {4, s_0_4, 3, 2, 0},
    /*  5 */ {4, s_0_5, 3, 2, 0},
    /*  6 */ {6, s_0_6, -1, 1, 0},
    /*  7 */ {8, s_0_7, 6, 2, 0},
    /*  8 */ {8, s_0_8, 6, 2, 0}};

static const symbol s_1_0[6] = {0xD0, 0xB5, 0xD0, 0xBC, 0xD1, 0x83};
static const symbol s_1_1[6] = {0xD0, 0xBE, 0xD0, 0xBC, 0xD1, 0x83};
static const symbol s_1_2[4] = {0xD1, 0x8B, 0xD1, 0x85};
static const symbol s_1_3[4] = {0xD0, 0xB8, 0xD1, 0x85};
static const symbol s_1_4[4] = {0xD1, 0x83, 0xD1, 0x8E};
static const symbol s_1_5[4] = {0xD1, 0x8E, 0xD1, 0x8E};
static const symbol s_1_6[4] = {0xD0, 0xB5, 0xD1, 0x8E};
static const symbol s_1_7[4] = {0xD0, 0xBE, 0xD1, 0x8E};
static const symbol s_1_8[4] = {0xD1, 0x8F, 0xD1, 0x8F};
static const symbol s_1_9[4] = {0xD0, 0xB0, 0xD1, 0x8F};
static const symbol s_1_10[4] = {0xD1, 0x8B, 0xD0, 0xB5};
static const symbol s_1_11[4] = {0xD0, 0xB5, 0xD0, 0xB5};
static const symbol s_1_12[4] = {0xD0, 0xB8, 0xD0, 0xB5};
static const symbol s_1_13[4] = {0xD0, 0xBE, 0xD0, 0xB5};
static const symbol s_1_14[6] = {0xD1, 0x8B, 0xD0, 0xBC, 0xD0, 0xB8};
static const symbol s_1_15[6] = {0xD0, 0xB8, 0xD0, 0xBC, 0xD0, 0xB8};
static const symbol s_1_16[4] = {0xD1, 0x8B, 0xD0, 0xB9};
static const symbol s_1_17[4] = {0xD0, 0xB5, 0xD0, 0xB9};
static const symbol s_1_18[4] = {0xD0, 0xB8, 0xD0, 0xB9};
static const symbol s_1_19[4] = {0xD0, 0xBE, 0xD0, 0xB9};
static const symbol s_1_20[4] = {0xD1, 0x8B, 0xD0, 0xBC};
static const symbol s_1_21[4] = {0xD0, 0xB5, 0xD0, 0xBC};
static const symbol s_1_22[4] = {0xD0, 0xB8, 0xD0, 0xBC};
static const symbol s_1_23[4] = {0xD0, 0xBE, 0xD0, 0xBC};
static const symbol s_1_24[6] = {0xD0, 0xB5, 0xD0, 0xB3, 0xD0, 0xBE};
static const symbol s_1_25[6] = {0xD0, 0xBE, 0xD0, 0xB3, 0xD0, 0xBE};

static const struct among a_1[26] = {
    /*  0 */ {6, s_1_0, -1, 1, 0},
    /*  1 */ {6, s_1_1, -1, 1, 0},
    /*  2 */ {4, s_1_2, -1, 1, 0},
    /*  3 */ {4, s_1_3, -1, 1, 0},
    /*  4 */ {4, s_1_4, -1, 1, 0},
    /*  5 */ {4, s_1_5, -1, 1, 0},
    /*  6 */ {4, s_1_6, -1, 1, 0},
    /*  7 */ {4, s_1_7, -1, 1, 0},
    /*  8 */ {4, s_1_8, -1, 1, 0},
    /*  9 */ {4, s_1_9, -1, 1, 0},
    /* 10 */ {4, s_1_10, -1, 1, 0},
    /* 11 */ {4, s_1_11, -1, 1, 0},
    /* 12 */ {4, s_1_12, -1, 1, 0},
    /* 13 */ {4, s_1_13, -1, 1, 0},
    /* 14 */ {6, s_1_14, -1, 1, 0},
    /* 15 */ {6, s_1_15, -1, 1, 0},
    /* 16 */ {4, s_1_16, -1, 1, 0},
    /* 17 */ {4, s_1_17, -1, 1, 0},
    /* 18 */ {4, s_1_18, -1, 1, 0},
    /* 19 */ {4, s_1_19, -1, 1, 0},
    /* 20 */ {4, s_1_20, -1, 1, 0},
    /* 21 */ {4, s_1_21, -1, 1, 0},
    /* 22 */ {4, s_1_22, -1, 1, 0},
    /* 23 */ {4, s_1_23, -1, 1, 0},
    /* 24 */ {6, s_1_24, -1, 1, 0},
    /* 25 */ {6, s_1_25, -1, 1, 0}};

static const symbol s_2_0[4] = {0xD0, 0xB2, 0xD1, 0x88};
static const symbol s_2_1[6] = {0xD1, 0x8B, 0xD0, 0xB2, 0xD1, 0x88};
static const symbol s_2_2[6] = {0xD0, 0xB8, 0xD0, 0xB2, 0xD1, 0x88};
static const symbol s_2_3[2] = {0xD1, 0x89};
static const symbol s_2_4[4] = {0xD1, 0x8E, 0xD1, 0x89};
static const symbol s_2_5[6] = {0xD1, 0x83, 0xD1, 0x8E, 0xD1, 0x89};
static const symbol s_2_6[4] = {0xD0, 0xB5, 0xD0, 0xBC};
static const symbol s_2_7[4] = {0xD0, 0xBD, 0xD0, 0xBD};

static const struct among a_2[8] = {
    /*  0 */ {4, s_2_0, -1, 1, 0},
    /*  1 */ {6, s_2_1, 0, 2, 0},
    /*  2 */ {6, s_2_2, 0, 2, 0},
    /*  3 */ {2, s_2_3, -1, 1, 0},
    /*  4 */ {4, s_2_4, 3, 1, 0},
    /*  5 */ {6, s_2_5, 4, 2, 0},
    /*  6 */ {4, s_2_6, -1, 1, 0},
    /*  7 */ {4, s_2_7, -1, 1, 0}};

static const symbol s_3_0[4] = {0xD1, 0x81, 0xD1, 0x8C};
static const symbol s_3_1[4] = {0xD1, 0x81, 0xD1, 0x8F};

static const struct among a_3[2] = {
    /*  0 */ {4, s_3_0, -1, 1, 0},
    /*  1 */ {4, s_3_1, -1, 1, 0}};

static const symbol s_4_0[4] = {0xD1, 0x8B, 0xD1, 0x82};
static const symbol s_4_1[4] = {0xD1, 0x8E, 0xD1, 0x82};
static const symbol s_4_2[6] = {0xD1, 0x83, 0xD1, 0x8E, 0xD1, 0x82};
static const symbol s_4_3[4] = {0xD1, 0x8F, 0xD1, 0x82};
static const symbol s_4_4[4] = {0xD0, 0xB5, 0xD1, 0x82};
static const symbol s_4_5[6] = {0xD1, 0x83, 0xD0, 0xB5, 0xD1, 0x82};
static const symbol s_4_6[4] = {0xD0, 0xB8, 0xD1, 0x82};
static const symbol s_4_7[4] = {0xD0, 0xBD, 0xD1, 0x8B};
static const symbol s_4_8[6] = {0xD0, 0xB5, 0xD0, 0xBD, 0xD1, 0x8B};
static const symbol s_4_9[4] = {0xD1, 0x82, 0xD1, 0x8C};
static const symbol s_4_10[6] = {0xD1, 0x8B, 0xD1, 0x82, 0xD1, 0x8C};
static const symbol s_4_11[6] = {0xD0, 0xB8, 0xD1, 0x82, 0xD1, 0x8C};
static const symbol s_4_12[6] = {0xD0, 0xB5, 0xD1, 0x88, 0xD1, 0x8C};
static const symbol s_4_13[6] = {0xD0, 0xB8, 0xD1, 0x88, 0xD1, 0x8C};
static const symbol s_4_14[2] = {0xD1, 0x8E};
static const symbol s_4_15[4] = {0xD1, 0x83, 0xD1, 0x8E};
static const symbol s_4_16[4] = {0xD0, 0xBB, 0xD0, 0xB0};
static const symbol s_4_17[6] = {0xD1, 0x8B, 0xD0, 0xBB, 0xD0, 0xB0};
static const symbol s_4_18[6] = {0xD0, 0xB8, 0xD0, 0xBB, 0xD0, 0xB0};
static const symbol s_4_19[4] = {0xD0, 0xBD, 0xD0, 0xB0};
static const symbol s_4_20[6] = {0xD0, 0xB5, 0xD0, 0xBD, 0xD0, 0xB0};
static const symbol s_4_21[6] = {0xD0, 0xB5, 0xD1, 0x82, 0xD0, 0xB5};
static const symbol s_4_22[6] = {0xD0, 0xB8, 0xD1, 0x82, 0xD0, 0xB5};
static const symbol s_4_23[6] = {0xD0, 0xB9, 0xD1, 0x82, 0xD0, 0xB5};
static const symbol s_4_24[8] = {0xD1, 0x83, 0xD0, 0xB9, 0xD1, 0x82, 0xD0, 0xB5};
static const symbol s_4_25[8] = {0xD0, 0xB5, 0xD0, 0xB9, 0xD1, 0x82, 0xD0, 0xB5};
static const symbol s_4_26[4] = {0xD0, 0xBB, 0xD0, 0xB8};
static const symbol s_4_27[6] = {0xD1, 0x8B, 0xD0, 0xBB, 0xD0, 0xB8};
static const symbol s_4_28[6] = {0xD0, 0xB8, 0xD0, 0xBB, 0xD0, 0xB8};
static const symbol s_4_29[2] = {0xD0, 0xB9};
static const symbol s_4_30[4] = {0xD1, 0x83, 0xD0, 0xB9};
static const symbol s_4_31[4] = {0xD0, 0xB5, 0xD0, 0xB9};
static const symbol s_4_32[2] = {0xD0, 0xBB};
static const symbol s_4_33[4] = {0xD1, 0x8B, 0xD0, 0xBB};
static const symbol s_4_34[4] = {0xD0, 0xB8, 0xD0, 0xBB};
static const symbol s_4_35[4] = {0xD1, 0x8B, 0xD0, 0xBC};
static const symbol s_4_36[4] = {0xD0, 0xB5, 0xD0, 0xBC};
static const symbol s_4_37[4] = {0xD0, 0xB8, 0xD0, 0xBC};
static const symbol s_4_38[2] = {0xD0, 0xBD};
static const symbol s_4_39[4] = {0xD0, 0xB5, 0xD0, 0xBD};
static const symbol s_4_40[4] = {0xD0, 0xBB, 0xD0, 0xBE};
static const symbol s_4_41[6] = {0xD1, 0x8B, 0xD0, 0xBB, 0xD0, 0xBE};
static const symbol s_4_42[6] = {0xD0, 0xB8, 0xD0, 0xBB, 0xD0, 0xBE};
static const symbol s_4_43[4] = {0xD0, 0xBD, 0xD0, 0xBE};
static const symbol s_4_44[6] = {0xD0, 0xB5, 0xD0, 0xBD, 0xD0, 0xBE};
static const symbol s_4_45[6] = {0xD0, 0xBD, 0xD0, 0xBD, 0xD0, 0xBE};

static const struct among a_4[46] = {
    /*  0 */ {4, s_4_0, -1, 2, 0},
    /*  1 */ {4, s_4_1, -1, 1, 0},
    /*  2 */ {6, s_4_2, 1, 2, 0},
    /*  3 */ {4, s_4_3, -1, 2, 0},
    /*  4 */ {4, s_4_4, -1, 1, 0},
    /*  5 */ {6, s_4_5, 4, 2, 0},
    /*  6 */ {4, s_4_6, -1, 2, 0},
    /*  7 */ {4, s_4_7, -1, 1, 0},
    /*  8 */ {6, s_4_8, 7, 2, 0},
    /*  9 */ {4, s_4_9, -1, 1, 0},
    /* 10 */ {6, s_4_10, 9, 2, 0},
    /* 11 */ {6, s_4_11, 9, 2, 0},
    /* 12 */ {6, s_4_12, -1, 1, 0},
    /* 13 */ {6, s_4_13, -1, 2, 0},
    /* 14 */ {2, s_4_14, -1, 2, 0},
    /* 15 */ {4, s_4_15, 14, 2, 0},
    /* 16 */ {4, s_4_16, -1, 1, 0},
    /* 17 */ {6, s_4_17, 16, 2, 0},
    /* 18 */ {6, s_4_18, 16, 2, 0},
    /* 19 */ {4, s_4_19, -1, 1, 0},
    /* 20 */ {6, s_4_20, 19, 2, 0},
    /* 21 */ {6, s_4_21, -1, 1, 0},
    /* 22 */ {6, s_4_22, -1, 2, 0},
    /* 23 */ {6, s_4_23, -1, 1, 0},
    /* 24 */ {8, s_4_24, 23, 2, 0},
    /* 25 */ {8, s_4_25, 23, 2, 0},
    /* 26 */ {4, s_4_26, -1, 1, 0},
    /* 27 */ {6, s_4_27, 26, 2, 0},
    /* 28 */ {6, s_4_28, 26, 2, 0},
    /* 29 */ {2, s_4_29, -1, 1, 0},
    /* 30 */ {4, s_4_30, 29, 2, 0},
    /* 31 */ {4, s_4_31, 29, 2, 0},
    /* 32 */ {2, s_4_32, -1, 1, 0},
    /* 33 */ {4, s_4_33, 32, 2, 0},
    /* 34 */ {4, s_4_34, 32, 2, 0},
    /* 35 */ {4, s_4_35, -1, 2, 0},
    /* 36 */ {4, s_4_36, -1, 1, 0},
    /* 37 */ {4, s_4_37, -1, 2, 0},
    /* 38 */ {2, s_4_38, -1, 1, 0},
    /* 39 */ {4, s_4_39, 38, 2, 0},
    /* 40 */ {4, s_4_40, -1, 1, 0},
    /* 41 */ {6, s_4_41, 40, 2, 0},
    /* 42 */ {6, s_4_42, 40, 2, 0},
    /* 43 */ {4, s_4_43, -1, 1, 0},
    /* 44 */ {6, s_4_44, 43, 2, 0},
    /* 45 */ {6, s_4_45, 43, 1, 0}};

static const symbol s_5_0[2] = {0xD1, 0x83};
static const symbol s_5_1[4] = {0xD1, 0x8F, 0xD1, 0x85};
static const symbol s_5_2[6] = {0xD0, 0xB8, 0xD1, 0x8F, 0xD1, 0x85};
static const symbol s_5_3[4] = {0xD0, 0xB0, 0xD1, 0x85};
static const symbol s_5_4[2] = {0xD1, 0x8B};
static const symbol s_5_5[2] = {0xD1, 0x8C};
static const symbol s_5_6[2] = {0xD1, 0x8E};
static const symbol s_5_7[4] = {0xD1, 0x8C, 0xD1, 0x8E};
static const symbol s_5_8[4] = {0xD0, 0xB8, 0xD1, 0x8E};
static const symbol s_5_9[2] = {0xD1, 0x8F};
static const symbol s_5_10[4] = {0xD1, 0x8C, 0xD1, 0x8F};
static const symbol s_5_11[4] = {0xD0, 0xB8, 0xD1, 0x8F};
static const symbol s_5_12[2] = {0xD0, 0xB0};
static const symbol s_5_13[4] = {0xD0, 0xB5, 0xD0, 0xB2};
static const symbol s_5_14[4] = {0xD0, 0xBE, 0xD0, 0xB2};
static const symbol s_5_15[2] = {0xD0, 0xB5};
static const symbol s_5_16[4] = {0xD1, 0x8C, 0xD0, 0xB5};
static const symbol s_5_17[4] = {0xD0, 0xB8, 0xD0, 0xB5};
static const symbol s_5_18[2] = {0xD0, 0xB8};
static const symbol s_5_19[4] = {0xD0, 0xB5, 0xD0, 0xB8};
static const symbol s_5_20[4] = {0xD0, 0xB8, 0xD0, 0xB8};
static const symbol s_5_21[6] = {0xD1, 0x8F, 0xD0, 0xBC, 0xD0, 0xB8};
static const symbol s_5_22[8] = {0xD0, 0xB8, 0xD1, 0x8F, 0xD0, 0xBC, 0xD0, 0xB8};
static const symbol s_5_23[6] = {0xD0, 0xB0, 0xD0, 0xBC, 0xD0, 0xB8};
static const symbol s_5_24[2] = {0xD0, 0xB9};
static const symbol s_5_25[4] = {0xD0, 0xB5, 0xD0, 0xB9};
static const symbol s_5_26[6] = {0xD0, 0xB8, 0xD0, 0xB5, 0xD0, 0xB9};
static const symbol s_5_27[4] = {0xD0, 0xB8, 0xD0, 0xB9};
static const symbol s_5_28[4] = {0xD0, 0xBE, 0xD0, 0xB9};
static const symbol s_5_29[4] = {0xD1, 0x8F, 0xD0, 0xBC};
static const symbol s_5_30[6] = {0xD0, 0xB8, 0xD1, 0x8F, 0xD0, 0xBC};
static const symbol s_5_31[4] = {0xD0, 0xB0, 0xD0, 0xBC};
static const symbol s_5_32[4] = {0xD0, 0xB5, 0xD0, 0xBC};
static const symbol s_5_33[6] = {0xD0, 0xB8, 0xD0, 0xB5, 0xD0, 0xBC};
static const symbol s_5_34[4] = {0xD0, 0xBE, 0xD0, 0xBC};
static const symbol s_5_35[2] = {0xD0, 0xBE};

static const struct among a_5[36] = {
    /*  0 */ {2, s_5_0, -1, 1, 0},
    /*  1 */ {4, s_5_1, -1, 1, 0},
    /*  2 */ {6, s_5_2, 1, 1, 0},
    /*  3 */ {4, s_5_3, -1, 1, 0},
    /*  4 */ {2, s_5_4, -1, 1, 0},
    /*  5 */ {2, s_5_5, -1, 1, 0},
    /*  6 */ {2, s_5_6, -1, 1, 0},
    /*  7 */ {4, s_5_7, 6, 1, 0},
    /*  8 */ {4, s_5_8, 6, 1, 0},
    /*  9 */ {2, s_5_9, -1, 1, 0},
    /* 10 */ {4, s_5_10, 9, 1, 0},
    /* 11 */ {4, s_5_11, 9, 1, 0},
    /* 12 */ {2, s_5_12, -1, 1, 0},
    /* 13 */ {4, s_5_13, -1, 1, 0},
    /* 14 */ {4, s_5_14, -1, 1, 0},
    /* 15 */ {2, s_5_15, -1, 1, 0},
    /* 16 */ {4, s_5_16, 15, 1, 0},
    /* 17 */ {4, s_5_17, 15, 1, 0},
    /* 18 */ {2, s_5_18, -1, 1, 0},
    /* 19 */ {4, s_5_19, 18, 1, 0},
    /* 20 */ {4, s_5_20, 18, 1, 0},
    /* 21 */ {6, s_5_21, 18, 1, 0},
    /* 22 */ {8, s_5_22, 21, 1, 0},
    /* 23 */ {6, s_5_23, 18, 1, 0},
    /* 24 */ {2, s_5_24, -1, 1, 0},
    /* 25 */ {4, s_5_25, 24, 1, 0},
    /* 26 */ {6, s_5_26, 25, 1, 0},
    /* 27 */ {4, s_5_27, 24, 1, 0},
    /* 28 */ {4, s_5_28, 24, 1, 0},
    /* 29 */ {4, s_5_29, -1, 1, 0},
    /* 30 */ {6, s_5_30, 29, 1, 0},
    /* 31 */ {4, s_5_31, -1, 1, 0},
    /* 32 */ {4, s_5_32, -1, 1, 0},
    /* 33 */ {6, s_5_33, 32, 1, 0},
    /* 34 */ {4, s_5_34, -1, 1, 0},
    /* 35 */ {2, s_5_35, -1, 1, 0}};

static const symbol s_6_0[6] = {0xD0, 0xBE, 0xD1, 0x81, 0xD1, 0x82};
static const symbol s_6_1[8] = {0xD0, 0xBE, 0xD1, 0x81, 0xD1, 0x82, 0xD1, 0x8C};

static const struct among a_6[2] = {
    /*  0 */ {6, s_6_0, -1, 1, 0},
    /*  1 */ {8, s_6_1, -1, 1, 0}};

static const symbol s_7_0[6] = {0xD0, 0xB5, 0xD0, 0xB9, 0xD1, 0x88};
static const symbol s_7_1[2] = {0xD1, 0x8C};
static const symbol s_7_2[8] = {0xD0, 0xB5, 0xD0, 0xB9, 0xD1, 0x88, 0xD0, 0xB5};
static const symbol s_7_3[2] = {0xD0, 0xBD};

static const struct among a_7[4] = {
    /*  0 */ {6, s_7_0, -1, 1, 0},
    /*  1 */ {2, s_7_1, -1, 3, 0},
    /*  2 */ {8, s_7_2, -1, 1, 0},
    /*  3 */ {2, s_7_3, -1, 2, 0}};

static const unsigned char g_v[] = {33, 65, 8, 232};

static const symbol s_0[] = {0xD0, 0xB0};
static const symbol s_1[] = {0xD1, 0x8F};
static const symbol s_2[] = {0xD0, 0xB0};
static const symbol s_3[] = {0xD1, 0x8F};
static const symbol s_4[] = {0xD0, 0xB0};
static const symbol s_5[] = {0xD1, 0x8F};
static const symbol s_6[] = {0xD0, 0xBD};
static const symbol s_7[] = {0xD0, 0xBD};
static const symbol s_8[] = {0xD0, 0xBD};
static const symbol s_9[] = {0xD0, 0xB8};

static int r_mark_regions(struct SN_env *z) {
    z->I[0] = z->l;
    z->I[1] = z->l;
    {
        int c1 = z->c; /* do, line 61 */
        { /* gopast */ /* grouping v, line 62 */
            int ret = out_grouping_U(z, g_v, 1072, 1103, 1);
            if (ret < 0)
                goto lab0;
            z->c += ret;
        }
        z->I[0] = z->c; /* setmark pV, line 62 */
        { /* gopast */  /* non v, line 62 */
            int ret = in_grouping_U(z, g_v, 1072, 1103, 1);
            if (ret < 0)
                goto lab0;
            z->c += ret;
        }
        { /* gopast */ /* grouping v, line 63 */
            int ret = out_grouping_U(z, g_v, 1072, 1103, 1);
            if (ret < 0)
                goto lab0;
            z->c += ret;
        }
        { /* gopast */ /* non v, line 63 */
            int ret = in_grouping_U(z, g_v, 1072, 1103, 1);
            if (ret < 0)
                goto lab0;
            z->c += ret;
        }
        z->I[1] = z->c; /* setmark p2, line 63 */
    lab0:
        z->c = c1;
    }
    return 1;
}

static int r_R2(struct SN_env *z) {
    if (!(z->I[1] <= z->c))
        return 0;
    return 1;
}

static int r_perfective_gerund(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                       /* [, line 72 */
    among_var = find_among_b(z, a_0, 9); /* substring, line 72 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 72 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int m1 = z->l - z->c;
            (void)m1; /* or, line 76 */
            if (!(eq_s_b(z, 2, s_0)))
                goto lab1;
            goto lab0;
        lab1:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 2, s_1)))
                return 0;
        }
        lab0: {
            int ret = slice_del(z); /* delete, line 76 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_del(z); /* delete, line 83 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_adjective(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                        /* [, line 88 */
    among_var = find_among_b(z, a_1, 26); /* substring, line 88 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 88 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 97 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_adjectival(struct SN_env *z) {
    int among_var;
    {
        int ret = r_adjective(z);
        if (ret == 0)
            return 0; /* call adjective, line 102 */
        if (ret < 0)
            return ret;
    }
    {
        int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 109 */
        z->ket = z->c;                                /* [, line 110 */
        among_var = find_among_b(z, a_2, 8);          /* substring, line 110 */
        if (!(among_var)) {
            z->c = z->l - m_keep;
            goto lab0;
        }
        z->bra = z->c; /* ], line 110 */
        switch (among_var) {
            case 0: {
                z->c = z->l - m_keep;
                goto lab0;
            }
            case 1: {
                int m1 = z->l - z->c;
                (void)m1; /* or, line 115 */
                if (!(eq_s_b(z, 2, s_2)))
                    goto lab2;
                goto lab1;
            lab2:
                z->c = z->l - m1;
                if (!(eq_s_b(z, 2, s_3))) {
                    z->c = z->l - m_keep;
                    goto lab0;
                }
            }
            lab1: {
                int ret = slice_del(z); /* delete, line 115 */
                if (ret < 0)
                    return ret;
            } break;
            case 2: {
                int ret = slice_del(z); /* delete, line 122 */
                if (ret < 0)
                    return ret;
            } break;
        }
    lab0:;
    }
    return 1;
}

static int r_reflexive(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 129 */
    if (z->c - 3 <= z->lb || (z->p[z->c - 1] != 140 && z->p[z->c - 1] != 143))
        return 0;
    among_var = find_among_b(z, a_3, 2); /* substring, line 129 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 129 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 132 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_verb(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                        /* [, line 137 */
    among_var = find_among_b(z, a_4, 46); /* substring, line 137 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 137 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int m1 = z->l - z->c;
            (void)m1; /* or, line 143 */
            if (!(eq_s_b(z, 2, s_4)))
                goto lab1;
            goto lab0;
        lab1:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 2, s_5)))
                return 0;
        }
        lab0: {
            int ret = slice_del(z); /* delete, line 143 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_del(z); /* delete, line 151 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_noun(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                        /* [, line 160 */
    among_var = find_among_b(z, a_5, 36); /* substring, line 160 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 160 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 167 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_derivational(struct SN_env *z) {
    int among_var;
    z->ket = z->c; /* [, line 176 */
    if (z->c - 5 <= z->lb || (z->p[z->c - 1] != 130 && z->p[z->c - 1] != 140))
        return 0;
    among_var = find_among_b(z, a_6, 2); /* substring, line 176 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 176 */
    {
        int ret = r_R2(z);
        if (ret == 0)
            return 0; /* call R2, line 176 */
        if (ret < 0)
            return ret;
    }
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 179 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_tidy_up(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                       /* [, line 184 */
    among_var = find_among_b(z, a_7, 4); /* substring, line 184 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 184 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_del(z); /* delete, line 188 */
            if (ret < 0)
                return ret;
        }
            z->ket = z->c; /* [, line 189 */
            if (!(eq_s_b(z, 2, s_6)))
                return 0;
            z->bra = z->c; /* ], line 189 */
            if (!(eq_s_b(z, 2, s_7)))
                return 0;
            {
                int ret = slice_del(z); /* delete, line 189 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 2:
            if (!(eq_s_b(z, 2, s_8)))
                return 0;
            {
                int ret = slice_del(z); /* delete, line 192 */
                if (ret < 0)
                    return ret;
            }
            break;
        case 3: {
            int ret = slice_del(z); /* delete, line 194 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

extern int russian_UTF_8_stem(struct SN_env *z) {
    {
        int c1 = z->c; /* do, line 201 */
        {
            int ret = r_mark_regions(z);
            if (ret == 0)
                goto lab0; /* call mark_regions, line 201 */
            if (ret < 0)
                return ret;
        }
    lab0:
        z->c = c1;
    }
    z->lb = z->c;
    z->c = z->l; /* backwards, line 202 */

    {
        int mlimit; /* setlimit, line 202 */
        int m2 = z->l - z->c;
        (void)m2;
        if (z->c < z->I[0])
            return 0;
        z->c = z->I[0]; /* tomark, line 202 */
        mlimit = z->lb;
        z->lb = z->c;
        z->c = z->l - m2;
        {
            int m3 = z->l - z->c;
            (void)m3; /* do, line 203 */
            {
                int m4 = z->l - z->c;
                (void)m4; /* or, line 204 */
                {
                    int ret = r_perfective_gerund(z);
                    if (ret == 0)
                        goto lab3; /* call perfective_gerund, line 204 */
                    if (ret < 0)
                        return ret;
                }
                goto lab2;
            lab3:
                z->c = z->l - m4;
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 205 */
                    {
                        int ret = r_reflexive(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab4;
                        } /* call reflexive, line 205 */
                        if (ret < 0)
                            return ret;
                    }
                lab4:;
                }
                {
                    int m5 = z->l - z->c;
                    (void)m5; /* or, line 206 */
                    {
                        int ret = r_adjectival(z);
                        if (ret == 0)
                            goto lab6; /* call adjectival, line 206 */
                        if (ret < 0)
                            return ret;
                    }
                    goto lab5;
                lab6:
                    z->c = z->l - m5;
                    {
                        int ret = r_verb(z);
                        if (ret == 0)
                            goto lab7; /* call verb, line 206 */
                        if (ret < 0)
                            return ret;
                    }
                    goto lab5;
                lab7:
                    z->c = z->l - m5;
                    {
                        int ret = r_noun(z);
                        if (ret == 0)
                            goto lab1; /* call noun, line 206 */
                        if (ret < 0)
                            return ret;
                    }
                }
            lab5:;
            }
        lab2:
        lab1:
            z->c = z->l - m3;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 209 */
            z->ket = z->c;                                /* [, line 209 */
            if (!(eq_s_b(z, 2, s_9))) {
                z->c = z->l - m_keep;
                goto lab8;
            }
            z->bra = z->c; /* ], line 209 */
            {
                int ret = slice_del(z); /* delete, line 209 */
                if (ret < 0)
                    return ret;
            }
        lab8:;
        }
        {
            int m6 = z->l - z->c;
            (void)m6; /* do, line 212 */
            {
                int ret = r_derivational(z);
                if (ret == 0)
                    goto lab9; /* call derivational, line 212 */
                if (ret < 0)
                    return ret;
            }
        lab9:
            z->c = z->l - m6;
        }
        {
            int m7 = z->l - z->c;
            (void)m7; /* do, line 213 */
            {
                int ret = r_tidy_up(z);
                if (ret == 0)
                    goto lab10; /* call tidy_up, line 213 */
                if (ret < 0)
                    return ret;
            }
        lab10:
            z->c = z->l - m7;
        }
        z->lb = mlimit;
    }
    z->c = z->lb;
    return 1;
}

extern struct SN_env *russian_UTF_8_create_env(void) { return SN_create_env(0, 2, 0); }

extern void russian_UTF_8_close_env(struct SN_env *z) { SN_close_env(z, 0); }
