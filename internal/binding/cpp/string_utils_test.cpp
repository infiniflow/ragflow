#include <cassert>
#include <string>
#include <vector>

#include "string_utils.h"

int main() {
    const std::string truncated_utf8 = "\xE3";
    assert(FullWidthToHalfWidth(truncated_utf8) == truncated_utf8);
    const std::string invalid_continuation = "\xE3\x80";
    assert(FullWidthToHalfWidth(invalid_continuation) == invalid_continuation);
    assert(FullWidthToHalfWidth("\xEF\xBC\xA1\xE3\x80\x80") == "A ");

    constexpr size_t output_limit = 4096 * 3;
    std::string input(output_limit * 2, 'A');
    std::vector<char> guarded_output(output_limit + 2, 'x');
    char* output = guarded_output.data() + 1;

    ToLower(input.data(), input.size(), output, output_limit);

    for (size_t i = 0; i < output_limit - 1; ++i) {
        assert(output[i] == 'a');
    }
    assert(output[output_limit - 1] == '\0');
    assert(guarded_output.front() == 'x');
    assert(guarded_output.back() == 'x');

    ToLower(input.data(), input.size(), nullptr, 0);

    std::string long_token(output_limit * 2, 'B');
    std::vector<char> full_output(long_token.size() + 1);
    ToLower(long_token.data(), long_token.size(), full_output.data(), full_output.size());
    assert(std::string(full_output.data()) == std::string(long_token.size(), 'b'));
}
