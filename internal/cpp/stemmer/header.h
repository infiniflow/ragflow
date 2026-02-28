
#pragma once

#include <limits.h>

#include "api.h"

#define MAXINT INT_MAX
#define MININT INT_MIN

#define HEAD 2 * sizeof(int)

#define SIZE(p) ((int *)(p))[-1]
#define SET_SIZE(p, n) ((int *)(p))[-1] = n
#define CAPACITY(p) ((int *)(p))[-2]

struct among {
    int s_size;      /* number of chars in string */
    const symbol *s; /* search string */
    int substring_i; /* index to longest matching substring */
    int result;      /* result of the lookup */
    int (*function)(struct SN_env *);
};

extern symbol *create_s(void);
extern void lose_s(symbol *p);

extern int skip_utf8(const symbol *p, int c, int lb, int l, int n);

extern int in_grouping_U(struct SN_env *z, const unsigned char *s, int min, int max, int repeat);
extern int in_grouping_b_U(struct SN_env *z, const unsigned char *s, int min, int max, int repeat);
extern int out_grouping_U(struct SN_env *z, const unsigned char *s, int min, int max, int repeat);
extern int out_grouping_b_U(struct SN_env *z, const unsigned char *s, int min, int max, int repeat);

extern int in_grouping(struct SN_env *z, const unsigned char *s, int min, int max, int repeat);
extern int in_grouping_b(struct SN_env *z, const unsigned char *s, int min, int max, int repeat);
extern int out_grouping(struct SN_env *z, const unsigned char *s, int min, int max, int repeat);
extern int out_grouping_b(struct SN_env *z, const unsigned char *s, int min, int max, int repeat);

extern int eq_s(struct SN_env *z, int s_size, const symbol *s);
extern int eq_s_b(struct SN_env *z, int s_size, const symbol *s);
extern int eq_v(struct SN_env *z, const symbol *p);
extern int eq_v_b(struct SN_env *z, const symbol *p);

extern int find_among(struct SN_env *z, const struct among *v, int v_size);
extern int find_among_b(struct SN_env *z, const struct among *v, int v_size);

extern int replace_s(struct SN_env *z, int c_bra, int c_ket, int s_size, const symbol *s, int *adjustment);
extern int slice_from_s(struct SN_env *z, int s_size, const symbol *s);
extern int slice_from_v(struct SN_env *z, const symbol *p);
extern int slice_del(struct SN_env *z);

extern int insert_s(struct SN_env *z, int bra, int ket, int s_size, const symbol *s);
extern int insert_v(struct SN_env *z, int bra, int ket, const symbol *p);

extern symbol *slice_to(struct SN_env *z, symbol *p);
extern symbol *assign_to(struct SN_env *z, symbol *p);

extern void debug(struct SN_env *z, int number, int line_count);
