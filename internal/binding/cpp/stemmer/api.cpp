// Copyright(C) 2023 InfiniFlow, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#include "header.h"

#include <stdlib.h> /* for calloc, free */

extern struct SN_env *SN_create_env(int S_size, int I_size, int B_size) {
    struct SN_env *z = (struct SN_env *)calloc(1, sizeof(struct SN_env));
    if (z == NULL)
        return NULL;
    z->p = create_s();
    if (z->p == NULL)
        goto error;
    if (S_size) {
        int i;
        z->S = (symbol **)calloc(S_size, sizeof(symbol *));
        if (z->S == NULL)
            goto error;

        for (i = 0; i < S_size; i++) {
            z->S[i] = create_s();
            if (z->S[i] == NULL)
                goto error;
        }
    }

    if (I_size) {
        z->I = (int *)calloc(I_size, sizeof(int));
        if (z->I == NULL)
            goto error;
    }

    if (B_size) {
        z->B = (unsigned char *)calloc(B_size, sizeof(unsigned char));
        if (z->B == NULL)
            goto error;
    }

    return z;
error:
    SN_close_env(z, S_size);
    return NULL;
}

extern void SN_close_env(struct SN_env *z, int S_size) {
    if (z == NULL)
        return;
    if (S_size) {
        int i;
        for (i = 0; i < S_size; i++) {
            lose_s(z->S[i]);
        }
        free(z->S);
    }
    free(z->I);
    free(z->B);
    if (z->p)
        lose_s(z->p);
    free(z);
}

extern int SN_set_current(struct SN_env *z, int size, const symbol *s) {
    int err = replace_s(z, 0, z->l, size, s, NULL);
    z->c = 0;
    return err;
}
