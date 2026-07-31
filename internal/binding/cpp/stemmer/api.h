
#pragma once

typedef unsigned char symbol;

/* Or replace 'char' above with 'short' for 16 bit characters.

   More precisely, replace 'char' with whatever type guarantees the
   character width you need. Note however that sizeof(symbol) should divide
   HEAD, defined in header.h as 2*sizeof(int), without remainder, otherwise
   there is an alignment problem. In the unlikely event of a problem here,
   consult Martin Porter.

*/

struct SN_env {
    symbol *p;
    int c;
    int l;
    int lb;
    int bra;
    int ket;
    symbol **S;
    int *I;
    unsigned char *B;
};

extern struct SN_env *SN_create_env(int S_size, int I_size, int B_size);
extern void SN_close_env(struct SN_env *z, int S_size);

extern int SN_set_current(struct SN_env *z, int size, const symbol *s);
