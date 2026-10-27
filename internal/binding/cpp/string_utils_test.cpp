#include <cassert>
#include <string>
#include <vector>

#include "string_utils.h"

int main() {
    constexpr size_t output_limit = 4096 * 3;
    std::string input(output_limit * 2, 'A');
    std::vector<char> output(output_limit, 'x');

    ToLower(input.data(), input.size(), output.data(), output.size());

    for (size_t i = 0; i < output_limit - 1; ++i) {
        assert(output[i] == 'a');
    }
    assert(output.back() == '\0');

    ToLower(input.data(), input.size(), nullptr, 0);
}
