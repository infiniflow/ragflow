#include "wordnet_lemmatizer.h"

#include <chrono>
#include <filesystem>
#include <fstream>
#include <iostream>

int main() {
    namespace fs = std::filesystem;

    const auto path = fs::temp_directory_path() /
        ("ragflow-wordnet-test-" + std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()));
    fs::create_directories(path);
    const auto cleanup = [&path]() {
        std::error_code error;
        fs::remove_all(path, error);
    };

    for (const auto *pos : {"adj", "adv", "noun", "verb"}) {
        std::ofstream index(path / (std::string("index.") + pos));
        if (std::string(pos) == "noun") {
            index << "dog\n";
        }
    }
    for (const auto *pos : {"adj", "adv", "verb"}) {
        std::ofstream exceptions(path / (std::string(pos) + ".exc"));
    }

    try {
        WordNetLemmatizer lemmatizer(path.string());
        const auto lemma = lemmatizer.Lemmatize("dogs");
        cleanup();
        if (lemma != "dog") {
            std::cerr << "Lemmatize(dogs) = " << lemma << ", want dog\n";
            return 1;
        }
    } catch (const std::exception &error) {
        cleanup();
        std::cerr << "Lemmatize threw with noun.exc missing: " << error.what() << '\n';
        return 1;
    }

    return 0;
}
