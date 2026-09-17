
/* This file was generated automatically by the Snowball to ANSI C compiler */

#pragma once

#ifdef __cplusplus
extern "C" {
#endif

extern struct SN_env *english_UTF_8_create_env(void);
extern void english_UTF_8_close_env(struct SN_env *z);

extern int english_UTF_8_stem(struct SN_env *z);

#ifdef __cplusplus
}
#endif
