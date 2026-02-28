// Copyright 2023 The RE2 Authors.  All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "re2/bitmap256.h"

#include <stdint.h>

#include "util/logging.h"
#include "util/util.h"

namespace re2 {

int Bitmap256::FindNextSetBit(int c) const {
    DCHECK_GE(c, 0);
    DCHECK_LE(c, 255);

    // Check the word that contains the bit. Mask out any lower bits.
    int i = c / 64;
    uint64_t word = words_[i] & (~uint64_t{0} << (c % 64));
    if (word != 0)
        return (i * 64) + FindLSBSet(word);

    // Check any following words.
    i++;
    switch (i) {
        case 1:
            if (words_[1] != 0)
                return (1 * 64) + FindLSBSet(words_[1]);
            FALLTHROUGH_INTENDED;
        case 2:
            if (words_[2] != 0)
                return (2 * 64) + FindLSBSet(words_[2]);
            FALLTHROUGH_INTENDED;
        case 3:
            if (words_[3] != 0)
                return (3 * 64) + FindLSBSet(words_[3]);
            FALLTHROUGH_INTENDED;
        default:
            return -1;
    }
}

} // namespace re2
