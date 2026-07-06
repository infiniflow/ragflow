//
// Created by infiniflow on 2/2/26.
//

#include <iostream>
#include <fstream>
#include <filesystem>
#include <numeric>
#include <unordered_set>
#include <cassert>
#include "rag_analyzer.h"

namespace fs = std::filesystem;

void test_analyze_enable_position() {
    fs::path RESOURCE_DIR = "/usr/share/infinity/resource";
    if (!fs::exists(RESOURCE_DIR)) {
        std::cerr << "Resource directory doesn't exist: " << RESOURCE_DIR << std::endl;
        return;
    }

    std::string rag_tokenizer_path_ = "test";
    std::string input_file_ = rag_tokenizer_path_ + "/tokenizer_input.txt";

    std::cout << "Looking for input file: " << input_file_ << std::endl;
    std::cout << "Current directory: " << fs::current_path() << std::endl;

    if (!fs::exists(input_file_)) {
        std::cerr << "ERROR: Input file doesn't exist: " << input_file_ << std::endl;
        std::cerr << "Full path: " << fs::absolute(input_file_) << std::endl;
        return;
    }

    std::ifstream infile(input_file_);
    if (!infile.is_open()) {
        std::cerr << "ERROR: Cannot open file: " << input_file_ << std::endl;
        std::cerr << "Error code: " << strerror(errno) << std::endl;
        return;
    }

    infile.seekg(0, std::ios::end);
    size_t file_size = infile.tellg();
    infile.seekg(0, std::ios::beg);
    std::cout << "File size: " << file_size << " bytes" << std::endl;

    auto analyzer_ = new RAGAnalyzer(RESOURCE_DIR.string());
    analyzer_->Load();

    analyzer_->SetEnablePosition(false);
    analyzer_->SetFineGrained(false);

    analyzer_->SetEnablePosition(true);
    analyzer_->SetFineGrained(false);

    std::string line;
    while (std::getline(infile, line)) {
        if (line.empty())
            continue;

        TermList term_list;
        analyzer_->Analyze(line, term_list);
        std::cout << "Input text: " << std::endl << line << std::endl;

        std::cout << "Analyze result: " << std::endl;
        for (unsigned i = 0; i < term_list.size(); ++i) {
            std::cout << "[" << term_list[i].text_ << "@" << term_list[i].word_offset_ << "," << term_list[i].
                end_offset_ << "] ";
        }
        std::cout << std::endl;
    }
    infile.close();

    delete analyzer_;
    analyzer_ = nullptr;
}

void test_analyze_enable_position_fine_grained() {
    fs::path RESOURCE_DIR = "/usr/share/infinity/resource";
    if (!fs::exists(RESOURCE_DIR)) {
        std::cerr << "Resource directory doesn't exist: " << RESOURCE_DIR << std::endl;
        return;
    }

    std::string rag_tokenizer_path_ = "test";
    std::string input_file_ = rag_tokenizer_path_ + "/tokenizer_input.txt";

    std::cout << "Looking for input file: " << input_file_ << std::endl;
    std::cout << "Current directory: " << fs::current_path() << std::endl;

    if (!fs::exists(input_file_)) {
        std::cerr << "ERROR: Input file doesn't exist: " << input_file_ << std::endl;
        std::cerr << "Full path: " << fs::absolute(input_file_) << std::endl;
        return;
    }

    std::ifstream infile(input_file_);
    if (!infile.is_open()) {
        std::cerr << "ERROR: Cannot open file: " << input_file_ << std::endl;
        std::cerr << "Error code: " << strerror(errno) << std::endl;
        return;
    }

    infile.seekg(0, std::ios::end);
    size_t file_size = infile.tellg();
    infile.seekg(0, std::ios::beg);
    std::cout << "File size: " << file_size << " bytes" << std::endl;

    auto analyzer_ = new RAGAnalyzer(RESOURCE_DIR.string());
    analyzer_->Load();

    analyzer_->SetEnablePosition(true);
    analyzer_->SetFineGrained(true);

    std::string line;

    while (std::getline(infile, line)) {
        if (line.empty())
            continue;

        TermList term_list;
        analyzer_->Analyze(line, term_list);
        std::cout << "Input text: " << std::endl << line << std::endl;

        std::cout << "Analyze result: " << std::endl;
        for (unsigned i = 0; i < term_list.size(); ++i) {
            std::cout << "[" << term_list[i].text_ << "@" << term_list[i].word_offset_ << "," << term_list[i].
                end_offset_ << "] ";
        }
        std::cout << std::endl;
    }
    infile.close();

    delete analyzer_;
    analyzer_ = nullptr;
}

void test_tokenize_consistency_with_position() {
    fs::path RESOURCE_DIR = "/usr/share/infinity/resource";
    if (!fs::exists(RESOURCE_DIR)) {
        std::cerr << "Resource directory doesn't exist: " << RESOURCE_DIR << std::endl;
        return;
    }

    std::string rag_tokenizer_path_ = "test";
    std::string input_file_ = rag_tokenizer_path_ + "/tokenizer_input.txt";

    std::cout << "Looking for input file: " << input_file_ << std::endl;
    std::cout << "Current directory: " << fs::current_path() << std::endl;

    if (!fs::exists(input_file_)) {
        std::cerr << "ERROR: Input file doesn't exist: " << input_file_ << std::endl;
        std::cerr << "Full path: " << fs::absolute(input_file_) << std::endl;
        return;
    }

    std::ifstream infile(input_file_);
    if (!infile.is_open()) {
        std::cerr << "ERROR: Cannot open file: " << input_file_ << std::endl;
        std::cerr << "Error code: " << strerror(errno) << std::endl;
        return;
    }

    infile.seekg(0, std::ios::end);
    size_t file_size = infile.tellg();
    infile.seekg(0, std::ios::beg);
    std::cout << "File size: " << file_size << " bytes" << std::endl;

    auto analyzer_ = new RAGAnalyzer(RESOURCE_DIR.string());
    analyzer_->Load();

    std::string line;

    while (std::getline(infile, line)) {
        if (line.empty())
            continue;

        // Test Tokenize (returns string)
        std::string tokens_str = analyzer_->Tokenize(line);
        std::istringstream iss(tokens_str);
        std::string token;
        std::vector<std::string> tokenize_result;
        while (iss >> token) {
            tokenize_result.push_back(token);
        }

        std::cout << "Input text: " << std::endl << line << std::endl;
        std::cout << "Tokenize result: " << std::endl << tokens_str << std::endl;

        // Test TokenizeWithPosition (returns vector of tokens and positions)
        auto [tokenize_with_pos_result, positions] = analyzer_->TokenizeWithPosition(line);

        // Check if results are identical
        bool tokens_match = (tokenize_result.size() == tokenize_with_pos_result.size());
        if (tokens_match) {
            for (size_t i = 0; i < tokenize_result.size(); ++i) {
                if (tokenize_result[i] != tokenize_with_pos_result[i]) {
                    tokens_match = false;
                    break;
                }
            }
        }

        assert(tokens_match == true);
        if (!tokens_match) {
            std::cout << "Tokenize count: " << tokenize_result.size() << ", TokenizeWithPosition count: " <<
                tokenize_with_pos_result.size()
                << std::endl;

            std::cout << "TokenizeWithPosition result: " << std::endl;
            std::string result_str = std::accumulate(tokenize_with_pos_result.begin(),
                                                     tokenize_with_pos_result.end(),
                                                     std::string(""),
                                                     [](const std::string &a, const std::string &b) {
                                                         return a + (a.empty() ? "" : " ") + b;
                                                     });
            std::cout << result_str << std::endl;
        }
    }
    infile.close();

    delete analyzer_;
    analyzer_ = nullptr;
}

std::vector<std::string> SplitString(const std::string &str) {
    std::vector<std::string> tokens;
    std::stringstream ss(str);
    std::string token;

    while (ss >> token) {
        tokens.push_back(token);
    }

    return tokens;
}

void test_tokenize_consistency_with_python() {
    fs::path RESOURCE_DIR = "/usr/share/infinity/resource";
    if (!fs::exists(RESOURCE_DIR)) {
        std::cerr << "Resource directory doesn't exist: " << RESOURCE_DIR << std::endl;
        return;
    }

    std::string rag_tokenizer_path_ = "test";
    std::string input_file_ = rag_tokenizer_path_ + "/tokenizer_input.txt";

    std::cout << "Looking for input file: " << input_file_ << std::endl;
    std::cout << "Current directory: " << fs::current_path() << std::endl;

    if (!fs::exists(input_file_)) {
        std::cerr << "ERROR: Input file doesn't exist: " << input_file_ << std::endl;
        std::cerr << "Full path: " << fs::absolute(input_file_) << std::endl;
        return;
    }

    std::ifstream infile(input_file_);
    if (!infile.is_open()) {
        std::cerr << "ERROR: Cannot open file: " << input_file_ << std::endl;
        std::cerr << "Error code: " << strerror(errno) << std::endl;
        return;
    }

    infile.seekg(0, std::ios::end);
    size_t file_size = infile.tellg();
    infile.seekg(0, std::ios::beg);
    std::cout << "File size: " << file_size << " bytes" << std::endl;

    auto analyzer_ = new RAGAnalyzer(RESOURCE_DIR.string());
    analyzer_->Load();

    std::unordered_set<std::string> mismatch_tokens_ = {"be", "datum", "ccs", "experi", "fast", "llms", "larg", "ass"};

    std::ifstream infile_python(rag_tokenizer_path_ + "/tokenizer_python_output.txt");
    std::string line;
    std::string python_tokens;
    while (std::getline(infile, line)) {
        if (line.empty())
            continue;

        std::string tokens = analyzer_->Tokenize(line);
        std::cout << "Input text: " << std::endl << line << std::endl;
        std::cout << "Tokenize result: " << std::endl << tokens << std::endl;

        std::getline(infile_python, python_tokens);

        std::vector<std::string> tokenize_result = SplitString(tokens);
        std::vector<std::string> python_tokenize_result = SplitString(python_tokens);

        bool is_size_match = tokenize_result.size() == python_tokenize_result.size();
        assert(is_size_match == true);

        bool is_match = true;
        bool is_bad_token = false;
        if (is_size_match) {
            for (size_t i = 0; i < tokenize_result.size(); ++i) {
                if (tokenize_result[i] != python_tokenize_result[i]) {
                    is_bad_token = mismatch_tokens_.contains(tokenize_result[i]);
                    if (!is_bad_token) {
                        is_match = false;
                        break;
                    }
                }
            }
            assert(is_match == true);
        }
        if (!is_size_match || !is_match || is_bad_token) {
            std::cout << "Tokenize count: " << tokenize_result.size() << ", Python tokenize count: " <<
                python_tokenize_result.size() << std::endl;

            std::cout << "Python tokenize result: " << std::endl << python_tokens << std::endl;
        }
    }
    infile.close();

    delete analyzer_;
    analyzer_ = nullptr;
}

void test_fine_grained_tokenize_consistency_with_python() {
    fs::path RESOURCE_DIR = "/usr/share/infinity/resource";
    if (!fs::exists(RESOURCE_DIR)) {
        std::cerr << "Resource directory doesn't exist: " << RESOURCE_DIR << std::endl;
        return;
    }

    std::string rag_tokenizer_path_ = "test";
    std::string input_file_ = rag_tokenizer_path_ + "/tokenizer_input.txt";

    std::cout << "Looking for input file: " << input_file_ << std::endl;
    std::cout << "Current directory: " << fs::current_path() << std::endl;

    if (!fs::exists(input_file_)) {
        std::cerr << "ERROR: Input file doesn't exist: " << input_file_ << std::endl;
        std::cerr << "Full path: " << fs::absolute(input_file_) << std::endl;
        return;
    }

    std::ifstream infile(input_file_);
    if (!infile.is_open()) {
        std::cerr << "ERROR: Cannot open file: " << input_file_ << std::endl;
        std::cerr << "Error code: " << strerror(errno) << std::endl;
        return;
    }

    infile.seekg(0, std::ios::end);
    size_t file_size = infile.tellg();
    infile.seekg(0, std::ios::beg);
    std::cout << "File size: " << file_size << " bytes" << std::endl;

    auto analyzer_ = new RAGAnalyzer(RESOURCE_DIR.string());
    analyzer_->Load();

    std::unordered_set<std::string> mismatch_tokens_ = {"be", "datum", "ccs", "experi", "fast", "llms", "larg", "ass"};

    analyzer_->SetEnablePosition(false);
    analyzer_->SetFineGrained(true);

    std::ifstream infile_python(rag_tokenizer_path_ + "/fine_grained_tokenizer_python_output.txt");
    std::string line;
    std::string python_tokens;
    while (std::getline(infile, line)) {
        if (line.empty())
            continue;

        TermList term_list;
        analyzer_->Analyze(line, term_list);

        std::string fine_grained_tokens =
            std::accumulate(term_list.begin(),
                            term_list.end(),
                            std::string(""),
                            [](const std::string &a, const Term &b) {
                                return a + (a.empty() ? "" : " ") + b.text_;
                            });

        std::cout << "Input text: " << std::endl << line << std::endl;
        std::cout << "Fine grained tokenize result: " << std::endl << fine_grained_tokens << std::endl;

        std::getline(infile_python, python_tokens);
        std::vector<std::string> python_tokenize_result = SplitString(python_tokens);

        bool is_size_match = term_list.size() == python_tokenize_result.size();
        assert(is_size_match == true);

        bool is_match = true;
        bool is_bad_token = false;
        if (is_size_match) {
            for (size_t i = 0; i < term_list.size(); ++i) {
                if (term_list[i].text_ != python_tokenize_result[i]) {
                    is_bad_token = mismatch_tokens_.contains(term_list[i].text_);
                    if (!is_bad_token) {
                        is_match = false;
                        break;
                    }
                }
            }
            assert(is_match == true);
        }
        if (!is_size_match || !is_match || is_bad_token) {
            std::cout << "Tokenize count: " << term_list.size() << ", Python tokenize count: " << python_tokenize_result
                .size() << std::endl;

            std::cout << "Python tokenize result: " << std::endl << python_tokens << std::endl;
        }
    }
    infile.close();

    delete analyzer_;
    analyzer_ = nullptr;
}

void test_tokenize_text(const std::string& text)
{
    fs::path RESOURCE_DIR = "/usr/share/infinity/resource";
    if (!fs::exists(RESOURCE_DIR)) {
        std::cerr << "Resource directory doesn't exist: " << RESOURCE_DIR << std::endl;
        return;
    }
    auto analyzer_ = new RAGAnalyzer(RESOURCE_DIR.string());
    analyzer_->Load();


    analyzer_->SetEnablePosition(false);
    analyzer_->SetFineGrained(false);

    std::string tokens = analyzer_->Tokenize(text);
    std::cout << "Input text: " << std::endl << text << std::endl;
    std::cout << "Tokenize result: " << std::endl << tokens << std::endl;

    delete analyzer_;
    analyzer_ = nullptr;
}

int main() {
    // test_analyze_enable_position();
    // test_analyze_enable_position_fine_grained();
    // test_tokenize_consistency_with_position();
    // test_tokenize_consistency_with_python();
    // test_fine_grained_tokenize_consistency_with_python();
    test_tokenize_text("在本研究中，我们提出了一种novel的neural network架构，用于解决multi-modal learning问题。我们的方法结合了CNN(Convolutional Neural Networks)和Transformer的优势，在ImageNet数据集上达到了state-of-the-art性能。实验结果表明，在batch size为256、learning rate为0.001的条件下，我们的模型在validation set上的accuracy达到了95.7%，比baseline方法提高了3.2%。此外，我们还进行了ablation study来分析不同components的contribution。所有代码已在GitHub上开源，地址是https://github.com/example/our-project。未来工作将集中在model compression和real-time inference optimization上。");
    return 0;
}