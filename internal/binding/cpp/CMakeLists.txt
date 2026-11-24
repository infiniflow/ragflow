cmake_minimum_required(VERSION 4.0)
project(rag_tokenizer)

set(CMAKE_CXX_STANDARD 23)

include(CTest)

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
#   - macOS: resolve the STATIC archive and stage it in this build dir, because
#     the cgo directives in internal/binding link pcre2 from there: cgo cannot
#     search library paths, and naming a Homebrew prefix in the Go source is
#     wrong for every host whose Homebrew lives elsewhere. find_file() rather
#     than find_library(), because the macOS SDK carries a .tbd stub for the
#     system libpcre2 that find_library() happily returns — a stub is not a
#     link-time dependency we want to pick up by accident. The search order is
#     CMake's standard one (CMAKE_PREFIX_PATH, then the Homebrew prefix, then
#     the usual system paths), and PCRE2_STATIC_LIB overrides it outright.
if(APPLE)
    # HINTS, not CMAKE_PREFIX_PATH: a non-empty CMAKE_PREFIX_PATH variable hides
    # the same environment variable, which would mask a caller-supplied prefix.
    find_file(PCRE2_STATIC_LIB NAMES libpcre2-8.a PATH_SUFFIXES lib
            HINTS "${HOMEBREW_PREFIX}" REQUIRED)

    # A pcre2 built for the other architecture fails deep inside ld64
    # ("ignoring file ... wrong architecture", then undefined symbols). Say it
    # here, with the way out.
    execute_process(
        COMMAND lipo -archs "${PCRE2_STATIC_LIB}"
        OUTPUT_VARIABLE _ragflow_pcre2_archs
        OUTPUT_STRIP_TRAILING_WHITESPACE
        ERROR_QUIET
    )
    set(_ragflow_target_archs ${CMAKE_OSX_ARCHITECTURES})
    if(NOT _ragflow_target_archs)
        set(_ragflow_target_archs ${CMAKE_SYSTEM_PROCESSOR})
    endif()
    foreach(_arch IN LISTS _ragflow_target_archs)
        if(_ragflow_pcre2_archs AND NOT _ragflow_pcre2_archs MATCHES "(^| )${_arch}( |$)")
            message(FATAL_ERROR
                "libpcre2-8.a at ${PCRE2_STATIC_LIB} is built for '${_ragflow_pcre2_archs}', "
                "but this build targets '${_arch}'.\n"
                "Install pcre2 with the Homebrew that matches the target architecture, "
                "or point CMake at a matching archive:\n"
                "  cmake ... -DPCRE2_STATIC_LIB=/path/to/libpcre2-8.a\n"
                "  build.sh: RAGFLOW_CMAKE_ARGS=-DPCRE2_STATIC_LIB=/path/to/libpcre2-8.a bash build.sh")
        endif()
    endforeach()
    set(PCRE2_LIB "${PCRE2_STATIC_LIB}")
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
        spm_unigram.cpp
        spm_unigram.h
        thinc_ner.cpp
        thinc_ner.h
        thinc_parser.cpp
        thinc_parser.h
        thinc_json.h
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

# macOS: stage the static pcre2 next to the archive the cgo directives already
# link, so the Go link finds it without any hard-coded Homebrew prefix (see the
# PCRE2_LIB resolution above).
if(APPLE)
    add_custom_command(TARGET rag_tokenizer_c_api POST_BUILD
            COMMAND ${CMAKE_COMMAND} -E copy_if_different
                    "${PCRE2_LIB}"
                    "$<TARGET_FILE_DIR:rag_tokenizer_c_api>/libpcre2-8.a"
            COMMENT "Staging libpcre2-8.a for the cgo link"
            VERBATIM)
endif()

# Test executable for C API
add_executable(rag_analyzer_c_test
        rag_analyzer_c_test.cpp
)

target_link_libraries(rag_analyzer_c_test rag_tokenizer_c_api stdc++ m ${PCRE2_LIB})
target_include_directories(rag_analyzer_c_test PUBLIC "${CMAKE_SOURCE_DIR}")
set_target_properties(rag_analyzer_c_test PROPERTIES
        CXX_STANDARD 20
        CXX_STANDARD_REQUIRED ON
)

if(BUILD_TESTING)
    add_executable(thinc_json_test
            thinc_json_test.cpp
    )
    target_link_libraries(thinc_json_test rag_tokenizer_c_api stdc++ m ${PCRE2_LIB})
    target_include_directories(thinc_json_test PUBLIC "${CMAKE_SOURCE_DIR}")
    set_target_properties(thinc_json_test PROPERTIES
            CXX_STANDARD 20
            CXX_STANDARD_REQUIRED ON
    )
    add_test(NAME thinc_json COMMAND thinc_json_test)

    add_executable(rag_analyzer_opencc_copy_test
            rag_analyzer_opencc_copy_test.cpp
    )
    target_link_libraries(rag_analyzer_opencc_copy_test rag_tokenizer_c_api stdc++ m ${PCRE2_LIB})
    target_include_directories(rag_analyzer_opencc_copy_test PUBLIC "${CMAKE_SOURCE_DIR}")
    set_target_properties(rag_analyzer_opencc_copy_test PROPERTIES
            CXX_STANDARD 20
            CXX_STANDARD_REQUIRED ON
    )
    add_test(NAME rag_analyzer_opencc_copy COMMAND rag_analyzer_opencc_copy_test)

    add_executable(wordnet_lemmatizer_missing_exceptions_test
            wordnet_lemmatizer_test.cpp
            wordnet_lemmatizer.cpp
    )
    target_include_directories(wordnet_lemmatizer_missing_exceptions_test PUBLIC "${CMAKE_SOURCE_DIR}")
    set_target_properties(wordnet_lemmatizer_missing_exceptions_test PROPERTIES
            CXX_STANDARD 20
            CXX_STANDARD_REQUIRED ON
    )
    add_test(NAME wordnet_lemmatizer_missing_exceptions
            COMMAND wordnet_lemmatizer_missing_exceptions_test)
endif()
