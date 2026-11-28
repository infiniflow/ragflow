/*
 * Scalar stand-in for the MSVC STL's vectorised `__std_unique_8`, needed when
 * the Windows ONNX Runtime archive is relinked into a DLL.
 *
 * The released archive was built with a newer MSVC STL than the runtime the
 * DLL links against: `std::unique` over trivially-copyable 8-byte elements
 * compiles down to this helper, which lives in the STL's runtime library and is
 * therefore absent from the archive itself. Adding it back here is what makes
 * the relink resolve; nothing else references it.
 *
 * ABI: the call site passes rcx = first, rdx = last and reads rax = new end,
 * and the semantics are `std::unique` comparing 8-byte elements bytewise.
 *
 * Compiled by tools/scripts/build_windows_native_dlls.sh with cl /MD, so it
 * uses the same dynamic CRT as the archive it is linked with.
 */
#include <string.h>

void *__std_unique_8(void *first, void *last) {
    unsigned char *out = (unsigned char *)first;
    unsigned char *end = (unsigned char *)last;
    unsigned char *cur;

    if (out == end) {
        return out;
    }
    for (cur = out + 8; cur < end; cur += 8) {
        if (memcmp(cur, out, 8) != 0) {
            out += 8;
            memcpy(out, cur, 8);
        }
    }
    return out + 8;
}
