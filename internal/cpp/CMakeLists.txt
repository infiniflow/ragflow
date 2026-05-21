cmake_minimum_required(VERSION 4.0)
project(rag_tokenizer)

set(CMAKE_CXX_STANDARD 23)

# macOS dependency discovery — Homebrew installs headers and libs under a
# prefix that is NOT on the compiler's default search path (Apple Silicon:
# /opt/homebrew, Intel: /usr/local). Linux is left completely untouched:
# the infinity_builder image already ships pcre2 + simde where the
# toolchain finds them, so adding paths there risks shadowing them.
if(APPLE)
    execute_process(
        COMMAND brew --prefix
        OUTPUT_VARIABLE HOMEBREW_PREFIX
        OUTPUT_STRIP_TRAILING_WHITESPACE
        RESULT_VARIABLE BREW_RC
    )
    if(BREW_RC EQUAL 0 AND HOMEBREW_PREFIX)
        message(STATUS "macOS detected; Homebrew prefix: ${HOMEBREW_PREFIX}")
        include_directories(SYSTEM "${HOMEBREW_PREFIX}/include")
        link_directories("${HOMEBREW_PREFIX}/lib")
    endif()
endif()

# Resolve libpcre2-8.
#   - Linux: keep upstream's bare `libpcre2-8.a` token verbatim. The linker
#     resolves it from its own default search path, which the
#     infinity_builder image populates. find_library() does NOT see that
#     path (pcre2 is built from source there), so calling it here would
#     break the CI build that worked before.
#   - macOS: the bare token fails (libpcre2-8.a is under the Homebrew
#     prefix, off the default path), so resolve the full path explicitly.
if(APPLE)
    find_library(PCRE2_LIB NAMES pcre2-8 REQUIRED)
else()
    set(PCRE2_LIB libpcre2-8.a)
endif()
message(STATUS "PCRE2 library: ${PCRE2_LIB}")

# Option to enable AddressSanitizer
option(ENABLE_ASAN "Enable AddressSanitizer" OFF)

if(ENABLE_ASAN)
    message(STATUS "AddressSanitizer enabled")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -fsanitize=address -fno-omit-frame-pointer -g")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fsanitize=address -fno-omit-frame-pointer -g")
    set(CMAKE_EXE_LINKER_FLAGS "${CMAKE_EXE_LINKER_FLAGS} -fsanitize=address")
    set(CMAKE_SHARED_LINKER_FLAGS "${CMAKE_SHARED_LINKER_FLAGS} -fsanitize=address")
endif()

file(GLOB_RECURSE
        stemmer_src
        CONFIGURE_DEPENDS
        stemmer/*.cpp
        stemmer/*.cc
        stemmer/*.c
        stemmer/*.h
)

file(GLOB_RECURSE
        opencc_src
        CONFIGURE_DEPENDS
        opencc/*.cpp
        opencc/*.cc
        opencc/*.c
        opencc/*.h
)

file(GLOB_RECURSE
        util_src
        CONFIGURE_DEPENDS
        util/*.cpp
        util/*.cc
        util/*.c
        util/*.h
)

file(GLOB_RECURSE
        re2_src
        CONFIGURE_DEPENDS
        re2/*.cpp
        re2/*.cc
        re2/*.c
        re2/*.h
)

file(GLOB_RECURSE
        darts_src
        CONFIGURE_DEPENDS
        darts/*.h
)

file(GLOB
        main_src
        CONFIGURE_DEPENDS
        *.cpp
        *.cc
        *.c
        *.h
)

# Filter out C API files from main_src
list(FILTER main_src EXCLUDE REGEX "rag_analyzer_c_api")

add_executable(rag_tokenizer
        main.cpp
        rag_analyzer.cpp
        rag_analyzer.h
        dart_trie.h
        darts_trie.cpp
        wordnet_lemmatizer.cpp
        wordnet_lemmatizer.h
        string_utils.h
        term.h
        term.cpp
        tokenizer.cpp
        tokenizer.h
        analyzer.h
        ${stemmer_src}
        ${opencc_src}
        ${util_src}
        ${darts_src}
        ${re2_src})

target_link_libraries(rag_tokenizer stdc++ m ${PCRE2_LIB})
target_include_directories(rag_tokenizer PUBLIC "${CMAKE_SOURCE_DIR}")
set_target_properties(rag_tokenizer PROPERTIES
        CXX_STANDARD 20
        CXX_STANDARD_REQUIRED ON
)

# Build C API static library for CGO
add_library(rag_tokenizer_c_api STATIC
        rag_analyzer_c_api.cpp
        rag_analyzer_c_api.h
        rag_analyzer.cpp
        rag_analyzer.h
        dart_trie.h
        darts_trie.cpp
        wordnet_lemmatizer.cpp
        wordnet_lemmatizer.h
        string_utils.h
        term.h
        term.cpp
        tokenizer.cpp
        tokenizer.h
        analyzer.h
        ${stemmer_src}
        ${opencc_src}
        ${util_src}
        ${darts_src}
        ${re2_src}
)

target_link_libraries(rag_tokenizer_c_api stdc++ libm.a ${PCRE2_LIB})
target_include_directories(rag_tokenizer_c_api PUBLIC "${CMAKE_SOURCE_DIR}")
set_target_properties(rag_tokenizer_c_api PROPERTIES
        CXX_STANDARD 20
        CXX_STANDARD_REQUIRED ON
)

# Test executable for C API
add_executable(rag_analyzer_c_test
        rag_analyzer_c_test.cpp
)

target_link_libraries(rag_analyzer_c_test rag_tokenizer_c_api stdc++ libm.a ${PCRE2_LIB})
target_include_directories(rag_analyzer_c_test PUBLIC "${CMAKE_SOURCE_DIR}")
set_target_properties(rag_analyzer_c_test PROPERTIES
        CXX_STANDARD 20
        CXX_STANDARD_REQUIRED ON
)
