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

#pragma once

#include <string>
#include <vector>
#include <unordered_map>
#include <unordered_set>

class WordNetLemmatizer {
public:
    explicit
    WordNetLemmatizer(const std::string &wordnet_path);

    ~WordNetLemmatizer();

    int32_t Load();

    std::string Lemmatize(const std::string &form, const std::string &pos = "");

private:
    int32_t LoadLemmas();

    void LoadExceptions();

    std::vector<std::string> Morphy(const std::string &form, const std::string &pos, bool check_exceptions = true);

    std::vector<std::string> CollectSubstitutions(const std::vector<std::string> &forms, const std::string &pos);
    std::vector<std::string> CollectSubstitutions(const std::string &form, const std::string &pos);

    std::vector<std::string> FilterForms(const std::vector<std::string> &forms, const std::string &pos);

    std::string wordnet_path_;

    std::unordered_map<std::string, std::unordered_set<std::string>> lemmas_;
    std::unordered_map<std::string, std::unordered_map<std::string, std::vector<std::string>>> exceptions_;
    std::unordered_map<std::string, std::vector<std::pair<std::string, std::string>>> MORPHOLOGICAL_SUBSTITUTIONS;
    std::vector<std::string> POS_LIST;
    std::unordered_map<std::string, std::string> file_map_;
};
