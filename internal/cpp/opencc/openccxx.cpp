#include "openccxx.h"
#include "opencc.h"
#include "utils.h"

#include <iostream>
#include <string>

OpenCC::OpenCC(const std::string &home_dir) : od((opencc_t)-1) {
    config_file = mstrcpy(OPENCC_DEFAULT_CONFIG_TRAD_TO_SIMP);
    open(config_file, home_dir.c_str());
}

OpenCC::~OpenCC() {
    if (od != (opencc_t)-1)
        opencc_close(od);
    free(config_file);
}

int OpenCC::open(const char *config_file, const char *home_dir) {
    if (od != (opencc_t)-1)
        opencc_close(od);
    od = opencc_open(config_file, home_dir);
    return (od == (opencc_t)-1) ? (-1) : (0);
}

long OpenCC::convert(const std::string &in, std::string &out, long length) {
    if (od == (opencc_t)-1)
        return -1;

    if (length == -1)
        length = in.length();

    char *outbuf = opencc_convert_utf8(od, in.c_str(), length);

    if (outbuf == (char *)-1)
        return -1;

    out = outbuf;
    free(outbuf);

    return length;
}

/**
 * Warning:
 * This method can be used only if wchar_t is encoded in UCS4 on your platform.
 */
long OpenCC::convert(const std::wstring &in, std::wstring &out, long length) {
    if (od == (opencc_t)-1)
        return -1;

    size_t inbuf_left = in.length();
    if (length >= 0 && length < (long)inbuf_left)
        inbuf_left = length;

    const ucs4_t *inbuf = (const ucs4_t *)in.c_str();
    long count = 0;

    while (inbuf_left != 0) {
        size_t retval;
        size_t outbuf_left;
        ucs4_t *outbuf;

        /* occupy space */
        outbuf_left = inbuf_left + 64;
        out.resize(count + outbuf_left);
        outbuf = (ucs4_t *)out.c_str() + count;

        retval = opencc_convert(od, (ucs4_t **)&inbuf, &inbuf_left, &outbuf, &outbuf_left);
        if (retval == (size_t)-1)
            return -1;
        count += retval;
    }

    /* set the zero termination and shrink the size */
    out.resize(count + 1);
    out[count] = L'\0';

    return count;
}
