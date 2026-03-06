# Makefile for RAG Tokenizer with CGO bindings

.PHONY: all clean build c_api c_api_debug c_api_asan test_go test_memory valgrind asan

BUILD_DIR := build
ASAN_BUILD_DIR := build-asan
C_API_LIB := $(BUILD_DIR)/librag_tokenizer_c_api.a
C_API_ASAN_LIB := $(ASAN_BUILD_DIR)/librag_tokenizer_c_api.a
C_API_DEBUG_LIB := $(BUILD_DIR)/librag_tokenizer_c_api_debug.a

all: build c_api

# Create build directory
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(ASAN_BUILD_DIR):
	mkdir -p $(ASAN_BUILD_DIR)

# Build the main executable and C API library
build: $(BUILD_DIR)
	cd $(BUILD_DIR) && cmake .. && make -j$$(nproc)

# Build only the C API library
c_api: $(BUILD_DIR)
	cd $(BUILD_DIR) && cmake .. && make rag_tokenizer_c_api -j$$(nproc)

# Build C API library with AddressSanitizer
c_api_asan: $(ASAN_BUILD_DIR)
	cd $(ASAN_BUILD_DIR) && cmake .. -DENABLE_ASAN=ON && make rag_tokenizer_c_api -j$$(nproc)
	@echo "ASan library built: $(C_API_ASAN_LIB)"

# Build debug version of C API library with memory tracking
c_api_debug: $(BUILD_DIR)
	cd $(BUILD_DIR) && \
	g++ -std=c++17 -static-libgcc -static-libstdc++ -DMEMORY_DEBUG \
		-I.. \
		../rag_analyzer_c_api_debug.cpp \
		../rag_analyzer.cpp \
		../darts_trie.cpp \
		../wordnet_lemmatizer.cpp \
		../term.cpp \
		../tokenizer.cpp \
		../stemmer/*.cpp \
		../opencc/*.c ../opencc/*.cpp \
		../util/*.cc \
		../re2/*.cc \
		-o librag_tokenizer_c_api_debug.a \
		-lstdc++ -lm -lpthread -lpcre2-8
	@echo "Debug library built: $(C_API_DEBUG_LIB)"

# Test the Go bindings
test_go: c_api
	cd go_bindings/example && go run main.go ../../$(BUILD_DIR) "这是一个测试文本。This is a test."

# Run memory test
test_memory: c_api
	cd go_bindings/example && go run memory_leak_check.go

# Run with valgrind
valgrind: c_api
	cd go_bindings/example && bash run_valgrind.sh

# Run with AddressSanitizer
asan: c_api_asan
	@echo "Running with AddressSanitizer..."
	cd go_bindings/example && \
	ASAN_OPTIONS=detect_leaks=1:print_stats=1:verbosity=0 \
	go run memory_leak_check.go

# Install the C API library (optional)
install: c_api
	sudo cp $(C_API_LIB) /usr/local/lib/
	sudo ldconfig

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -rf $(ASAN_BUILD_DIR)
	rm -f go_bindings/example/valgrind.log
	rm -f go_bindings/example/memory_test_bin
