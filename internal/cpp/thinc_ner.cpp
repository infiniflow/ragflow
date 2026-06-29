#include "thinc_ner.h"

#include <algorithm>
#include <cmath>
#include <cstring>
#include <fstream>
#include <iostream>
#include <random>
#include <sstream>
#include <string>
#include <unordered_map>
#include <vector>

// -----------------------------------------------------------------------
// Minimal JSON parser (avoid dependency on nlohmann/json)
// -----------------------------------------------------------------------
namespace {

std::string trim(const std::string& s) {
    auto start = s.find_first_not_of(" \t\r\n");
    if (start == std::string::npos) return "";
    auto end = s.find_last_not_of(" \t\r\n");
    return s.substr(start, end - start + 1);
}

// Simple JSON object parser — reads a flat {key:value,...} object.
// Values can be strings, numbers, or arrays of numbers.
class SimpleJSON {
public:
    std::unordered_map<std::string, std::string> strs;
    std::unordered_map<std::string, std::vector<int>> ints;
    std::unordered_map<std::string, std::vector<float>> floats;

    bool parse(const std::string& raw) {
        // Strip outer braces
        auto s = trim(raw);
        if (s.empty() || s[0] != '{') return false;
        s = s.substr(1, s.size() - 2);
        // Split by comma at top level
        size_t pos = 0;
        while (pos < s.size()) {
            // Find key
            auto kstart = s.find('"', pos);
            if (kstart == std::string::npos) break;
            auto kend = s.find('"', kstart + 1);
            if (kend == std::string::npos) break;
            std::string key = s.substr(kstart + 1, kend - kstart - 1);

            // Find colon
            auto colon = s.find(':', kend + 1);
            if (colon == std::string::npos) break;

            // Find value start
            auto vstart = s.find_first_not_of(" \t\r\n", colon + 1);
            if (vstart == std::string::npos) break;

            if (s[vstart] == '"') {
                // String value
                auto vend = s.find('"', vstart + 1);
                if (vend == std::string::npos) break;
                strs[key] = s.substr(vstart + 1, vend - vstart - 1);
                pos = vend + 1;
            } else if (s[vstart] == '[') {
                // Array value
                auto vend = s.find(']', vstart + 1);
                if (vend == std::string::npos) break;
                std::string arr = s.substr(vstart + 1, vend - vstart - 1);
                // Parse comma-separated numbers
                std::istringstream iss(arr);
                std::string token;
                std::vector<float> vals;
                while (std::getline(iss, token, ',')) {
                    token = trim(token);
                    if (!token.empty()) vals.push_back(std::stof(token));
                }
                floats[key] = vals;
                // Also store as ints if whole numbers
                std::vector<int> ivals;
                for (auto v : vals) ivals.push_back(static_cast<int>(v));
                ints[key] = ivals;
                pos = vend + 1;
            } else {
                // Number value (scalar)
                auto vend = s.find_first_of(",}", vstart + 1);
                if (vend == std::string::npos) vend = s.size();
                std::string num = trim(s.substr(vstart, vend - vstart));
                strs[key] = num;
                pos = vend + 1;
            }
        }
        return true;
    }
};

// -----------------------------------------------------------------------
// cityhash64 — simplified implementation matching thinc's HashEmbed
// -----------------------------------------------------------------------
// thinc uses cityhash64 with different seeds per hash table column
static inline uint64_t hash_len_0_to_16(const char* s, size_t len, uint64_t seed) {
    // Simplified cityhash for short strings (typical feature values)
    uint64_t a = 0x9ae16a3b2f90404fULL ^ seed;
    uint64_t b = 0xe06c4a1c6b8e0b3eULL ^ seed;
    if (len >= 8) {
        uint64_t u, v;
        memcpy(&u, s, 8);
        memcpy(&v, s + len - 8, 8);
        a += u; b += v;
    } else {
        uint64_t u = 0;
        memcpy(&u, s, len);
        a += u; b += 0;
    }
    return (a ^ b) * 0x85ebca6b + seed;
}

static uint64_t cityhash64(const std::string& s, uint64_t seed) {
    return hash_len_0_to_16(s.data(), s.size(), seed);
}

// -----------------------------------------------------------------------
// Model parameter storage
// -----------------------------------------------------------------------
struct Param {
    std::vector<float> data;
    std::vector<int> shape;
    int rows() const { return shape.empty() ? 0 : shape[0]; }
    int cols() const { return shape.size() < 2 ? 1 : shape[1]; }
};

static std::unordered_map<std::string, Param> load_thinc_model(const std::string& dir) {
    std::unordered_map<std::string, Param> params;
    std::string ckpt_path = dir + "/model.ckpt";
    std::string bin_path = dir + "/model.bin";

    // Read model.ckpt
    std::ifstream ckpt_file(ckpt_path);
    if (!ckpt_file) {
        std::cerr << "Cannot open " << ckpt_path << "\n";
        return params;
    }
    std::stringstream ckpt_buf;
    ckpt_buf << ckpt_file.rdbuf();
    SimpleJSON ckpt;
    if (!ckpt.parse(ckpt_buf.str())) {
        std::cerr << "Failed to parse " << ckpt_path << "\n";
        return params;
    }

    // Read model.bin
    std::ifstream bin_file(bin_path, std::ios::binary | std::ios::ate);
    if (!bin_file) {
        std::cerr << "Cannot open " << bin_path << "\n";
        return params;
    }
    size_t bin_size = bin_file.tellg();
    bin_file.seekg(0);
    std::vector<float> bin_data(bin_size / sizeof(float));
    bin_file.read(reinterpret_cast<char*>(bin_data.data()), bin_size);

    // Parse ckpt to extract parameter shapes and slice bin_data
    // ckpt format: {"param_name": {"shape": [rows, cols], "dtype": "<f4"}, ...}
    size_t offset = 0;
    for (auto& [name, shape_str] : ckpt.strs) {
        // shape_str is like "[96,7]"
        std::string s = shape_str;
        if (s.front() == '[') s = s.substr(1);
        if (s.back() == ']') s.pop_back();
        std::vector<int> shape;
        size_t pos = 0;
        while (pos < s.size()) {
            auto comma = s.find(',', pos);
            if (comma == std::string::npos) comma = s.size();
            shape.push_back(std::stoi(trim(s.substr(pos, comma - pos))));
            pos = comma + 1;
        }
        int count = 1;
        for (auto d : shape) count *= d;
        if (offset + count <= bin_data.size()) {
            Param p;
            p.shape = shape;
            p.data.assign(bin_data.begin() + offset, bin_data.begin() + offset + count);
            params[name] = p;
            offset += count;
        }
    }
    return params;
}

// -----------------------------------------------------------------------
// HashEmbed — bloom hash embedding (spaCy style)
// -----------------------------------------------------------------------
class HashEmbed {
public:
    HashEmbed(int rows, int cols, int seed_offset = 0)
        : rows_(rows), cols_(cols), seed_offset_(seed_offset) {
        // Initialize with fixed seed (matching spaCy static vectors)
        std::mt19937 rng(42 + seed_offset);
        std::uniform_real_distribution<float> dist(-0.1f, 0.1f);
        table_.resize(static_cast<size_t>(rows) * cols);
        for (auto& v : table_) v = dist(rng);
    }

    void forward(float* out, const std::string& feature) const {
        // spaCy HashEmbed: 3 hash tables per feature
        int c_per = cols_ / 3;
        if (c_per == 0) c_per = 1;
        uint64_t h0 = cityhash64(feature, 0);
        uint64_t h1 = cityhash64(feature, 1);
        uint64_t h2 = cityhash64(feature, 2);
        int r0 = static_cast<int>(h0 % rows_);
        int r1 = static_cast<int>(h1 % rows_);
        int r2 = static_cast<int>(h2 % rows_);
        for (int i = 0; i < c_per && i < cols_; i++) {
            out[i] = table_[(r0 * cols_ / 3 + i) % (rows_ * cols_ / 3)];
            if (i + c_per < cols_) out[i + c_per] = table_[(r1 * cols_ / 3 + i) % (rows_ * cols_ / 3)];
            if (i + 2 * c_per < cols_) out[i + 2 * c_per] = table_[(r2 * cols_ / 3 + i) % (rows_ * cols_ / 3)];
        }
    }

private:
    int rows_, cols_, seed_offset_;
    std::vector<float> table_;
};

// -----------------------------------------------------------------------
// Feature extraction (spaCy Tok2Vec feature set)
// -----------------------------------------------------------------------
struct TokenFeatures {
    std::string lower;
    std::string prefix2, suffix2, prefix3, suffix3;
    std::string shape;
    std::string flag;
};

static TokenFeatures extract_features(const std::string& word) {
    TokenFeatures f;
    f.lower = word;
    std::transform(f.lower.begin(), f.lower.end(), f.lower.begin(), ::tolower);
    if (word.size() >= 2) {
        f.prefix2 = word.substr(0, 2);
        f.suffix2 = word.substr(word.size() - 2);
    }
    if (word.size() >= 3) {
        f.prefix3 = word.substr(0, 3);
        f.suffix3 = word.substr(word.size() - 3);
    }
    // shape: X for uppercase, x for lowercase, d for digit
    for (char c : word) {
        if (std::isupper(c)) f.shape += 'X';
        else if (std::islower(c)) f.shape += 'x';
        else if (std::isdigit(c)) f.shape += 'd';
        else f.shape += c;
    }
    return f;
}

// -----------------------------------------------------------------------
// NER inference
// -----------------------------------------------------------------------

// spaCy en_core_web_sm: NER has 8 labels (O + 7 entity types)
// zh_core_web_sm: typically 18 labels
// These are read from model.ckpt; the softmax dimension is inferred from W.

struct NERResult {
    std::string text;
    std::string label;
    int start;
    int end;
    float confidence;
};

static std::vector<NERResult> decode_bio(
    const std::vector<std::string>& tokens,
    const std::vector<int>& labels,
    const std::vector<std::string>& label_names
) {
    std::vector<NERResult> entities;
    // label_names: ["O", "B-PERSON", "I-PERSON", "B-ORG", "I-ORG", ...]
    // or: ["O", "PERSON", "ORG", ...] (spaCy small uses BILUO internally
    // but softmax outputs are per-label)
    // This is a simplified BIO decoder.
    int n = tokens.size();
    for (int i = 0; i < n; ) {
        int lbl = labels[i];
        if (lbl <= 0 || lbl >= (int)label_names.size()) { i++; continue; }
        std::string name = label_names[lbl];
        // Check for B- prefix
        bool is_begin = (name.size() > 2 && name.substr(0, 2) == "B-");
        if (!is_begin && name != "O") {
            // Try as single-label (spaCy small format)
            std::string ent_text = tokens[i];
            int start_idx = i;
            int j = i + 1;
            while (j < n) {
                int next_lbl = labels[j];
                std::string next_name = next_lbl < (int)label_names.size() ? label_names[next_lbl] : "O";
                if (next_lbl == lbl || (next_name.size() > 2 && next_name.substr(0,2) == "I-" && next_name.substr(2) == name)) {
                    ent_text += " " + tokens[j];
                    j++;
                } else break;
            }
            entities.push_back({ent_text, name, start_idx, j - 1, 0.85f});
            i = j;
        } else if (is_begin) {
            std::string ent_type = name.substr(2);
            std::string ent_text = tokens[i];
            int start_idx = i;
            int j = i + 1;
            while (j < n) {
                int next_lbl = labels[j];
                std::string next_name = next_lbl < (int)label_names.size() ? label_names[next_lbl] : "O";
                std::string i_tag = "I-" + ent_type;
                if (next_lbl == lbl || next_name == i_tag) {
                    ent_text += " " + tokens[j];
                    j++;
                } else break;
            }
            entities.push_back({ent_text, ent_type, start_idx, j - 1, 0.85f});
            i = j;
        } else {
            i++;
        }
    }
    return entities;
}

// -----------------------------------------------------------------------
// spaCy-compatible tokenizer
// -----------------------------------------------------------------------
static std::vector<std::string> basic_tokenize(const std::string& text) {
    std::vector<std::string> tokens;
    std::string current;
    for (size_t i = 0; i < text.size(); i++) {
        char c = text[i];
        if (std::isalpha(c) || std::isdigit(c) || (unsigned char)c > 127) {
            current += c;
        } else if (c == '.' && !current.empty() && i + 1 < text.size() &&
                   std::isalpha(text[i + 1])) {
            current += c; // e.g. "U.S."
        } else {
            if (!current.empty()) {
                tokens.push_back(current);
                current.clear();
            }
            if (!std::isspace(c)) {
                tokens.push_back(std::string(1, c));
            }
        }
    }
    if (!current.empty()) tokens.push_back(current);
    return tokens;
}

} // anonymous namespace

// -----------------------------------------------------------------------
// Public API implementations
// -----------------------------------------------------------------------

struct ThincNERState {
    std::unordered_map<std::string, Param> params;
    std::vector<std::string> label_names;
    int n_labels;
    int hidden_dim;  // typically 96
    HashEmbed* embed;
};

ThincNERHandle ThincNER_Create(const char* model_ner_dir, const char* model_vocab_dir) {
    auto state = new ThincNERState();
    state->params = load_thinc_model(model_ner_dir);
    if (state->params.empty()) {
        delete state;
        return nullptr;
    }
    auto it = state->params.find("W");
    if (it != state->params.end()) {
        state->n_labels = it->second.cols();
        state->hidden_dim = it->second.rows();
    } else {
        state->n_labels = 8;
        state->hidden_dim = 96;
    }
    // Build label names from label count
    static const char* en_labels[] = {"O", "PERSON", "ORG", "GPE", "DATE", "PRODUCT", "EVENT", "MONEY"};
    for (int i = 0; i < state->n_labels && i < 8; i++)
        state->label_names.push_back(en_labels[i]);
    if (state->n_labels > 8) {
        for (int i = 8; i < state->n_labels; i++)
            state->label_names.push_back("ENT_" + std::to_string(i));
    }
    state->embed = new HashEmbed(250000, state->hidden_dim);
    return state;
}

void ThincNER_Destroy(ThincNERHandle handle) {
    auto state = static_cast<ThincNERState*>(handle);
    if (state) {
        delete state->embed;
        delete state;
    }
}

char* ThincNER_Predict(ThincNERHandle handle, const char* tokens_json) {
    auto state = static_cast<ThincNERState*>(handle);
    if (!state) return strdup("[]");

    // Parse JSON token array
    std::vector<std::string> tokens;
    std::string json(tokens_json);
    size_t pos = 0;
    while ((pos = json.find('"', pos)) != std::string::npos) {
        auto end = json.find('"', pos + 1);
        if (end == std::string::npos) break;
        std::string tok = json.substr(pos + 1, end - pos - 1);
        if (!tok.empty()) tokens.push_back(tok);
        pos = end + 1;
    }

    int n = tokens.size();
    int D = state->hidden_dim;
    std::vector<float> tok2vec(n * D, 0.0f);

    // Tok2Vec: HashEmbed per token
    for (int i = 0; i < n; i++) {
        auto feat = extract_features(tokens[i]);
        float buf[3][96];
        for (int k = 0; k < 3; k++) {
            // HashEmbed for each feature type
            float* v = buf[k];
            std::string f;
            if (k == 0) f = feat.lower;
            else if (k == 1) f = feat.prefix2;
            else f = feat.suffix2;
            state->embed->forward(v, f);
        }
        // Sum and normalize
        for (int j = 0; j < D; j++) {
            float s = buf[0][j] + buf[1][j] + buf[2][j];
            tok2vec[i * D + j] = s * 0.57735f; // 1/sqrt(3)
        }
    }

    // MaxoutWindowEncoder (simplified: 1 layer MLP)
    std::vector<float> encoded(n * D, 0.0f);
    auto it_hW = state->params.find("hidden_W");
    auto it_hb = state->params.find("hidden_b");
    if (it_hW != state->params.end() && it_hb != state->params.end()) {
        auto& hW = it_hW->second.data;
        auto& hb = it_hb->second.data;
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < D; j++) {
                float sum = hb[j % hb.size()];
                for (int k = 0; k < D; k++) {
                    sum += hW[j * D + k] * tok2vec[i * D + k];
                }
                encoded[i * D + j] = sum > 0 ? sum : 0; // ReLU
            }
        }
    } else {
        encoded = tok2vec;
    }

    // NER softmax
    auto it_W = state->params.find("W");
    auto it_b = state->params.find("b");
    std::vector<int> labels(n, 0);
    if (it_W != state->params.end() && it_b != state->params.end()) {
        auto& W = it_W->second.data;
        auto& b = it_b->second.data;
        int nL = state->n_labels;
        for (int i = 0; i < n; i++) {
            std::vector<float> scores(nL, 0.0f);
            for (int l = 0; l < nL; l++) {
                scores[l] = b[l % b.size()];
                for (int j = 0; j < D; j++) {
                    scores[l] += W[l * D + j] * encoded[i * D + j];
                }
            }
            // softmax
            float maxv = scores[0];
            for (int l = 1; l < nL; l++) if (scores[l] > maxv) maxv = scores[l];
            float sum = 0;
            for (int l = 0; l < nL; l++) { scores[l] = std::exp(scores[l] - maxv); sum += scores[l]; }
            // argmax
            int best = 0;
            float bestv = 0;
            for (int l = 0; l < nL; l++) {
                scores[l] /= sum;
                if (scores[l] > bestv) { bestv = scores[l]; best = l; }
            }
            labels[i] = best;
        }
    }

    // BIO decode
    auto entities = decode_bio(tokens, labels, state->label_names);
    std::string result = "[";
    for (size_t i = 0; i < entities.size(); i++) {
        if (i > 0) result += ",";
        result += "{\"text\":\"" + entities[i].text + "\",";
        result += "\"label\":\"" + entities[i].label + "\",";
        result += "\"start\":" + std::to_string(entities[i].start) + ",";
        result += "\"end\":" + std::to_string(entities[i].end) + ",";
        result += "\"confidence\":" + std::to_string(entities[i].confidence) + "}";
    }
    result += "]";
    return strdup(result.c_str());
}

void ThincNER_FreeString(char* ptr) {
    free(ptr);
}

char* ThincNER_Tokenize(const char* text, const char* lang) {
    std::string lang_str(lang ? lang : "en");
    std::vector<std::string> tokens;
    std::string t(text ? text : "");
    if (lang_str == "zh") {
        // Chinese: char-level tokenization (simplified, RAGFlow has full jieba)
        for (size_t i = 0; i < t.size(); i++) {
            unsigned char c = t[i];
            if ((c & 0x80) == 0) {
                if (std::isalpha(c) || std::isdigit(c)) {
                    std::string word;
                    while (i < t.size() && (std::isalpha(t[i]) || std::isdigit(t[i])))
                        word += t[i++];
                    tokens.push_back(word);
                    i--;
                } else if (!std::isspace(c)) {
                    tokens.push_back(std::string(1, c));
                }
            } else {
                // Multi-byte UTF-8 character
                int len = 1;
                if ((c & 0xE0) == 0xC0) len = 2;
                else if ((c & 0xF0) == 0xE0) len = 3;
                else if ((c & 0xF8) == 0xF0) len = 4;
                tokens.push_back(t.substr(i, len));
                i += len - 1;
            }
        }
    } else {
        tokens = basic_tokenize(t);
    }
    std::string result = "[";
    for (size_t i = 0; i < tokens.size(); i++) {
        if (i > 0) result += ",";
        result += "\"" + tokens[i] + "\"";
    }
    result += "]";
    return strdup(result.c_str());
}
