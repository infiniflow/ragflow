// Copyright(C) 2024 InfiniFlow, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#include "wordnet_lemmatizer.h"
#include <fstream>
#include <filesystem>

namespace fs = std::filesystem;

static const std::string ADJ = "a";
static const std::string ADJ_SAT = "s";
static const std::string ADV = "r";
static const std::string NOUN = "n";
static const std::string VERB = "v";

WordNetLemmatizer::WordNetLemmatizer(const std::string &wordnet_path) : wordnet_path_(wordnet_path) { Load(); }

WordNetLemmatizer::~WordNetLemmatizer() = default;

int32_t WordNetLemmatizer::Load() {
    file_map_ = {{ADJ, "adj"}, {ADV, "adv"}, {NOUN, "noun"}, {VERB, "verb"}};

    MORPHOLOGICAL_SUBSTITUTIONS = {
        {NOUN, {{"s", ""}, {"ses", "s"}, {"ves", "f"}, {"xes", "x"}, {"zes", "z"}, {"ches", "ch"}, {"shes", "sh"}, {"men", "man"}, {"ies", "y"}}},
        {VERB, {{"s", ""}, {"ies", "y"}, {"es", "e"}, {"es", ""}, {"ed", "e"}, {"ed", ""}, {"ing", "e"}, {"ing", ""}}},
        {ADJ, {{"er", ""}, {"est", ""}, {"er", "e"}, {"est", "e"}}},
        {ADV, {}},
        {ADJ_SAT, {{"er", ""}, {"est", ""}, {"er", "e"}, {"est", "e"}}}};

    POS_LIST = {NOUN, VERB, ADJ, ADV};

    auto ret = LoadLemmas();
    if (ret != 0) {
        return ret;
    }

    LoadExceptions();
    // return Status::OK();
    return 0;
}

int32_t WordNetLemmatizer::LoadLemmas() {
    fs::path root(wordnet_path_);
    for (const auto &pair : file_map_) {
        const std::string &pos_abbrev = pair.first;
        const std::string &pos_name = pair.second;
        fs::path index_path(root / ("index." + pos_name));

        std::ifstream file(index_path.string());
        if (!file.is_open()) {
            return -1;
            // return Status::InvalidAnalyzerFile(fmt::format("Failed to load WordNet lemmatizer, index.{}", pos_name));
        }

        std::string line;

        while (std::getline(file, line)) {
            if (line.empty() || line[0] == ' ') {
                continue;
            }

            std::istringstream stream(line);
            try {
                std::string lemma;
                stream >> lemma;

                if (lemmas_.find(lemma) == lemmas_.end()) {
                    lemmas_[lemma] = std::unordered_set<std::string>();
                }
                lemmas_[lemma].insert(pos_abbrev);

                if (pos_abbrev == ADJ) {
                    if (lemmas_.find(lemma) == lemmas_.end()) {
                        lemmas_[lemma] = std::unordered_set<std::string>();
                    }
                    lemmas_[lemma].insert(ADJ_SAT);
                }

            } catch (const std::exception &e) {
                return -1;
                // return Status::InvalidAnalyzerFile("Failed to load WordNet lemmatizer lemmas");
            }
        }
    }
    // return Status::OK();
    return 0;
}

void WordNetLemmatizer::LoadExceptions() {
    fs::path root(wordnet_path_);
    for (const auto &pair : file_map_) {
        const std::string &pos_abbrev = pair.first;
        const std::string &pos_name = pair.second;
        fs::path exc_path(root / (pos_name + ".exc"));

        std::ifstream file(exc_path.string());
        if (!file.is_open()) {
            continue;
        }

        exceptions_[pos_abbrev] = {};

        std::string line;
        while (std::getline(file, line)) {
            std::istringstream stream(line);
            std::string inflected_form;
            stream >> inflected_form;

            std::vector<std::string> base_forms;
            std::string base_form;
            while (stream >> base_form) {
                base_forms.push_back(base_form);
            }

            exceptions_[pos_abbrev][inflected_form] = base_forms;
        }
    }
    exceptions_[ADJ_SAT] = exceptions_[ADJ];
}

std::vector<std::string> WordNetLemmatizer::CollectSubstitutions(const std::vector<std::string> &forms, const std::string &pos) {
    const auto &substitutions = MORPHOLOGICAL_SUBSTITUTIONS.at(pos);
    std::vector<std::string> results;

    for (const auto &form : forms) {
        for (const auto &[old_suffix, new_suffix] : substitutions) {
            if (form.size() >= old_suffix.size() && form.compare(form.size() - old_suffix.size(), old_suffix.size(), old_suffix) == 0) {
                results.push_back(form.substr(0, form.size() - old_suffix.size()) + new_suffix);
            }
        }
    }
    return results;
}

std::vector<std::string> WordNetLemmatizer::CollectSubstitutions(const std::string &form, const std::string &pos) {
    const auto &substitutions = MORPHOLOGICAL_SUBSTITUTIONS.at(pos);
    std::vector<std::string> results;

    for (const auto &[old_suffix, new_suffix] : substitutions) {
        if (form.size() >= old_suffix.size() && form.compare(form.size() - old_suffix.size(), old_suffix.size(), old_suffix) == 0) {
            results.push_back(form.substr(0, form.size() - old_suffix.size()) + new_suffix);
        }
    }
    return results;
}

std::vector<std::string> WordNetLemmatizer::FilterForms(const std::vector<std::string> &forms, const std::string &pos) {
    std::vector<std::string> result;
    std::unordered_set<std::string> seen;

    for (const auto &form : forms) {
        if (lemmas_.find(form) != lemmas_.end()) {
            if (lemmas_[form].find(pos) != lemmas_[form].end()) {
                if (seen.find(form) == seen.end()) {
                    result.push_back(form);
                    seen.insert(form);
                }
            }
        }
    }
    return result;
}

std::vector<std::string> WordNetLemmatizer::Morphy(const std::string &form, const std::string &pos, bool check_exceptions) {
    const auto &pos_exceptions = exceptions_.at(pos);

    // Check exceptions first
    if (check_exceptions && pos_exceptions.find(form) != pos_exceptions.end()) {
        std::vector<std::string> forms = pos_exceptions.at(form);
        forms.push_back(form);
        return FilterForms(forms, pos);
    }

    // Apply morphological rules with recursion (like Java version)
    std::vector<std::string> forms = CollectSubstitutions(form, pos);
    std::vector<std::string> combined_forms = forms;
    combined_forms.push_back(form);

    // First attempt with original form and first-level substitutions
    auto results = FilterForms(combined_forms, pos);
    if (!results.empty()) {
        return results;
    }

    // Recursively apply rules (Java version's while loop)
    while (!forms.empty()) {
        forms = CollectSubstitutions(forms, pos);
        results = FilterForms(forms, pos);
        if (!results.empty()) {
            return results;
        }
    }

    // Return empty result if no valid lemma found
    return {};
}

std::string WordNetLemmatizer::Lemmatize(const std::string &form, const std::string &pos) {
    std::vector<std::string> parts_of_speech;
    if (!pos.empty()) {
        parts_of_speech.push_back(pos);
    } else {
        parts_of_speech = POS_LIST;
    }

    for (const auto &part : parts_of_speech) {
        auto analyses = Morphy(form, part);
        if (!analyses.empty()) {
            return analyses[0];
        }
    }

    return form;
}