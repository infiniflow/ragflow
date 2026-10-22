#include <iostream>
#include <thread>
#include <vector>
#include <cassert>
#include <cstring>
#include <string>
#include "rag_analyzer_c_api.h"

// Test case 1: Single thread, loop 1000 times
void test_single_thread() {
    std::cout << "Test 1: Single thread, 1000 iterations..." << std::endl;

    // Create analyzer instance
    RAGAnalyzerHandle handle = RAGAnalyzer_Create("./resource");
    assert(handle != nullptr && "Failed to create RAGAnalyzer");

    // Load the analyzer
    int result = RAGAnalyzer_Load(handle);
    if (result != 0) {
        printf("Failed to load RAGAnalyzer: %d\n", result);
    }
    assert(result == 0 && "Failed to load RAGAnalyzer");

    const char* input = "rag";
    bool all_passed = true;

    for (int i = 0; i < 1000; ++i) {
        char* tokens = RAGAnalyzer_Tokenize(handle, input);

        if (tokens == nullptr || strlen(tokens) == 0) {
            std::cerr << "Iteration " << i << ": Failed - returned empty or null string" << std::endl;
            all_passed = false;
        }

        // Free the returned string
        if (tokens != nullptr) {
            free(tokens);
        }
    }

    // Destroy analyzer instance
    RAGAnalyzer_Destroy(handle);

    if (all_passed) {
        std::cout << "Test 1: PASSED" << std::endl;
    } else {
        std::cout << "Test 1: FAILED" << std::endl;
        exit(1);
    }
}

// Test case 2: 16 threads, each loop 1000 times
void test_multi_thread() {
    std::cout << "Test 2: 32 threads, each 100000 iterations..." << std::endl;

    // Create analyzer instance (shared across threads)
    RAGAnalyzerHandle handle = RAGAnalyzer_Create(".");
    assert(handle != nullptr && "Failed to create RAGAnalyzer");

    // Load the analyzer
    int result = RAGAnalyzer_Load(handle);
    assert(result == 0 && "Failed to load RAGAnalyzer");

    const char* input = "rag";
    const int num_threads = 32;
    const int iterations_per_thread = 100000;

    std::vector<std::thread> threads;
    std::vector<bool> thread_results(num_threads, true);

    for (int t = 0; t < num_threads; ++t) {
        threads.emplace_back([&, t]() {
            for (int i = 0; i < iterations_per_thread; ++i) {
                char* tokens = RAGAnalyzer_Tokenize(handle, input);

                if (tokens == nullptr || strlen(tokens) == 0) {
                    std::cerr << "Thread " << t << " Iteration " << i << ": Failed - returned empty or null string" << std::endl;
                    thread_results[t] = false;
                }

                // Free the returned string
                if (tokens != nullptr) {
                    free(tokens);
                }
            }
        });
    }

    // Wait for all threads to complete
    for (auto& t : threads) {
        t.join();
    }

    // Destroy analyzer instance
    RAGAnalyzer_Destroy(handle);

    bool all_passed = true;
    for (int t = 0; t < num_threads; ++t) {
        if (!thread_results[t]) {
            all_passed = false;
            break;
        }
    }

    if (all_passed) {
        std::cout << "Test 2: PASSED" << std::endl;
    } else {
        std::cout << "Test 2: FAILED" << std::endl;
        exit(1);
    }
}

// Test case 3: tokens longer than the analyzer's 12 KB lowercase buffer.
// ToLower() used to memcpy them into that fixed buffer, corrupting the heap.
// Build with -DENABLE_ASAN=ON for this test to catch an overflow, not just a
// wrong result.
void test_long_tokens() {
    std::cout << "Test 3: tokens longer than the lowercase buffer..." << std::endl;

    RAGAnalyzerHandle base = RAGAnalyzer_Create("./resource");
    assert(base != nullptr && "Failed to create RAGAnalyzer");
    int result = RAGAnalyzer_Load(base);
    assert(result == 0 && "Failed to load RAGAnalyzer");
    // Pool instances are copies of the loaded base, as in internal/tokenizer.
    RAGAnalyzerHandle handle = RAGAnalyzer_Copy(base);
    assert(handle != nullptr && "Failed to copy RAGAnalyzer");

    std::string accented;
    for (int i = 0; i < 6000; ++i) {
        accented += "ŠÔ";
    }
    const std::string ascii(20000, 'A');
    // Ends in the lead byte of a 2-byte sequence: StrQ2B read past the end.
    // Invalid UTF-8 tokenizes to nothing, so only the absence of a crash counts.
    const std::string truncated = "pôdorys \xC3";

    bool all_passed = true;
    const struct {
        const char* language;
        std::string text;
        std::string expected; // empty: only the absence of a crash is checked
    } cases[] = {
        {"English", ascii, std::string(20000, 'a')},
        {"English", accented, ""},
        {"English", truncated, ""},
    };
    for (const auto& c : cases) {
        RAGAnalyzer_SetLanguage(handle, c.language);

        char* tokens = RAGAnalyzer_Tokenize(handle, c.text.c_str());
        if (tokens == nullptr) {
            std::cerr << c.language << " Tokenize(" << c.text.size() << " bytes): null result" << std::endl;
            all_passed = false;
        } else if (!c.expected.empty() && c.expected != tokens) {
            std::cerr << c.language << " Tokenize(" << c.text.size() << " bytes): got " << strlen(tokens)
                      << " bytes, want the whole token lowercased (" << c.expected.size() << " bytes)" << std::endl;
            all_passed = false;
        }
        free(tokens);

        RAGTokenList* list = RAGAnalyzer_TokenizeWithPosition(handle, c.text.c_str());
        if (list == nullptr) {
            std::cerr << c.language << " TokenizeWithPosition(" << c.text.size() << " bytes): null result" << std::endl;
            all_passed = false;
        }
        RAGAnalyzer_FreeTokenList(list);
    }

    RAGAnalyzer_Destroy(handle);
    RAGAnalyzer_Destroy(base);

    if (all_passed) {
        std::cout << "Test 3: PASSED" << std::endl;
    } else {
        std::cout << "Test 3: FAILED" << std::endl;
        exit(1);
    }
}

int main() {
    std::cout << "Please make sure the './resource' directory exists and is accessible." << std::endl;
    std::cout << "=== RAGAnalyzer C API Test ===" << std::endl;

    test_single_thread();
    // test_multi_thread();
    test_long_tokens();

    std::cout << "=== All tests PASSED ===" << std::endl;
    return 0;
}
