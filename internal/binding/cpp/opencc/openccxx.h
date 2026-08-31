#pragma once

#include "opencc_types.h"
#include <string>

class OpenCC {
public:
    OpenCC(const std::string &home_dir);
    virtual ~OpenCC();

    int open(const char *config_file, const char *home_dir);

    long convert(const std::string &in, std::string &out, long length = -1);

    long convert(const std::wstring &in, std::wstring &out, long length = -1);

private:
    char *config_file;
    opencc_t od;
};
