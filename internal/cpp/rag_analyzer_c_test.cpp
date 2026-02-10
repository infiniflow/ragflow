#include <iostream>
#include <thread>
#include <vector>
#include <cassert>
#include <cstring>
#include "rag_analyzer_c_api.h"

// Test case 1: Single thread, loop 1000 times
void test_single_thread() {
    std::cout << "Test 1: Single thread, 1000 iterations..." << std::endl;
    
    // Create analyzer instance
    RAGAnalyzerHandle handle = RAGAnalyzer_Create(".");
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

int main() {
    std::cout << "=== RAGAnalyzer C API Test ===" << std::endl;
    
    test_single_thread();
    // test_multi_thread();
    
    std::cout << "=== All tests PASSED ===" << std::endl;
    return 0;
}
