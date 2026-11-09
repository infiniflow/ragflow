#pragma once

#include <string>
#include <string_view>

namespace thinc {

inline std::string JsonString(std::string_view value)
{
    static constexpr char hex[] = "0123456789abcdef";
    std::string result;
    result.reserve(value.size() + 2);
    result.push_back('"');
    for (unsigned char c : value)
    {
        switch (c)
        {
        case '"': result += "\\\""; break;
        case '\\': result += "\\\\"; break;
        case '\b': result += "\\b"; break;
        case '\f': result += "\\f"; break;
        case '\n': result += "\\n"; break;
        case '\r': result += "\\r"; break;
        case '\t': result += "\\t"; break;
        default:
            if (c < 0x20)
            {
                result += "\\u00";
                result.push_back(hex[c >> 4]);
                result.push_back(hex[c & 0x0F]);
            }
            else
            {
                result.push_back(static_cast<char>(c));
            }
        }
    }
    result.push_back('"');
    return result;
}

} // namespace thinc
