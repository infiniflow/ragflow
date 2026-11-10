#include "thinc_ner.h"
#include "thinc_json.h"

#include <iostream>
#include <string>

int main() {
    const std::string escaped = thinc::JsonString("quote\" slash\\ line\n tab\t control\x01");
    const std::string expected_escaped = R"json("quote\" slash\\ line\n tab\t control\u0001")json";
    if (escaped != expected_escaped) {
        std::cerr << "JSON string escaping failed\nexpected: " << expected_escaped << "\nactual:   " << escaped << '\n';
        return 1;
    }

    char* result = ThincNER_Tokenize("\"say\" \\ now", "en");
    if (!result) {
        std::cerr << "ThincNER_Tokenize returned null\n";
        return 1;
    }

    const std::string expected = R"json(["\"","say","\"","\\","now"])json";
    const std::string actual(result);
    ThincNER_FreeString(result);
    if (actual != expected) {
        std::cerr << "invalid Thinc tokenizer JSON\nexpected: " << expected << "\nactual:   " << actual << '\n';
        return 1;
    }
    return 0;
}
