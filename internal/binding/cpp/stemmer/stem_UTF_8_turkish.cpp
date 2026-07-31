
/* This file was generated automatically by the Snowball to ANSI C compiler */

#include "header.h"

#ifdef __cplusplus
extern "C" {
#endif
extern int turkish_UTF_8_stem(struct SN_env *z);
#ifdef __cplusplus
}
#endif
static int r_stem_suffix_chain_before_ki(struct SN_env *z);
static int r_stem_noun_suffixes(struct SN_env *z);
static int r_stem_nominal_verb_suffixes(struct SN_env *z);
static int r_postlude(struct SN_env *z);
static int r_post_process_last_consonants(struct SN_env *z);
static int r_more_than_one_syllable_word(struct SN_env *z);
static int r_mark_suffix_with_optional_s_consonant(struct SN_env *z);
static int r_mark_suffix_with_optional_n_consonant(struct SN_env *z);
static int r_mark_suffix_with_optional_U_vowel(struct SN_env *z);
static int r_mark_suffix_with_optional_y_consonant(struct SN_env *z);
static int r_mark_ysA(struct SN_env *z);
static int r_mark_ymUs_(struct SN_env *z);
static int r_mark_yken(struct SN_env *z);
static int r_mark_yDU(struct SN_env *z);
static int r_mark_yUz(struct SN_env *z);
static int r_mark_yUm(struct SN_env *z);
static int r_mark_yU(struct SN_env *z);
static int r_mark_ylA(struct SN_env *z);
static int r_mark_yA(struct SN_env *z);
static int r_mark_possessives(struct SN_env *z);
static int r_mark_sUnUz(struct SN_env *z);
static int r_mark_sUn(struct SN_env *z);
static int r_mark_sU(struct SN_env *z);
static int r_mark_nUz(struct SN_env *z);
static int r_mark_nUn(struct SN_env *z);
static int r_mark_nU(struct SN_env *z);
static int r_mark_ndAn(struct SN_env *z);
static int r_mark_ndA(struct SN_env *z);
static int r_mark_ncA(struct SN_env *z);
static int r_mark_nA(struct SN_env *z);
static int r_mark_lArI(struct SN_env *z);
static int r_mark_lAr(struct SN_env *z);
static int r_mark_ki(struct SN_env *z);
static int r_mark_DUr(struct SN_env *z);
static int r_mark_DAn(struct SN_env *z);
static int r_mark_DA(struct SN_env *z);
static int r_mark_cAsInA(struct SN_env *z);
static int r_is_reserved_word(struct SN_env *z);
static int r_check_vowel_harmony(struct SN_env *z);
static int r_append_U_to_stems_ending_with_d_or_g(struct SN_env *z);
#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *turkish_UTF_8_create_env(void);
extern void turkish_UTF_8_close_env(struct SN_env *z);

#ifdef __cplusplus
}
#endif
static const symbol s_0_0[1] = {'m'};
static const symbol s_0_1[1] = {'n'};
static const symbol s_0_2[3] = {'m', 'i', 'z'};
static const symbol s_0_3[3] = {'n', 'i', 'z'};
static const symbol s_0_4[3] = {'m', 'u', 'z'};
static const symbol s_0_5[3] = {'n', 'u', 'z'};
static const symbol s_0_6[4] = {'m', 0xC4, 0xB1, 'z'};
static const symbol s_0_7[4] = {'n', 0xC4, 0xB1, 'z'};
static const symbol s_0_8[4] = {'m', 0xC3, 0xBC, 'z'};
static const symbol s_0_9[4] = {'n', 0xC3, 0xBC, 'z'};

static const struct among a_0[10] = {
    /*  0 */ {1, s_0_0, -1, -1, 0},
    /*  1 */ {1, s_0_1, -1, -1, 0},
    /*  2 */ {3, s_0_2, -1, -1, 0},
    /*  3 */ {3, s_0_3, -1, -1, 0},
    /*  4 */ {3, s_0_4, -1, -1, 0},
    /*  5 */ {3, s_0_5, -1, -1, 0},
    /*  6 */ {4, s_0_6, -1, -1, 0},
    /*  7 */ {4, s_0_7, -1, -1, 0},
    /*  8 */ {4, s_0_8, -1, -1, 0},
    /*  9 */ {4, s_0_9, -1, -1, 0}};

static const symbol s_1_0[4] = {'l', 'e', 'r', 'i'};
static const symbol s_1_1[5] = {'l', 'a', 'r', 0xC4, 0xB1};

static const struct among a_1[2] = {
    /*  0 */ {4, s_1_0, -1, -1, 0},
    /*  1 */ {5, s_1_1, -1, -1, 0}};

static const symbol s_2_0[2] = {'n', 'i'};
static const symbol s_2_1[2] = {'n', 'u'};
static const symbol s_2_2[3] = {'n', 0xC4, 0xB1};
static const symbol s_2_3[3] = {'n', 0xC3, 0xBC};

static const struct among a_2[4] = {
    /*  0 */ {2, s_2_0, -1, -1, 0},
    /*  1 */ {2, s_2_1, -1, -1, 0},
    /*  2 */ {3, s_2_2, -1, -1, 0},
    /*  3 */ {3, s_2_3, -1, -1, 0}};

static const symbol s_3_0[2] = {'i', 'n'};
static const symbol s_3_1[2] = {'u', 'n'};
static const symbol s_3_2[3] = {0xC4, 0xB1, 'n'};
static const symbol s_3_3[3] = {0xC3, 0xBC, 'n'};

static const struct among a_3[4] = {
    /*  0 */ {2, s_3_0, -1, -1, 0},
    /*  1 */ {2, s_3_1, -1, -1, 0},
    /*  2 */ {3, s_3_2, -1, -1, 0},
    /*  3 */ {3, s_3_3, -1, -1, 0}};

static const symbol s_4_0[1] = {'a'};
static const symbol s_4_1[1] = {'e'};

static const struct among a_4[2] = {
    /*  0 */ {1, s_4_0, -1, -1, 0},
    /*  1 */ {1, s_4_1, -1, -1, 0}};

static const symbol s_5_0[2] = {'n', 'a'};
static const symbol s_5_1[2] = {'n', 'e'};

static const struct among a_5[2] = {
    /*  0 */ {2, s_5_0, -1, -1, 0},
    /*  1 */ {2, s_5_1, -1, -1, 0}};

static const symbol s_6_0[2] = {'d', 'a'};
static const symbol s_6_1[2] = {'t', 'a'};
static const symbol s_6_2[2] = {'d', 'e'};
static const symbol s_6_3[2] = {'t', 'e'};

static const struct among a_6[4] = {
    /*  0 */ {2, s_6_0, -1, -1, 0},
    /*  1 */ {2, s_6_1, -1, -1, 0},
    /*  2 */ {2, s_6_2, -1, -1, 0},
    /*  3 */ {2, s_6_3, -1, -1, 0}};

static const symbol s_7_0[3] = {'n', 'd', 'a'};
static const symbol s_7_1[3] = {'n', 'd', 'e'};

static const struct among a_7[2] = {
    /*  0 */ {3, s_7_0, -1, -1, 0},
    /*  1 */ {3, s_7_1, -1, -1, 0}};

static const symbol s_8_0[3] = {'d', 'a', 'n'};
static const symbol s_8_1[3] = {'t', 'a', 'n'};
static const symbol s_8_2[3] = {'d', 'e', 'n'};
static const symbol s_8_3[3] = {'t', 'e', 'n'};

static const struct among a_8[4] = {
    /*  0 */ {3, s_8_0, -1, -1, 0},
    /*  1 */ {3, s_8_1, -1, -1, 0},
    /*  2 */ {3, s_8_2, -1, -1, 0},
    /*  3 */ {3, s_8_3, -1, -1, 0}};

static const symbol s_9_0[4] = {'n', 'd', 'a', 'n'};
static const symbol s_9_1[4] = {'n', 'd', 'e', 'n'};

static const struct among a_9[2] = {
    /*  0 */ {4, s_9_0, -1, -1, 0},
    /*  1 */ {4, s_9_1, -1, -1, 0}};

static const symbol s_10_0[2] = {'l', 'a'};
static const symbol s_10_1[2] = {'l', 'e'};

static const struct among a_10[2] = {
    /*  0 */ {2, s_10_0, -1, -1, 0},
    /*  1 */ {2, s_10_1, -1, -1, 0}};

static const symbol s_11_0[2] = {'c', 'a'};
static const symbol s_11_1[2] = {'c', 'e'};

static const struct among a_11[2] = {
    /*  0 */ {2, s_11_0, -1, -1, 0},
    /*  1 */ {2, s_11_1, -1, -1, 0}};

static const symbol s_12_0[2] = {'i', 'm'};
static const symbol s_12_1[2] = {'u', 'm'};
static const symbol s_12_2[3] = {0xC4, 0xB1, 'm'};
static const symbol s_12_3[3] = {0xC3, 0xBC, 'm'};

static const struct among a_12[4] = {
    /*  0 */ {2, s_12_0, -1, -1, 0},
    /*  1 */ {2, s_12_1, -1, -1, 0},
    /*  2 */ {3, s_12_2, -1, -1, 0},
    /*  3 */ {3, s_12_3, -1, -1, 0}};

static const symbol s_13_0[3] = {'s', 'i', 'n'};
static const symbol s_13_1[3] = {'s', 'u', 'n'};
static const symbol s_13_2[4] = {'s', 0xC4, 0xB1, 'n'};
static const symbol s_13_3[4] = {'s', 0xC3, 0xBC, 'n'};

static const struct among a_13[4] = {
    /*  0 */ {3, s_13_0, -1, -1, 0},
    /*  1 */ {3, s_13_1, -1, -1, 0},
    /*  2 */ {4, s_13_2, -1, -1, 0},
    /*  3 */ {4, s_13_3, -1, -1, 0}};

static const symbol s_14_0[2] = {'i', 'z'};
static const symbol s_14_1[2] = {'u', 'z'};
static const symbol s_14_2[3] = {0xC4, 0xB1, 'z'};
static const symbol s_14_3[3] = {0xC3, 0xBC, 'z'};

static const struct among a_14[4] = {
    /*  0 */ {2, s_14_0, -1, -1, 0},
    /*  1 */ {2, s_14_1, -1, -1, 0},
    /*  2 */ {3, s_14_2, -1, -1, 0},
    /*  3 */ {3, s_14_3, -1, -1, 0}};

static const symbol s_15_0[5] = {'s', 'i', 'n', 'i', 'z'};
static const symbol s_15_1[5] = {'s', 'u', 'n', 'u', 'z'};
static const symbol s_15_2[7] = {'s', 0xC4, 0xB1, 'n', 0xC4, 0xB1, 'z'};
static const symbol s_15_3[7] = {'s', 0xC3, 0xBC, 'n', 0xC3, 0xBC, 'z'};

static const struct among a_15[4] = {
    /*  0 */ {5, s_15_0, -1, -1, 0},
    /*  1 */ {5, s_15_1, -1, -1, 0},
    /*  2 */ {7, s_15_2, -1, -1, 0},
    /*  3 */ {7, s_15_3, -1, -1, 0}};

static const symbol s_16_0[3] = {'l', 'a', 'r'};
static const symbol s_16_1[3] = {'l', 'e', 'r'};

static const struct among a_16[2] = {
    /*  0 */ {3, s_16_0, -1, -1, 0},
    /*  1 */ {3, s_16_1, -1, -1, 0}};

static const symbol s_17_0[3] = {'n', 'i', 'z'};
static const symbol s_17_1[3] = {'n', 'u', 'z'};
static const symbol s_17_2[4] = {'n', 0xC4, 0xB1, 'z'};
static const symbol s_17_3[4] = {'n', 0xC3, 0xBC, 'z'};

static const struct among a_17[4] = {
    /*  0 */ {3, s_17_0, -1, -1, 0},
    /*  1 */ {3, s_17_1, -1, -1, 0},
    /*  2 */ {4, s_17_2, -1, -1, 0},
    /*  3 */ {4, s_17_3, -1, -1, 0}};

static const symbol s_18_0[3] = {'d', 'i', 'r'};
static const symbol s_18_1[3] = {'t', 'i', 'r'};
static const symbol s_18_2[3] = {'d', 'u', 'r'};
static const symbol s_18_3[3] = {'t', 'u', 'r'};
static const symbol s_18_4[4] = {'d', 0xC4, 0xB1, 'r'};
static const symbol s_18_5[4] = {'t', 0xC4, 0xB1, 'r'};
static const symbol s_18_6[4] = {'d', 0xC3, 0xBC, 'r'};
static const symbol s_18_7[4] = {'t', 0xC3, 0xBC, 'r'};

static const struct among a_18[8] = {
    /*  0 */ {3, s_18_0, -1, -1, 0},
    /*  1 */ {3, s_18_1, -1, -1, 0},
    /*  2 */ {3, s_18_2, -1, -1, 0},
    /*  3 */ {3, s_18_3, -1, -1, 0},
    /*  4 */ {4, s_18_4, -1, -1, 0},
    /*  5 */ {4, s_18_5, -1, -1, 0},
    /*  6 */ {4, s_18_6, -1, -1, 0},
    /*  7 */ {4, s_18_7, -1, -1, 0}};

static const symbol s_19_0[7] = {'c', 'a', 's', 0xC4, 0xB1, 'n', 'a'};
static const symbol s_19_1[6] = {'c', 'e', 's', 'i', 'n', 'e'};

static const struct among a_19[2] = {
    /*  0 */ {7, s_19_0, -1, -1, 0},
    /*  1 */ {6, s_19_1, -1, -1, 0}};

static const symbol s_20_0[2] = {'d', 'i'};
static const symbol s_20_1[2] = {'t', 'i'};
static const symbol s_20_2[3] = {'d', 'i', 'k'};
static const symbol s_20_3[3] = {'t', 'i', 'k'};
static const symbol s_20_4[3] = {'d', 'u', 'k'};
static const symbol s_20_5[3] = {'t', 'u', 'k'};
static const symbol s_20_6[4] = {'d', 0xC4, 0xB1, 'k'};
static const symbol s_20_7[4] = {'t', 0xC4, 0xB1, 'k'};
static const symbol s_20_8[4] = {'d', 0xC3, 0xBC, 'k'};
static const symbol s_20_9[4] = {'t', 0xC3, 0xBC, 'k'};
static const symbol s_20_10[3] = {'d', 'i', 'm'};
static const symbol s_20_11[3] = {'t', 'i', 'm'};
static const symbol s_20_12[3] = {'d', 'u', 'm'};
static const symbol s_20_13[3] = {'t', 'u', 'm'};
static const symbol s_20_14[4] = {'d', 0xC4, 0xB1, 'm'};
static const symbol s_20_15[4] = {'t', 0xC4, 0xB1, 'm'};
static const symbol s_20_16[4] = {'d', 0xC3, 0xBC, 'm'};
static const symbol s_20_17[4] = {'t', 0xC3, 0xBC, 'm'};
static const symbol s_20_18[3] = {'d', 'i', 'n'};
static const symbol s_20_19[3] = {'t', 'i', 'n'};
static const symbol s_20_20[3] = {'d', 'u', 'n'};
static const symbol s_20_21[3] = {'t', 'u', 'n'};
static const symbol s_20_22[4] = {'d', 0xC4, 0xB1, 'n'};
static const symbol s_20_23[4] = {'t', 0xC4, 0xB1, 'n'};
static const symbol s_20_24[4] = {'d', 0xC3, 0xBC, 'n'};
static const symbol s_20_25[4] = {'t', 0xC3, 0xBC, 'n'};
static const symbol s_20_26[2] = {'d', 'u'};
static const symbol s_20_27[2] = {'t', 'u'};
static const symbol s_20_28[3] = {'d', 0xC4, 0xB1};
static const symbol s_20_29[3] = {'t', 0xC4, 0xB1};
static const symbol s_20_30[3] = {'d', 0xC3, 0xBC};
static const symbol s_20_31[3] = {'t', 0xC3, 0xBC};

static const struct among a_20[32] = {
    /*  0 */ {2, s_20_0, -1, -1, 0},
    /*  1 */ {2, s_20_1, -1, -1, 0},
    /*  2 */ {3, s_20_2, -1, -1, 0},
    /*  3 */ {3, s_20_3, -1, -1, 0},
    /*  4 */ {3, s_20_4, -1, -1, 0},
    /*  5 */ {3, s_20_5, -1, -1, 0},
    /*  6 */ {4, s_20_6, -1, -1, 0},
    /*  7 */ {4, s_20_7, -1, -1, 0},
    /*  8 */ {4, s_20_8, -1, -1, 0},
    /*  9 */ {4, s_20_9, -1, -1, 0},
    /* 10 */ {3, s_20_10, -1, -1, 0},
    /* 11 */ {3, s_20_11, -1, -1, 0},
    /* 12 */ {3, s_20_12, -1, -1, 0},
    /* 13 */ {3, s_20_13, -1, -1, 0},
    /* 14 */ {4, s_20_14, -1, -1, 0},
    /* 15 */ {4, s_20_15, -1, -1, 0},
    /* 16 */ {4, s_20_16, -1, -1, 0},
    /* 17 */ {4, s_20_17, -1, -1, 0},
    /* 18 */ {3, s_20_18, -1, -1, 0},
    /* 19 */ {3, s_20_19, -1, -1, 0},
    /* 20 */ {3, s_20_20, -1, -1, 0},
    /* 21 */ {3, s_20_21, -1, -1, 0},
    /* 22 */ {4, s_20_22, -1, -1, 0},
    /* 23 */ {4, s_20_23, -1, -1, 0},
    /* 24 */ {4, s_20_24, -1, -1, 0},
    /* 25 */ {4, s_20_25, -1, -1, 0},
    /* 26 */ {2, s_20_26, -1, -1, 0},
    /* 27 */ {2, s_20_27, -1, -1, 0},
    /* 28 */ {3, s_20_28, -1, -1, 0},
    /* 29 */ {3, s_20_29, -1, -1, 0},
    /* 30 */ {3, s_20_30, -1, -1, 0},
    /* 31 */ {3, s_20_31, -1, -1, 0}};

static const symbol s_21_0[2] = {'s', 'a'};
static const symbol s_21_1[2] = {'s', 'e'};
static const symbol s_21_2[3] = {'s', 'a', 'k'};
static const symbol s_21_3[3] = {'s', 'e', 'k'};
static const symbol s_21_4[3] = {'s', 'a', 'm'};
static const symbol s_21_5[3] = {'s', 'e', 'm'};
static const symbol s_21_6[3] = {'s', 'a', 'n'};
static const symbol s_21_7[3] = {'s', 'e', 'n'};

static const struct among a_21[8] = {
    /*  0 */ {2, s_21_0, -1, -1, 0},
    /*  1 */ {2, s_21_1, -1, -1, 0},
    /*  2 */ {3, s_21_2, -1, -1, 0},
    /*  3 */ {3, s_21_3, -1, -1, 0},
    /*  4 */ {3, s_21_4, -1, -1, 0},
    /*  5 */ {3, s_21_5, -1, -1, 0},
    /*  6 */ {3, s_21_6, -1, -1, 0},
    /*  7 */ {3, s_21_7, -1, -1, 0}};

static const symbol s_22_0[4] = {'m', 'i', 0xC5, 0x9F};
static const symbol s_22_1[4] = {'m', 'u', 0xC5, 0x9F};
static const symbol s_22_2[5] = {'m', 0xC4, 0xB1, 0xC5, 0x9F};
static const symbol s_22_3[5] = {'m', 0xC3, 0xBC, 0xC5, 0x9F};

static const struct among a_22[4] = {
    /*  0 */ {4, s_22_0, -1, -1, 0},
    /*  1 */ {4, s_22_1, -1, -1, 0},
    /*  2 */ {5, s_22_2, -1, -1, 0},
    /*  3 */ {5, s_22_3, -1, -1, 0}};

static const symbol s_23_0[1] = {'b'};
static const symbol s_23_1[1] = {'c'};
static const symbol s_23_2[1] = {'d'};
static const symbol s_23_3[2] = {0xC4, 0x9F};

static const struct among a_23[4] = {
    /*  0 */ {1, s_23_0, -1, 1, 0},
    /*  1 */ {1, s_23_1, -1, 2, 0},
    /*  2 */ {1, s_23_2, -1, 3, 0},
    /*  3 */ {2, s_23_3, -1, 4, 0}};

static const unsigned char g_vowel[] = {17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 8, 0, 0, 0, 0, 0, 0, 1};

static const unsigned char g_U[] = {1, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 1};

static const unsigned char g_vowel1[] = {1, 64, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1};

static const unsigned char g_vowel2[] = {17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 130};

static const unsigned char g_vowel3[] = {1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1};

static const unsigned char g_vowel4[] = {17};

static const unsigned char g_vowel5[] = {65};

static const unsigned char g_vowel6[] = {65};

static const symbol s_0[] = {'a'};
static const symbol s_1[] = {'e'};
static const symbol s_2[] = {0xC4, 0xB1};
static const symbol s_3[] = {'i'};
static const symbol s_4[] = {'o'};
static const symbol s_5[] = {0xC3, 0xB6};
static const symbol s_6[] = {'u'};
static const symbol s_7[] = {0xC3, 0xBC};
static const symbol s_8[] = {'n'};
static const symbol s_9[] = {'n'};
static const symbol s_10[] = {'s'};
static const symbol s_11[] = {'s'};
static const symbol s_12[] = {'y'};
static const symbol s_13[] = {'y'};
static const symbol s_14[] = {'k', 'i'};
static const symbol s_15[] = {'k', 'e', 'n'};
static const symbol s_16[] = {'p'};
static const symbol s_17[] = {0xC3, 0xA7};
static const symbol s_18[] = {'t'};
static const symbol s_19[] = {'k'};
static const symbol s_20[] = {'d'};
static const symbol s_21[] = {'g'};
static const symbol s_22[] = {'a'};
static const symbol s_23[] = {0xC4, 0xB1};
static const symbol s_24[] = {0xC4, 0xB1};
static const symbol s_25[] = {'e'};
static const symbol s_26[] = {'i'};
static const symbol s_27[] = {'i'};
static const symbol s_28[] = {'o'};
static const symbol s_29[] = {'u'};
static const symbol s_30[] = {'u'};
static const symbol s_31[] = {0xC3, 0xB6};
static const symbol s_32[] = {0xC3, 0xBC};
static const symbol s_33[] = {0xC3, 0xBC};
static const symbol s_34[] = {'a', 'd'};
static const symbol s_35[] = {'s', 'o', 'y', 'a', 'd'};

static int r_check_vowel_harmony(struct SN_env *z) {
    {
        int m_test = z->l - z->c; /* test, line 112 */
        if (out_grouping_b_U(z, g_vowel, 97, 305, 1) < 0)
            return 0; /* goto */ /* grouping vowel, line 114 */
        {
            int m1 = z->l - z->c;
            (void)m1; /* or, line 116 */
            if (!(eq_s_b(z, 1, s_0)))
                goto lab1;
            if (out_grouping_b_U(z, g_vowel1, 97, 305, 1) < 0)
                goto lab1; /* goto */ /* grouping vowel1, line 116 */
            goto lab0;
        lab1:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 1, s_1)))
                goto lab2;
            if (out_grouping_b_U(z, g_vowel2, 101, 252, 1) < 0)
                goto lab2; /* goto */ /* grouping vowel2, line 117 */
            goto lab0;
        lab2:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 2, s_2)))
                goto lab3;
            if (out_grouping_b_U(z, g_vowel3, 97, 305, 1) < 0)
                goto lab3; /* goto */ /* grouping vowel3, line 118 */
            goto lab0;
        lab3:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 1, s_3)))
                goto lab4;
            if (out_grouping_b_U(z, g_vowel4, 101, 105, 1) < 0)
                goto lab4; /* goto */ /* grouping vowel4, line 119 */
            goto lab0;
        lab4:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 1, s_4)))
                goto lab5;
            if (out_grouping_b_U(z, g_vowel5, 111, 117, 1) < 0)
                goto lab5; /* goto */ /* grouping vowel5, line 120 */
            goto lab0;
        lab5:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 2, s_5)))
                goto lab6;
            if (out_grouping_b_U(z, g_vowel6, 246, 252, 1) < 0)
                goto lab6; /* goto */ /* grouping vowel6, line 121 */
            goto lab0;
        lab6:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 1, s_6)))
                goto lab7;
            if (out_grouping_b_U(z, g_vowel5, 111, 117, 1) < 0)
                goto lab7; /* goto */ /* grouping vowel5, line 122 */
            goto lab0;
        lab7:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 2, s_7)))
                return 0;
            if (out_grouping_b_U(z, g_vowel6, 246, 252, 1) < 0)
                return 0; /* goto */ /* grouping vowel6, line 123 */
        }
    lab0:
        z->c = z->l - m_test;
    }
    return 1;
}

static int r_mark_suffix_with_optional_n_consonant(struct SN_env *z) {
    {
        int m1 = z->l - z->c;
        (void)m1; /* or, line 134 */
        {
            int m_test = z->l - z->c; /* test, line 133 */
            if (!(eq_s_b(z, 1, s_8)))
                goto lab1;
            z->c = z->l - m_test;
        }
        {
            int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
            if (ret < 0)
                goto lab1;
            z->c = ret; /* next, line 133 */
        }
        {
            int m_test = z->l - z->c; /* test, line 133 */
            if (in_grouping_b_U(z, g_vowel, 97, 305, 0))
                goto lab1;
            z->c = z->l - m_test;
        }
        goto lab0;
    lab1:
        z->c = z->l - m1;
        {
            int m2 = z->l - z->c;
            (void)m2; /* not, line 135 */
            {
                int m_test = z->l - z->c; /* test, line 135 */
                if (!(eq_s_b(z, 1, s_9)))
                    goto lab2;
                z->c = z->l - m_test;
            }
            return 0;
        lab2:
            z->c = z->l - m2;
        }
        {
            int m_test = z->l - z->c; /* test, line 135 */
            {
                int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
                if (ret < 0)
                    return 0;
                z->c = ret; /* next, line 135 */
            }
            {
                int m_test = z->l - z->c; /* test, line 135 */
                if (in_grouping_b_U(z, g_vowel, 97, 305, 0))
                    return 0;
                z->c = z->l - m_test;
            }
            z->c = z->l - m_test;
        }
    }
lab0:
    return 1;
}

static int r_mark_suffix_with_optional_s_consonant(struct SN_env *z) {
    {
        int m1 = z->l - z->c;
        (void)m1; /* or, line 145 */
        {
            int m_test = z->l - z->c; /* test, line 144 */
            if (!(eq_s_b(z, 1, s_10)))
                goto lab1;
            z->c = z->l - m_test;
        }
        {
            int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
            if (ret < 0)
                goto lab1;
            z->c = ret; /* next, line 144 */
        }
        {
            int m_test = z->l - z->c; /* test, line 144 */
            if (in_grouping_b_U(z, g_vowel, 97, 305, 0))
                goto lab1;
            z->c = z->l - m_test;
        }
        goto lab0;
    lab1:
        z->c = z->l - m1;
        {
            int m2 = z->l - z->c;
            (void)m2; /* not, line 146 */
            {
                int m_test = z->l - z->c; /* test, line 146 */
                if (!(eq_s_b(z, 1, s_11)))
                    goto lab2;
                z->c = z->l - m_test;
            }
            return 0;
        lab2:
            z->c = z->l - m2;
        }
        {
            int m_test = z->l - z->c; /* test, line 146 */
            {
                int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
                if (ret < 0)
                    return 0;
                z->c = ret; /* next, line 146 */
            }
            {
                int m_test = z->l - z->c; /* test, line 146 */
                if (in_grouping_b_U(z, g_vowel, 97, 305, 0))
                    return 0;
                z->c = z->l - m_test;
            }
            z->c = z->l - m_test;
        }
    }
lab0:
    return 1;
}

static int r_mark_suffix_with_optional_y_consonant(struct SN_env *z) {
    {
        int m1 = z->l - z->c;
        (void)m1; /* or, line 155 */
        {
            int m_test = z->l - z->c; /* test, line 154 */
            if (!(eq_s_b(z, 1, s_12)))
                goto lab1;
            z->c = z->l - m_test;
        }
        {
            int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
            if (ret < 0)
                goto lab1;
            z->c = ret; /* next, line 154 */
        }
        {
            int m_test = z->l - z->c; /* test, line 154 */
            if (in_grouping_b_U(z, g_vowel, 97, 305, 0))
                goto lab1;
            z->c = z->l - m_test;
        }
        goto lab0;
    lab1:
        z->c = z->l - m1;
        {
            int m2 = z->l - z->c;
            (void)m2; /* not, line 156 */
            {
                int m_test = z->l - z->c; /* test, line 156 */
                if (!(eq_s_b(z, 1, s_13)))
                    goto lab2;
                z->c = z->l - m_test;
            }
            return 0;
        lab2:
            z->c = z->l - m2;
        }
        {
            int m_test = z->l - z->c; /* test, line 156 */
            {
                int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
                if (ret < 0)
                    return 0;
                z->c = ret; /* next, line 156 */
            }
            {
                int m_test = z->l - z->c; /* test, line 156 */
                if (in_grouping_b_U(z, g_vowel, 97, 305, 0))
                    return 0;
                z->c = z->l - m_test;
            }
            z->c = z->l - m_test;
        }
    }
lab0:
    return 1;
}

static int r_mark_suffix_with_optional_U_vowel(struct SN_env *z) {
    {
        int m1 = z->l - z->c;
        (void)m1; /* or, line 161 */
        {
            int m_test = z->l - z->c; /* test, line 160 */
            if (in_grouping_b_U(z, g_U, 105, 305, 0))
                goto lab1;
            z->c = z->l - m_test;
        }
        {
            int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
            if (ret < 0)
                goto lab1;
            z->c = ret; /* next, line 160 */
        }
        {
            int m_test = z->l - z->c; /* test, line 160 */
            if (out_grouping_b_U(z, g_vowel, 97, 305, 0))
                goto lab1;
            z->c = z->l - m_test;
        }
        goto lab0;
    lab1:
        z->c = z->l - m1;
        {
            int m2 = z->l - z->c;
            (void)m2; /* not, line 162 */
            {
                int m_test = z->l - z->c; /* test, line 162 */
                if (in_grouping_b_U(z, g_U, 105, 305, 0))
                    goto lab2;
                z->c = z->l - m_test;
            }
            return 0;
        lab2:
            z->c = z->l - m2;
        }
        {
            int m_test = z->l - z->c; /* test, line 162 */
            {
                int ret = skip_utf8(z->p, z->c, z->lb, 0, -1);
                if (ret < 0)
                    return 0;
                z->c = ret; /* next, line 162 */
            }
            {
                int m_test = z->l - z->c; /* test, line 162 */
                if (out_grouping_b_U(z, g_vowel, 97, 305, 0))
                    return 0;
                z->c = z->l - m_test;
            }
            z->c = z->l - m_test;
        }
    }
lab0:
    return 1;
}

static int r_mark_possessives(struct SN_env *z) {
    if (z->c <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((67133440 >> (z->p[z->c - 1] & 0x1f)) & 1))
        return 0;
    if (!(find_among_b(z, a_0, 10)))
        return 0; /* among, line 167 */
    {
        int ret = r_mark_suffix_with_optional_U_vowel(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_U_vowel, line 169 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_sU(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 173 */
        if (ret < 0)
            return ret;
    }
    if (in_grouping_b_U(z, g_U, 105, 305, 0))
        return 0;
    {
        int ret = r_mark_suffix_with_optional_s_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_s_consonant, line 175 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_lArI(struct SN_env *z) {
    if (z->c - 3 <= z->lb || (z->p[z->c - 1] != 105 && z->p[z->c - 1] != 177))
        return 0;
    if (!(find_among_b(z, a_1, 2)))
        return 0; /* among, line 179 */
    return 1;
}

static int r_mark_yU(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 183 */
        if (ret < 0)
            return ret;
    }
    if (in_grouping_b_U(z, g_U, 105, 305, 0))
        return 0;
    {
        int ret = r_mark_suffix_with_optional_y_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_y_consonant, line 185 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_nU(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 189 */
        if (ret < 0)
            return ret;
    }
    if (!(find_among_b(z, a_2, 4)))
        return 0; /* among, line 190 */
    return 1;
}

static int r_mark_nUn(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 194 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 1 <= z->lb || z->p[z->c - 1] != 110)
        return 0;
    if (!(find_among_b(z, a_3, 4)))
        return 0; /* among, line 195 */
    {
        int ret = r_mark_suffix_with_optional_n_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_n_consonant, line 196 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_yA(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 200 */
        if (ret < 0)
            return ret;
    }
    if (z->c <= z->lb || (z->p[z->c - 1] != 97 && z->p[z->c - 1] != 101))
        return 0;
    if (!(find_among_b(z, a_4, 2)))
        return 0; /* among, line 201 */
    {
        int ret = r_mark_suffix_with_optional_y_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_y_consonant, line 202 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_nA(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 206 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 1 <= z->lb || (z->p[z->c - 1] != 97 && z->p[z->c - 1] != 101))
        return 0;
    if (!(find_among_b(z, a_5, 2)))
        return 0; /* among, line 207 */
    return 1;
}

static int r_mark_DA(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 211 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 1 <= z->lb || (z->p[z->c - 1] != 97 && z->p[z->c - 1] != 101))
        return 0;
    if (!(find_among_b(z, a_6, 4)))
        return 0; /* among, line 212 */
    return 1;
}

static int r_mark_ndA(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 216 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 2 <= z->lb || (z->p[z->c - 1] != 97 && z->p[z->c - 1] != 101))
        return 0;
    if (!(find_among_b(z, a_7, 2)))
        return 0; /* among, line 217 */
    return 1;
}

static int r_mark_DAn(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 221 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 2 <= z->lb || z->p[z->c - 1] != 110)
        return 0;
    if (!(find_among_b(z, a_8, 4)))
        return 0; /* among, line 222 */
    return 1;
}

static int r_mark_ndAn(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 226 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 3 <= z->lb || z->p[z->c - 1] != 110)
        return 0;
    if (!(find_among_b(z, a_9, 2)))
        return 0; /* among, line 227 */
    return 1;
}

static int r_mark_ylA(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 231 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 1 <= z->lb || (z->p[z->c - 1] != 97 && z->p[z->c - 1] != 101))
        return 0;
    if (!(find_among_b(z, a_10, 2)))
        return 0; /* among, line 232 */
    {
        int ret = r_mark_suffix_with_optional_y_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_y_consonant, line 233 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_ki(struct SN_env *z) {
    if (!(eq_s_b(z, 2, s_14)))
        return 0;
    return 1;
}

static int r_mark_ncA(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 241 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 1 <= z->lb || (z->p[z->c - 1] != 97 && z->p[z->c - 1] != 101))
        return 0;
    if (!(find_among_b(z, a_11, 2)))
        return 0; /* among, line 242 */
    {
        int ret = r_mark_suffix_with_optional_n_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_n_consonant, line 243 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_yUm(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 247 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 1 <= z->lb || z->p[z->c - 1] != 109)
        return 0;
    if (!(find_among_b(z, a_12, 4)))
        return 0; /* among, line 248 */
    {
        int ret = r_mark_suffix_with_optional_y_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_y_consonant, line 249 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_sUn(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 253 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 2 <= z->lb || z->p[z->c - 1] != 110)
        return 0;
    if (!(find_among_b(z, a_13, 4)))
        return 0; /* among, line 254 */
    return 1;
}

static int r_mark_yUz(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 258 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 1 <= z->lb || z->p[z->c - 1] != 122)
        return 0;
    if (!(find_among_b(z, a_14, 4)))
        return 0; /* among, line 259 */
    {
        int ret = r_mark_suffix_with_optional_y_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_y_consonant, line 260 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_sUnUz(struct SN_env *z) {
    if (z->c - 4 <= z->lb || z->p[z->c - 1] != 122)
        return 0;
    if (!(find_among_b(z, a_15, 4)))
        return 0; /* among, line 264 */
    return 1;
}

static int r_mark_lAr(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 268 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 2 <= z->lb || z->p[z->c - 1] != 114)
        return 0;
    if (!(find_among_b(z, a_16, 2)))
        return 0; /* among, line 269 */
    return 1;
}

static int r_mark_nUz(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 273 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 2 <= z->lb || z->p[z->c - 1] != 122)
        return 0;
    if (!(find_among_b(z, a_17, 4)))
        return 0; /* among, line 274 */
    return 1;
}

static int r_mark_DUr(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 278 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 2 <= z->lb || z->p[z->c - 1] != 114)
        return 0;
    if (!(find_among_b(z, a_18, 8)))
        return 0; /* among, line 279 */
    return 1;
}

static int r_mark_cAsInA(struct SN_env *z) {
    if (z->c - 5 <= z->lb || (z->p[z->c - 1] != 97 && z->p[z->c - 1] != 101))
        return 0;
    if (!(find_among_b(z, a_19, 2)))
        return 0; /* among, line 283 */
    return 1;
}

static int r_mark_yDU(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 287 */
        if (ret < 0)
            return ret;
    }
    if (!(find_among_b(z, a_20, 32)))
        return 0; /* among, line 288 */
    {
        int ret = r_mark_suffix_with_optional_y_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_y_consonant, line 292 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_ysA(struct SN_env *z) {
    if (z->c - 1 <= z->lb || z->p[z->c - 1] >> 5 != 3 || !((26658 >> (z->p[z->c - 1] & 0x1f)) & 1))
        return 0;
    if (!(find_among_b(z, a_21, 8)))
        return 0; /* among, line 297 */
    {
        int ret = r_mark_suffix_with_optional_y_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_y_consonant, line 298 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_ymUs_(struct SN_env *z) {
    {
        int ret = r_check_vowel_harmony(z);
        if (ret == 0)
            return 0; /* call check_vowel_harmony, line 302 */
        if (ret < 0)
            return ret;
    }
    if (z->c - 3 <= z->lb || z->p[z->c - 1] != 159)
        return 0;
    if (!(find_among_b(z, a_22, 4)))
        return 0; /* among, line 303 */
    {
        int ret = r_mark_suffix_with_optional_y_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_y_consonant, line 304 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_mark_yken(struct SN_env *z) {
    if (!(eq_s_b(z, 3, s_15)))
        return 0;
    {
        int ret = r_mark_suffix_with_optional_y_consonant(z);
        if (ret == 0)
            return 0; /* call mark_suffix_with_optional_y_consonant, line 308 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_stem_nominal_verb_suffixes(struct SN_env *z) {
    z->ket = z->c; /* [, line 312 */
    z->B[0] = 1;   /* set continue_stemming_noun_suffixes, line 313 */
    {
        int m1 = z->l - z->c;
        (void)m1; /* or, line 315 */
        {
            int m2 = z->l - z->c;
            (void)m2; /* or, line 314 */
            {
                int ret = r_mark_ymUs_(z);
                if (ret == 0)
                    goto lab3; /* call mark_ymUs_, line 314 */
                if (ret < 0)
                    return ret;
            }
            goto lab2;
        lab3:
            z->c = z->l - m2;
            {
                int ret = r_mark_yDU(z);
                if (ret == 0)
                    goto lab4; /* call mark_yDU, line 314 */
                if (ret < 0)
                    return ret;
            }
            goto lab2;
        lab4:
            z->c = z->l - m2;
            {
                int ret = r_mark_ysA(z);
                if (ret == 0)
                    goto lab5; /* call mark_ysA, line 314 */
                if (ret < 0)
                    return ret;
            }
            goto lab2;
        lab5:
            z->c = z->l - m2;
            {
                int ret = r_mark_yken(z);
                if (ret == 0)
                    goto lab1; /* call mark_yken, line 314 */
                if (ret < 0)
                    return ret;
            }
        }
    lab2:
        goto lab0;
    lab1:
        z->c = z->l - m1;
        {
            int ret = r_mark_cAsInA(z);
            if (ret == 0)
                goto lab6; /* call mark_cAsInA, line 316 */
            if (ret < 0)
                return ret;
        }
        {
            int m3 = z->l - z->c;
            (void)m3; /* or, line 316 */
            {
                int ret = r_mark_sUnUz(z);
                if (ret == 0)
                    goto lab8; /* call mark_sUnUz, line 316 */
                if (ret < 0)
                    return ret;
            }
            goto lab7;
        lab8:
            z->c = z->l - m3;
            {
                int ret = r_mark_lAr(z);
                if (ret == 0)
                    goto lab9; /* call mark_lAr, line 316 */
                if (ret < 0)
                    return ret;
            }
            goto lab7;
        lab9:
            z->c = z->l - m3;
            {
                int ret = r_mark_yUm(z);
                if (ret == 0)
                    goto lab10; /* call mark_yUm, line 316 */
                if (ret < 0)
                    return ret;
            }
            goto lab7;
        lab10:
            z->c = z->l - m3;
            {
                int ret = r_mark_sUn(z);
                if (ret == 0)
                    goto lab11; /* call mark_sUn, line 316 */
                if (ret < 0)
                    return ret;
            }
            goto lab7;
        lab11:
            z->c = z->l - m3;
            {
                int ret = r_mark_yUz(z);
                if (ret == 0)
                    goto lab12; /* call mark_yUz, line 316 */
                if (ret < 0)
                    return ret;
            }
            goto lab7;
        lab12:
            z->c = z->l - m3;
        }
    lab7: {
        int ret = r_mark_ymUs_(z);
        if (ret == 0)
            goto lab6; /* call mark_ymUs_, line 316 */
        if (ret < 0)
            return ret;
    }
        goto lab0;
    lab6:
        z->c = z->l - m1;
        {
            int ret = r_mark_lAr(z);
            if (ret == 0)
                goto lab13; /* call mark_lAr, line 319 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 319 */
        {
            int ret = slice_del(z); /* delete, line 319 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 319 */
            z->ket = z->c;                                /* [, line 319 */
            {
                int m4 = z->l - z->c;
                (void)m4; /* or, line 319 */
                {
                    int ret = r_mark_DUr(z);
                    if (ret == 0)
                        goto lab16; /* call mark_DUr, line 319 */
                    if (ret < 0)
                        return ret;
                }
                goto lab15;
            lab16:
                z->c = z->l - m4;
                {
                    int ret = r_mark_yDU(z);
                    if (ret == 0)
                        goto lab17; /* call mark_yDU, line 319 */
                    if (ret < 0)
                        return ret;
                }
                goto lab15;
            lab17:
                z->c = z->l - m4;
                {
                    int ret = r_mark_ysA(z);
                    if (ret == 0)
                        goto lab18; /* call mark_ysA, line 319 */
                    if (ret < 0)
                        return ret;
                }
                goto lab15;
            lab18:
                z->c = z->l - m4;
                {
                    int ret = r_mark_ymUs_(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab14;
                    } /* call mark_ymUs_, line 319 */
                    if (ret < 0)
                        return ret;
                }
            }
        lab15:
        lab14:;
        }
        z->B[0] = 0; /* unset continue_stemming_noun_suffixes, line 320 */
        goto lab0;
    lab13:
        z->c = z->l - m1;
        {
            int ret = r_mark_nUz(z);
            if (ret == 0)
                goto lab19; /* call mark_nUz, line 323 */
            if (ret < 0)
                return ret;
        }
        {
            int m5 = z->l - z->c;
            (void)m5; /* or, line 323 */
            {
                int ret = r_mark_yDU(z);
                if (ret == 0)
                    goto lab21; /* call mark_yDU, line 323 */
                if (ret < 0)
                    return ret;
            }
            goto lab20;
        lab21:
            z->c = z->l - m5;
            {
                int ret = r_mark_ysA(z);
                if (ret == 0)
                    goto lab19; /* call mark_ysA, line 323 */
                if (ret < 0)
                    return ret;
            }
        }
    lab20:
        goto lab0;
    lab19:
        z->c = z->l - m1;
        {
            int m6 = z->l - z->c;
            (void)m6; /* or, line 325 */
            {
                int ret = r_mark_sUnUz(z);
                if (ret == 0)
                    goto lab24; /* call mark_sUnUz, line 325 */
                if (ret < 0)
                    return ret;
            }
            goto lab23;
        lab24:
            z->c = z->l - m6;
            {
                int ret = r_mark_yUz(z);
                if (ret == 0)
                    goto lab25; /* call mark_yUz, line 325 */
                if (ret < 0)
                    return ret;
            }
            goto lab23;
        lab25:
            z->c = z->l - m6;
            {
                int ret = r_mark_sUn(z);
                if (ret == 0)
                    goto lab26; /* call mark_sUn, line 325 */
                if (ret < 0)
                    return ret;
            }
            goto lab23;
        lab26:
            z->c = z->l - m6;
            {
                int ret = r_mark_yUm(z);
                if (ret == 0)
                    goto lab22; /* call mark_yUm, line 325 */
                if (ret < 0)
                    return ret;
            }
        }
    lab23:
        z->bra = z->c; /* ], line 325 */
        {
            int ret = slice_del(z); /* delete, line 325 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 325 */
            z->ket = z->c;                                /* [, line 325 */
            {
                int ret = r_mark_ymUs_(z);
                if (ret == 0) {
                    z->c = z->l - m_keep;
                    goto lab27;
                } /* call mark_ymUs_, line 325 */
                if (ret < 0)
                    return ret;
            }
        lab27:;
        }
        goto lab0;
    lab22:
        z->c = z->l - m1;
        {
            int ret = r_mark_DUr(z);
            if (ret == 0)
                return 0; /* call mark_DUr, line 327 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 327 */
        {
            int ret = slice_del(z); /* delete, line 327 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 327 */
            z->ket = z->c;                                /* [, line 327 */
            {
                int m7 = z->l - z->c;
                (void)m7; /* or, line 327 */
                {
                    int ret = r_mark_sUnUz(z);
                    if (ret == 0)
                        goto lab30; /* call mark_sUnUz, line 327 */
                    if (ret < 0)
                        return ret;
                }
                goto lab29;
            lab30:
                z->c = z->l - m7;
                {
                    int ret = r_mark_lAr(z);
                    if (ret == 0)
                        goto lab31; /* call mark_lAr, line 327 */
                    if (ret < 0)
                        return ret;
                }
                goto lab29;
            lab31:
                z->c = z->l - m7;
                {
                    int ret = r_mark_yUm(z);
                    if (ret == 0)
                        goto lab32; /* call mark_yUm, line 327 */
                    if (ret < 0)
                        return ret;
                }
                goto lab29;
            lab32:
                z->c = z->l - m7;
                {
                    int ret = r_mark_sUn(z);
                    if (ret == 0)
                        goto lab33; /* call mark_sUn, line 327 */
                    if (ret < 0)
                        return ret;
                }
                goto lab29;
            lab33:
                z->c = z->l - m7;
                {
                    int ret = r_mark_yUz(z);
                    if (ret == 0)
                        goto lab34; /* call mark_yUz, line 327 */
                    if (ret < 0)
                        return ret;
                }
                goto lab29;
            lab34:
                z->c = z->l - m7;
            }
        lab29: {
            int ret = r_mark_ymUs_(z);
            if (ret == 0) {
                z->c = z->l - m_keep;
                goto lab28;
            } /* call mark_ymUs_, line 327 */
            if (ret < 0)
                return ret;
        }
        lab28:;
        }
    }
lab0:
    z->bra = z->c; /* ], line 328 */
    {
        int ret = slice_del(z); /* delete, line 328 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

static int r_stem_suffix_chain_before_ki(struct SN_env *z) {
    z->ket = z->c; /* [, line 333 */
    {
        int ret = r_mark_ki(z);
        if (ret == 0)
            return 0; /* call mark_ki, line 334 */
        if (ret < 0)
            return ret;
    }
    {
        int m1 = z->l - z->c;
        (void)m1; /* or, line 342 */
        {
            int ret = r_mark_DA(z);
            if (ret == 0)
                goto lab1; /* call mark_DA, line 336 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 336 */
        {
            int ret = slice_del(z); /* delete, line 336 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 336 */
            z->ket = z->c;                                /* [, line 336 */
            {
                int m2 = z->l - z->c;
                (void)m2; /* or, line 338 */
                {
                    int ret = r_mark_lAr(z);
                    if (ret == 0)
                        goto lab4; /* call mark_lAr, line 337 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 337 */
                {
                    int ret = slice_del(z); /* delete, line 337 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 337 */
                    {
                        int ret = r_stem_suffix_chain_before_ki(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab5;
                        } /* call stem_suffix_chain_before_ki, line 337 */
                        if (ret < 0)
                            return ret;
                    }
                lab5:;
                }
                goto lab3;
            lab4:
                z->c = z->l - m2;
                {
                    int ret = r_mark_possessives(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab2;
                    } /* call mark_possessives, line 339 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 339 */
                {
                    int ret = slice_del(z); /* delete, line 339 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 339 */
                    z->ket = z->c;                                /* [, line 339 */
                    {
                        int ret = r_mark_lAr(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab6;
                        } /* call mark_lAr, line 339 */
                        if (ret < 0)
                            return ret;
                    }
                    z->bra = z->c; /* ], line 339 */
                    {
                        int ret = slice_del(z); /* delete, line 339 */
                        if (ret < 0)
                            return ret;
                    }
                    {
                        int ret = r_stem_suffix_chain_before_ki(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab6;
                        } /* call stem_suffix_chain_before_ki, line 339 */
                        if (ret < 0)
                            return ret;
                    }
                lab6:;
                }
            }
        lab3:
        lab2:;
        }
        goto lab0;
    lab1:
        z->c = z->l - m1;
        {
            int ret = r_mark_nUn(z);
            if (ret == 0)
                goto lab7; /* call mark_nUn, line 343 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 343 */
        {
            int ret = slice_del(z); /* delete, line 343 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 343 */
            z->ket = z->c;                                /* [, line 343 */
            {
                int m3 = z->l - z->c;
                (void)m3; /* or, line 345 */
                {
                    int ret = r_mark_lArI(z);
                    if (ret == 0)
                        goto lab10; /* call mark_lArI, line 344 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 344 */
                {
                    int ret = slice_del(z); /* delete, line 344 */
                    if (ret < 0)
                        return ret;
                }
                goto lab9;
            lab10:
                z->c = z->l - m3;
                z->ket = z->c; /* [, line 346 */
                {
                    int m4 = z->l - z->c;
                    (void)m4; /* or, line 346 */
                    {
                        int ret = r_mark_possessives(z);
                        if (ret == 0)
                            goto lab13; /* call mark_possessives, line 346 */
                        if (ret < 0)
                            return ret;
                    }
                    goto lab12;
                lab13:
                    z->c = z->l - m4;
                    {
                        int ret = r_mark_sU(z);
                        if (ret == 0)
                            goto lab11; /* call mark_sU, line 346 */
                        if (ret < 0)
                            return ret;
                    }
                }
            lab12:
                z->bra = z->c; /* ], line 346 */
                {
                    int ret = slice_del(z); /* delete, line 346 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 346 */
                    z->ket = z->c;                                /* [, line 346 */
                    {
                        int ret = r_mark_lAr(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab14;
                        } /* call mark_lAr, line 346 */
                        if (ret < 0)
                            return ret;
                    }
                    z->bra = z->c; /* ], line 346 */
                    {
                        int ret = slice_del(z); /* delete, line 346 */
                        if (ret < 0)
                            return ret;
                    }
                    {
                        int ret = r_stem_suffix_chain_before_ki(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab14;
                        } /* call stem_suffix_chain_before_ki, line 346 */
                        if (ret < 0)
                            return ret;
                    }
                lab14:;
                }
                goto lab9;
            lab11:
                z->c = z->l - m3;
                {
                    int ret = r_stem_suffix_chain_before_ki(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab8;
                    } /* call stem_suffix_chain_before_ki, line 348 */
                    if (ret < 0)
                        return ret;
                }
            }
        lab9:
        lab8:;
        }
        goto lab0;
    lab7:
        z->c = z->l - m1;
        {
            int ret = r_mark_ndA(z);
            if (ret == 0)
                return 0; /* call mark_ndA, line 351 */
            if (ret < 0)
                return ret;
        }
        {
            int m5 = z->l - z->c;
            (void)m5; /* or, line 353 */
            {
                int ret = r_mark_lArI(z);
                if (ret == 0)
                    goto lab16; /* call mark_lArI, line 352 */
                if (ret < 0)
                    return ret;
            }
            z->bra = z->c; /* ], line 352 */
            {
                int ret = slice_del(z); /* delete, line 352 */
                if (ret < 0)
                    return ret;
            }
            goto lab15;
        lab16:
            z->c = z->l - m5;
            {
                int ret = r_mark_sU(z);
                if (ret == 0)
                    goto lab17; /* call mark_sU, line 354 */
                if (ret < 0)
                    return ret;
            }
            z->bra = z->c; /* ], line 354 */
            {
                int ret = slice_del(z); /* delete, line 354 */
                if (ret < 0)
                    return ret;
            }
            {
                int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 354 */
                z->ket = z->c;                                /* [, line 354 */
                {
                    int ret = r_mark_lAr(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab18;
                    } /* call mark_lAr, line 354 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 354 */
                {
                    int ret = slice_del(z); /* delete, line 354 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int ret = r_stem_suffix_chain_before_ki(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab18;
                    } /* call stem_suffix_chain_before_ki, line 354 */
                    if (ret < 0)
                        return ret;
                }
            lab18:;
            }
            goto lab15;
        lab17:
            z->c = z->l - m5;
            {
                int ret = r_stem_suffix_chain_before_ki(z);
                if (ret == 0)
                    return 0; /* call stem_suffix_chain_before_ki, line 356 */
                if (ret < 0)
                    return ret;
            }
        }
    lab15:;
    }
lab0:
    return 1;
}

static int r_stem_noun_suffixes(struct SN_env *z) {
    {
        int m1 = z->l - z->c;
        (void)m1;      /* or, line 363 */
        z->ket = z->c; /* [, line 362 */
        {
            int ret = r_mark_lAr(z);
            if (ret == 0)
                goto lab1; /* call mark_lAr, line 362 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 362 */
        {
            int ret = slice_del(z); /* delete, line 362 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 362 */
            {
                int ret = r_stem_suffix_chain_before_ki(z);
                if (ret == 0) {
                    z->c = z->l - m_keep;
                    goto lab2;
                } /* call stem_suffix_chain_before_ki, line 362 */
                if (ret < 0)
                    return ret;
            }
        lab2:;
        }
        goto lab0;
    lab1:
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 364 */
        {
            int ret = r_mark_ncA(z);
            if (ret == 0)
                goto lab3; /* call mark_ncA, line 364 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 364 */
        {
            int ret = slice_del(z); /* delete, line 364 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 365 */
            {
                int m2 = z->l - z->c;
                (void)m2;      /* or, line 367 */
                z->ket = z->c; /* [, line 366 */
                {
                    int ret = r_mark_lArI(z);
                    if (ret == 0)
                        goto lab6; /* call mark_lArI, line 366 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 366 */
                {
                    int ret = slice_del(z); /* delete, line 366 */
                    if (ret < 0)
                        return ret;
                }
                goto lab5;
            lab6:
                z->c = z->l - m2;
                z->ket = z->c; /* [, line 368 */
                {
                    int m3 = z->l - z->c;
                    (void)m3; /* or, line 368 */
                    {
                        int ret = r_mark_possessives(z);
                        if (ret == 0)
                            goto lab9; /* call mark_possessives, line 368 */
                        if (ret < 0)
                            return ret;
                    }
                    goto lab8;
                lab9:
                    z->c = z->l - m3;
                    {
                        int ret = r_mark_sU(z);
                        if (ret == 0)
                            goto lab7; /* call mark_sU, line 368 */
                        if (ret < 0)
                            return ret;
                    }
                }
            lab8:
                z->bra = z->c; /* ], line 368 */
                {
                    int ret = slice_del(z); /* delete, line 368 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 368 */
                    z->ket = z->c;                                /* [, line 368 */
                    {
                        int ret = r_mark_lAr(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab10;
                        } /* call mark_lAr, line 368 */
                        if (ret < 0)
                            return ret;
                    }
                    z->bra = z->c; /* ], line 368 */
                    {
                        int ret = slice_del(z); /* delete, line 368 */
                        if (ret < 0)
                            return ret;
                    }
                    {
                        int ret = r_stem_suffix_chain_before_ki(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab10;
                        } /* call stem_suffix_chain_before_ki, line 368 */
                        if (ret < 0)
                            return ret;
                    }
                lab10:;
                }
                goto lab5;
            lab7:
                z->c = z->l - m2;
                z->ket = z->c; /* [, line 370 */
                {
                    int ret = r_mark_lAr(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab4;
                    } /* call mark_lAr, line 370 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 370 */
                {
                    int ret = slice_del(z); /* delete, line 370 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int ret = r_stem_suffix_chain_before_ki(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab4;
                    } /* call stem_suffix_chain_before_ki, line 370 */
                    if (ret < 0)
                        return ret;
                }
            }
        lab5:
        lab4:;
        }
        goto lab0;
    lab3:
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 374 */
        {
            int m4 = z->l - z->c;
            (void)m4; /* or, line 374 */
            {
                int ret = r_mark_ndA(z);
                if (ret == 0)
                    goto lab13; /* call mark_ndA, line 374 */
                if (ret < 0)
                    return ret;
            }
            goto lab12;
        lab13:
            z->c = z->l - m4;
            {
                int ret = r_mark_nA(z);
                if (ret == 0)
                    goto lab11; /* call mark_nA, line 374 */
                if (ret < 0)
                    return ret;
            }
        }
    lab12: {
        int m5 = z->l - z->c;
        (void)m5; /* or, line 377 */
        {
            int ret = r_mark_lArI(z);
            if (ret == 0)
                goto lab15; /* call mark_lArI, line 376 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 376 */
        {
            int ret = slice_del(z); /* delete, line 376 */
            if (ret < 0)
                return ret;
        }
        goto lab14;
    lab15:
        z->c = z->l - m5;
        {
            int ret = r_mark_sU(z);
            if (ret == 0)
                goto lab16; /* call mark_sU, line 378 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 378 */
        {
            int ret = slice_del(z); /* delete, line 378 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 378 */
            z->ket = z->c;                                /* [, line 378 */
            {
                int ret = r_mark_lAr(z);
                if (ret == 0) {
                    z->c = z->l - m_keep;
                    goto lab17;
                } /* call mark_lAr, line 378 */
                if (ret < 0)
                    return ret;
            }
            z->bra = z->c; /* ], line 378 */
            {
                int ret = slice_del(z); /* delete, line 378 */
                if (ret < 0)
                    return ret;
            }
            {
                int ret = r_stem_suffix_chain_before_ki(z);
                if (ret == 0) {
                    z->c = z->l - m_keep;
                    goto lab17;
                } /* call stem_suffix_chain_before_ki, line 378 */
                if (ret < 0)
                    return ret;
            }
        lab17:;
        }
        goto lab14;
    lab16:
        z->c = z->l - m5;
        {
            int ret = r_stem_suffix_chain_before_ki(z);
            if (ret == 0)
                goto lab11; /* call stem_suffix_chain_before_ki, line 380 */
            if (ret < 0)
                return ret;
        }
    }
    lab14:
        goto lab0;
    lab11:
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 384 */
        {
            int m6 = z->l - z->c;
            (void)m6; /* or, line 384 */
            {
                int ret = r_mark_ndAn(z);
                if (ret == 0)
                    goto lab20; /* call mark_ndAn, line 384 */
                if (ret < 0)
                    return ret;
            }
            goto lab19;
        lab20:
            z->c = z->l - m6;
            {
                int ret = r_mark_nU(z);
                if (ret == 0)
                    goto lab18; /* call mark_nU, line 384 */
                if (ret < 0)
                    return ret;
            }
        }
    lab19: {
        int m7 = z->l - z->c;
        (void)m7; /* or, line 384 */
        {
            int ret = r_mark_sU(z);
            if (ret == 0)
                goto lab22; /* call mark_sU, line 384 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 384 */
        {
            int ret = slice_del(z); /* delete, line 384 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 384 */
            z->ket = z->c;                                /* [, line 384 */
            {
                int ret = r_mark_lAr(z);
                if (ret == 0) {
                    z->c = z->l - m_keep;
                    goto lab23;
                } /* call mark_lAr, line 384 */
                if (ret < 0)
                    return ret;
            }
            z->bra = z->c; /* ], line 384 */
            {
                int ret = slice_del(z); /* delete, line 384 */
                if (ret < 0)
                    return ret;
            }
            {
                int ret = r_stem_suffix_chain_before_ki(z);
                if (ret == 0) {
                    z->c = z->l - m_keep;
                    goto lab23;
                } /* call stem_suffix_chain_before_ki, line 384 */
                if (ret < 0)
                    return ret;
            }
        lab23:;
        }
        goto lab21;
    lab22:
        z->c = z->l - m7;
        {
            int ret = r_mark_lArI(z);
            if (ret == 0)
                goto lab18; /* call mark_lArI, line 384 */
            if (ret < 0)
                return ret;
        }
    }
    lab21:
        goto lab0;
    lab18:
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 386 */
        {
            int ret = r_mark_DAn(z);
            if (ret == 0)
                goto lab24; /* call mark_DAn, line 386 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 386 */
        {
            int ret = slice_del(z); /* delete, line 386 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 386 */
            z->ket = z->c;                                /* [, line 386 */
            {
                int m8 = z->l - z->c;
                (void)m8; /* or, line 389 */
                {
                    int ret = r_mark_possessives(z);
                    if (ret == 0)
                        goto lab27; /* call mark_possessives, line 388 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 388 */
                {
                    int ret = slice_del(z); /* delete, line 388 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 388 */
                    z->ket = z->c;                                /* [, line 388 */
                    {
                        int ret = r_mark_lAr(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab28;
                        } /* call mark_lAr, line 388 */
                        if (ret < 0)
                            return ret;
                    }
                    z->bra = z->c; /* ], line 388 */
                    {
                        int ret = slice_del(z); /* delete, line 388 */
                        if (ret < 0)
                            return ret;
                    }
                    {
                        int ret = r_stem_suffix_chain_before_ki(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab28;
                        } /* call stem_suffix_chain_before_ki, line 388 */
                        if (ret < 0)
                            return ret;
                    }
                lab28:;
                }
                goto lab26;
            lab27:
                z->c = z->l - m8;
                {
                    int ret = r_mark_lAr(z);
                    if (ret == 0)
                        goto lab29; /* call mark_lAr, line 390 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 390 */
                {
                    int ret = slice_del(z); /* delete, line 390 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 390 */
                    {
                        int ret = r_stem_suffix_chain_before_ki(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab30;
                        } /* call stem_suffix_chain_before_ki, line 390 */
                        if (ret < 0)
                            return ret;
                    }
                lab30:;
                }
                goto lab26;
            lab29:
                z->c = z->l - m8;
                {
                    int ret = r_stem_suffix_chain_before_ki(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab25;
                    } /* call stem_suffix_chain_before_ki, line 392 */
                    if (ret < 0)
                        return ret;
                }
            }
        lab26:
        lab25:;
        }
        goto lab0;
    lab24:
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 396 */
        {
            int m9 = z->l - z->c;
            (void)m9; /* or, line 396 */
            {
                int ret = r_mark_nUn(z);
                if (ret == 0)
                    goto lab33; /* call mark_nUn, line 396 */
                if (ret < 0)
                    return ret;
            }
            goto lab32;
        lab33:
            z->c = z->l - m9;
            {
                int ret = r_mark_ylA(z);
                if (ret == 0)
                    goto lab31; /* call mark_ylA, line 396 */
                if (ret < 0)
                    return ret;
            }
        }
    lab32:
        z->bra = z->c; /* ], line 396 */
        {
            int ret = slice_del(z); /* delete, line 396 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 397 */
            {
                int m10 = z->l - z->c;
                (void)m10;     /* or, line 399 */
                z->ket = z->c; /* [, line 398 */
                {
                    int ret = r_mark_lAr(z);
                    if (ret == 0)
                        goto lab36; /* call mark_lAr, line 398 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 398 */
                {
                    int ret = slice_del(z); /* delete, line 398 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int ret = r_stem_suffix_chain_before_ki(z);
                    if (ret == 0)
                        goto lab36; /* call stem_suffix_chain_before_ki, line 398 */
                    if (ret < 0)
                        return ret;
                }
                goto lab35;
            lab36:
                z->c = z->l - m10;
                z->ket = z->c; /* [, line 400 */
                {
                    int m11 = z->l - z->c;
                    (void)m11; /* or, line 400 */
                    {
                        int ret = r_mark_possessives(z);
                        if (ret == 0)
                            goto lab39; /* call mark_possessives, line 400 */
                        if (ret < 0)
                            return ret;
                    }
                    goto lab38;
                lab39:
                    z->c = z->l - m11;
                    {
                        int ret = r_mark_sU(z);
                        if (ret == 0)
                            goto lab37; /* call mark_sU, line 400 */
                        if (ret < 0)
                            return ret;
                    }
                }
            lab38:
                z->bra = z->c; /* ], line 400 */
                {
                    int ret = slice_del(z); /* delete, line 400 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 400 */
                    z->ket = z->c;                                /* [, line 400 */
                    {
                        int ret = r_mark_lAr(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab40;
                        } /* call mark_lAr, line 400 */
                        if (ret < 0)
                            return ret;
                    }
                    z->bra = z->c; /* ], line 400 */
                    {
                        int ret = slice_del(z); /* delete, line 400 */
                        if (ret < 0)
                            return ret;
                    }
                    {
                        int ret = r_stem_suffix_chain_before_ki(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab40;
                        } /* call stem_suffix_chain_before_ki, line 400 */
                        if (ret < 0)
                            return ret;
                    }
                lab40:;
                }
                goto lab35;
            lab37:
                z->c = z->l - m10;
                {
                    int ret = r_stem_suffix_chain_before_ki(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab34;
                    } /* call stem_suffix_chain_before_ki, line 402 */
                    if (ret < 0)
                        return ret;
                }
            }
        lab35:
        lab34:;
        }
        goto lab0;
    lab31:
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 406 */
        {
            int ret = r_mark_lArI(z);
            if (ret == 0)
                goto lab41; /* call mark_lArI, line 406 */
            if (ret < 0)
                return ret;
        }
        z->bra = z->c; /* ], line 406 */
        {
            int ret = slice_del(z); /* delete, line 406 */
            if (ret < 0)
                return ret;
        }
        goto lab0;
    lab41:
        z->c = z->l - m1;
        {
            int ret = r_stem_suffix_chain_before_ki(z);
            if (ret == 0)
                goto lab42; /* call stem_suffix_chain_before_ki, line 408 */
            if (ret < 0)
                return ret;
        }
        goto lab0;
    lab42:
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 410 */
        {
            int m12 = z->l - z->c;
            (void)m12; /* or, line 410 */
            {
                int ret = r_mark_DA(z);
                if (ret == 0)
                    goto lab45; /* call mark_DA, line 410 */
                if (ret < 0)
                    return ret;
            }
            goto lab44;
        lab45:
            z->c = z->l - m12;
            {
                int ret = r_mark_yU(z);
                if (ret == 0)
                    goto lab46; /* call mark_yU, line 410 */
                if (ret < 0)
                    return ret;
            }
            goto lab44;
        lab46:
            z->c = z->l - m12;
            {
                int ret = r_mark_yA(z);
                if (ret == 0)
                    goto lab43; /* call mark_yA, line 410 */
                if (ret < 0)
                    return ret;
            }
        }
    lab44:
        z->bra = z->c; /* ], line 410 */
        {
            int ret = slice_del(z); /* delete, line 410 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 410 */
            z->ket = z->c;                                /* [, line 410 */
            {
                int m13 = z->l - z->c;
                (void)m13; /* or, line 410 */
                {
                    int ret = r_mark_possessives(z);
                    if (ret == 0)
                        goto lab49; /* call mark_possessives, line 410 */
                    if (ret < 0)
                        return ret;
                }
                z->bra = z->c; /* ], line 410 */
                {
                    int ret = slice_del(z); /* delete, line 410 */
                    if (ret < 0)
                        return ret;
                }
                {
                    int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 410 */
                    z->ket = z->c;                                /* [, line 410 */
                    {
                        int ret = r_mark_lAr(z);
                        if (ret == 0) {
                            z->c = z->l - m_keep;
                            goto lab50;
                        } /* call mark_lAr, line 410 */
                        if (ret < 0)
                            return ret;
                    }
                lab50:;
                }
                goto lab48;
            lab49:
                z->c = z->l - m13;
                {
                    int ret = r_mark_lAr(z);
                    if (ret == 0) {
                        z->c = z->l - m_keep;
                        goto lab47;
                    } /* call mark_lAr, line 410 */
                    if (ret < 0)
                        return ret;
                }
            }
        lab48:
            z->bra = z->c; /* ], line 410 */
            {
                int ret = slice_del(z); /* delete, line 410 */
                if (ret < 0)
                    return ret;
            }
            z->ket = z->c; /* [, line 410 */
            {
                int ret = r_stem_suffix_chain_before_ki(z);
                if (ret == 0) {
                    z->c = z->l - m_keep;
                    goto lab47;
                } /* call stem_suffix_chain_before_ki, line 410 */
                if (ret < 0)
                    return ret;
            }
        lab47:;
        }
        goto lab0;
    lab43:
        z->c = z->l - m1;
        z->ket = z->c; /* [, line 412 */
        {
            int m14 = z->l - z->c;
            (void)m14; /* or, line 412 */
            {
                int ret = r_mark_possessives(z);
                if (ret == 0)
                    goto lab52; /* call mark_possessives, line 412 */
                if (ret < 0)
                    return ret;
            }
            goto lab51;
        lab52:
            z->c = z->l - m14;
            {
                int ret = r_mark_sU(z);
                if (ret == 0)
                    return 0; /* call mark_sU, line 412 */
                if (ret < 0)
                    return ret;
            }
        }
    lab51:
        z->bra = z->c; /* ], line 412 */
        {
            int ret = slice_del(z); /* delete, line 412 */
            if (ret < 0)
                return ret;
        }
        {
            int m_keep = z->l - z->c; /* (void) m_keep;*/ /* try, line 412 */
            z->ket = z->c;                                /* [, line 412 */
            {
                int ret = r_mark_lAr(z);
                if (ret == 0) {
                    z->c = z->l - m_keep;
                    goto lab53;
                } /* call mark_lAr, line 412 */
                if (ret < 0)
                    return ret;
            }
            z->bra = z->c; /* ], line 412 */
            {
                int ret = slice_del(z); /* delete, line 412 */
                if (ret < 0)
                    return ret;
            }
            {
                int ret = r_stem_suffix_chain_before_ki(z);
                if (ret == 0) {
                    z->c = z->l - m_keep;
                    goto lab53;
                } /* call stem_suffix_chain_before_ki, line 412 */
                if (ret < 0)
                    return ret;
            }
        lab53:;
        }
    }
lab0:
    return 1;
}

static int r_post_process_last_consonants(struct SN_env *z) {
    int among_var;
    z->ket = z->c;                        /* [, line 416 */
    among_var = find_among_b(z, a_23, 4); /* substring, line 416 */
    if (!(among_var))
        return 0;
    z->bra = z->c; /* ], line 416 */
    switch (among_var) {
        case 0:
            return 0;
        case 1: {
            int ret = slice_from_s(z, 1, s_16); /* <-, line 417 */
            if (ret < 0)
                return ret;
        } break;
        case 2: {
            int ret = slice_from_s(z, 2, s_17); /* <-, line 418 */
            if (ret < 0)
                return ret;
        } break;
        case 3: {
            int ret = slice_from_s(z, 1, s_18); /* <-, line 419 */
            if (ret < 0)
                return ret;
        } break;
        case 4: {
            int ret = slice_from_s(z, 1, s_19); /* <-, line 420 */
            if (ret < 0)
                return ret;
        } break;
    }
    return 1;
}

static int r_append_U_to_stems_ending_with_d_or_g(struct SN_env *z) {
    {
        int m_test = z->l - z->c; /* test, line 431 */
        {
            int m1 = z->l - z->c;
            (void)m1; /* or, line 431 */
            if (!(eq_s_b(z, 1, s_20)))
                goto lab1;
            goto lab0;
        lab1:
            z->c = z->l - m1;
            if (!(eq_s_b(z, 1, s_21)))
                return 0;
        }
    lab0:
        z->c = z->l - m_test;
    }
    {
        int m2 = z->l - z->c;
        (void)m2; /* or, line 433 */
        {
            int m_test = z->l - z->c; /* test, line 432 */
            if (out_grouping_b_U(z, g_vowel, 97, 305, 1) < 0)
                goto lab3; /* goto */ /* grouping vowel, line 432 */
            {
                int m3 = z->l - z->c;
                (void)m3; /* or, line 432 */
                if (!(eq_s_b(z, 1, s_22)))
                    goto lab5;
                goto lab4;
            lab5:
                z->c = z->l - m3;
                if (!(eq_s_b(z, 2, s_23)))
                    goto lab3;
            }
        lab4:
            z->c = z->l - m_test;
        }
        {
            int c_keep = z->c;
            int ret = insert_s(z, z->c, z->c, 2, s_24); /* <+, line 432 */
            z->c = c_keep;
            if (ret < 0)
                return ret;
        }
        goto lab2;
    lab3:
        z->c = z->l - m2;
        {
            int m_test = z->l - z->c; /* test, line 434 */
            if (out_grouping_b_U(z, g_vowel, 97, 305, 1) < 0)
                goto lab6; /* goto */ /* grouping vowel, line 434 */
            {
                int m4 = z->l - z->c;
                (void)m4; /* or, line 434 */
                if (!(eq_s_b(z, 1, s_25)))
                    goto lab8;
                goto lab7;
            lab8:
                z->c = z->l - m4;
                if (!(eq_s_b(z, 1, s_26)))
                    goto lab6;
            }
        lab7:
            z->c = z->l - m_test;
        }
        {
            int c_keep = z->c;
            int ret = insert_s(z, z->c, z->c, 1, s_27); /* <+, line 434 */
            z->c = c_keep;
            if (ret < 0)
                return ret;
        }
        goto lab2;
    lab6:
        z->c = z->l - m2;
        {
            int m_test = z->l - z->c; /* test, line 436 */
            if (out_grouping_b_U(z, g_vowel, 97, 305, 1) < 0)
                goto lab9; /* goto */ /* grouping vowel, line 436 */
            {
                int m5 = z->l - z->c;
                (void)m5; /* or, line 436 */
                if (!(eq_s_b(z, 1, s_28)))
                    goto lab11;
                goto lab10;
            lab11:
                z->c = z->l - m5;
                if (!(eq_s_b(z, 1, s_29)))
                    goto lab9;
            }
        lab10:
            z->c = z->l - m_test;
        }
        {
            int c_keep = z->c;
            int ret = insert_s(z, z->c, z->c, 1, s_30); /* <+, line 436 */
            z->c = c_keep;
            if (ret < 0)
                return ret;
        }
        goto lab2;
    lab9:
        z->c = z->l - m2;
        {
            int m_test = z->l - z->c; /* test, line 438 */
            if (out_grouping_b_U(z, g_vowel, 97, 305, 1) < 0)
                return 0; /* goto */ /* grouping vowel, line 438 */
            {
                int m6 = z->l - z->c;
                (void)m6; /* or, line 438 */
                if (!(eq_s_b(z, 2, s_31)))
                    goto lab13;
                goto lab12;
            lab13:
                z->c = z->l - m6;
                if (!(eq_s_b(z, 2, s_32)))
                    return 0;
            }
        lab12:
            z->c = z->l - m_test;
        }
        {
            int c_keep = z->c;
            int ret = insert_s(z, z->c, z->c, 2, s_33); /* <+, line 438 */
            z->c = c_keep;
            if (ret < 0)
                return ret;
        }
    }
lab2:
    return 1;
}

static int r_more_than_one_syllable_word(struct SN_env *z) {
    {
        int c_test = z->c; /* test, line 446 */
        {
            int i = 2;
            while (1) { /* atleast, line 446 */
                int c1 = z->c;
                { /* gopast */ /* grouping vowel, line 446 */
                    int ret = out_grouping_U(z, g_vowel, 97, 305, 1);
                    if (ret < 0)
                        goto lab0;
                    z->c += ret;
                }
                i--;
                continue;
            lab0:
                z->c = c1;
                break;
            }
            if (i > 0)
                return 0;
        }
        z->c = c_test;
    }
    return 1;
}

static int r_is_reserved_word(struct SN_env *z) {
    {
        int c1 = z->c; /* or, line 451 */
        {
            int c_test = z->c; /* test, line 450 */
            while (1) {        /* gopast, line 450 */
                if (!(eq_s(z, 2, s_34)))
                    goto lab2;
                break;
            lab2: {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    goto lab1;
                z->c = ret; /* gopast, line 450 */
            }
            }
            z->I[0] = 2;
            if (!(z->I[0] == z->l))
                goto lab1;
            z->c = c_test;
        }
        goto lab0;
    lab1:
        z->c = c1;
        {
            int c_test = z->c; /* test, line 452 */
            while (1) {        /* gopast, line 452 */
                if (!(eq_s(z, 5, s_35)))
                    goto lab3;
                break;
            lab3: {
                int ret = skip_utf8(z->p, z->c, 0, z->l, 1);
                if (ret < 0)
                    return 0;
                z->c = ret; /* gopast, line 452 */
            }
            }
            z->I[0] = 5;
            if (!(z->I[0] == z->l))
                return 0;
            z->c = c_test;
        }
    }
lab0:
    return 1;
}

static int r_postlude(struct SN_env *z) {
    {
        int c1 = z->c; /* not, line 456 */
        {
            int ret = r_is_reserved_word(z);
            if (ret == 0)
                goto lab0; /* call is_reserved_word, line 456 */
            if (ret < 0)
                return ret;
        }
        return 0;
    lab0:
        z->c = c1;
    }
    z->lb = z->c;
    z->c = z->l; /* backwards, line 457 */

    {
        int m2 = z->l - z->c;
        (void)m2; /* do, line 458 */
        {
            int ret = r_append_U_to_stems_ending_with_d_or_g(z);
            if (ret == 0)
                goto lab1; /* call append_U_to_stems_ending_with_d_or_g, line 458 */
            if (ret < 0)
                return ret;
        }
    lab1:
        z->c = z->l - m2;
    }
    {
        int m3 = z->l - z->c;
        (void)m3; /* do, line 459 */
        {
            int ret = r_post_process_last_consonants(z);
            if (ret == 0)
                goto lab2; /* call post_process_last_consonants, line 459 */
            if (ret < 0)
                return ret;
        }
    lab2:
        z->c = z->l - m3;
    }
    z->c = z->lb;
    return 1;
}

extern int turkish_UTF_8_stem(struct SN_env *z) {
    {
        int ret = r_more_than_one_syllable_word(z);
        if (ret == 0)
            return 0; /* call more_than_one_syllable_word, line 465 */
        if (ret < 0)
            return ret;
    }
    z->lb = z->c;
    z->c = z->l; /* backwards, line 467 */

    {
        int m1 = z->l - z->c;
        (void)m1; /* do, line 468 */
        {
            int ret = r_stem_nominal_verb_suffixes(z);
            if (ret == 0)
                goto lab0; /* call stem_nominal_verb_suffixes, line 468 */
            if (ret < 0)
                return ret;
        }
    lab0:
        z->c = z->l - m1;
    }
    if (!(z->B[0]))
        return 0; /* Boolean test continue_stemming_noun_suffixes, line 469 */
    {
        int m2 = z->l - z->c;
        (void)m2; /* do, line 470 */
        {
            int ret = r_stem_noun_suffixes(z);
            if (ret == 0)
                goto lab1; /* call stem_noun_suffixes, line 470 */
            if (ret < 0)
                return ret;
        }
    lab1:
        z->c = z->l - m2;
    }
    z->c = z->lb;
    {
        int ret = r_postlude(z);
        if (ret == 0)
            return 0; /* call postlude, line 473 */
        if (ret < 0)
            return ret;
    }
    return 1;
}

extern struct SN_env *turkish_UTF_8_create_env(void) { return SN_create_env(0, 1, 1); }

extern void turkish_UTF_8_close_env(struct SN_env *z) { SN_close_env(z, 0); }
