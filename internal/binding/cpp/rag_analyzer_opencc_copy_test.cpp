#include "rag_analyzer.h"

#include <atomic>
#include <chrono>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <memory>
#include <string>
#include <thread>
#include <vector>

int main() {
    namespace fs = std::filesystem;

    const fs::path resource_path = fs::temp_directory_path() /
        ("rag_analyzer_opencc_copy_test_" +
         std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()));
    const fs::path opencc_path = resource_path / "opencc";
    fs::create_directories(opencc_path);

    {
        std::ofstream config(opencc_path / "zht2zhs.ini");
        config << "title = test\n"
               << "dict0 = TEXT test.txt\n";
        if (!config) {
            std::cerr << "failed to write OpenCC config\n";
            fs::remove_all(resource_path);
            return 1;
        }
    }
    {
        std::ofstream dictionary(opencc_path / "test.txt");
        dictionary << "繁體\t繁体\n";
        if (!dictionary) {
            std::cerr << "failed to write OpenCC dictionary\n";
            fs::remove_all(resource_path);
            return 1;
        }
    }

    RAGAnalyzer base(resource_path.string());
    base.opencc_ = std::make_unique<::OpenCC>(opencc_path.string());

    constexpr int worker_count = 8;
    std::vector<std::unique_ptr<RAGAnalyzer>> analyzers;
    analyzers.reserve(worker_count);
    for (int i = 0; i < worker_count; ++i) {
        auto copy = std::make_unique<RAGAnalyzer>(base);
        if (!copy->opencc_ || copy->opencc_.get() == base.opencc_.get()) {
            std::cerr << "RAGAnalyzer copy shares its OpenCC converter\n";
            fs::remove_all(resource_path);
            return 1;
        }
        for (const auto& existing : analyzers) {
            if (copy->opencc_.get() == existing->opencc_.get()) {
                std::cerr << "RAGAnalyzer copies share an OpenCC converter\n";
                fs::remove_all(resource_path);
                return 1;
            }
        }
        analyzers.push_back(std::move(copy));
    }

    std::string input;
    std::string expected;
    for (int i = 0; i < 128; ++i) {
        input += "繁體";
        expected += "繁体";
    }

    std::atomic<int> failures{0};
    std::vector<std::thread> workers;
    workers.reserve(worker_count);
    for (int i = 0; i < worker_count; ++i) {
        workers.emplace_back([&, i] {
            for (int iteration = 0; iteration < 100; ++iteration) {
                std::string converted;
                if (analyzers[i]->opencc_->convert(input, converted) < 0 || converted != expected) {
                    ++failures;
                }
            }
        });
    }
    for (auto& worker : workers) {
        worker.join();
    }

    fs::remove_all(resource_path);
    if (failures != 0) {
        std::cerr << "concurrent OpenCC conversions failed " << failures << " times\n";
        return 1;
    }
    return 0;
}
