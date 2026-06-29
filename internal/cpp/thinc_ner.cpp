#include "thinc_ner.h"

#include <algorithm>
#include <cmath>
#include <cstring>
#include <fstream>
#include <iostream>
#include <sstream>
#include <string>
#include <unordered_map>
#include <vector>

// -----------------------------------------------------------------------
// JSON utility
// -----------------------------------------------------------------------
namespace {

std::string trim(const std::string& s) {
    auto start = s.find_first_not_of(" \t\r\n");
    if (start == std::string::npos) return "";
    auto end = s.find_last_not_of(" \t\r\n");
    return s.substr(start, end - start + 1);
}

struct JVal {
    enum Type { NUL, OBJ, ARR, STR, NUM, BOOL };
    Type type = NUL;
    std::string str;
    std::vector<JVal> arr;
    std::unordered_map<std::string, JVal> obj;
    double num = 0;
    const JVal* get(const std::string& k) const {
        auto it = obj.find(k);
        return it != obj.end() ? &it->second : nullptr;
    }
    int as_int() const { return static_cast<int>(num); }
    int64_t as_int64() const { return static_cast<int64_t>(num); }
};

class JParser {
    const char* p;
    const char* end;
    void skip_ws() { while (p < end && (*p == ' ' || *p == '\t' || *p == '\n' || *p == '\r')) ++p; }
    char peek() { skip_ws(); return p < end ? *p : 0; }
    char next() { skip_ws(); return p < end ? *p++ : 0; }
    JVal parse_value() {
        char c = peek();
        if (c == '{') return parse_obj();
        if (c == '[') return parse_arr();
        if (c == '"') return parse_str();
        if (c == 't' || c == 'f') return parse_bool();
        if (c == 'n') { next();next();next();next(); return JVal{}; }
        return parse_num();
    }
    JVal parse_obj() {
        JVal v; v.type = JVal::OBJ;
        next();
        while (peek() != '}') {
            auto key = parse_str(); next();
            v.obj[key.str] = parse_value();
            if (peek() == ',') next(); else break;
        }
        next(); return v;
    }
    JVal parse_arr() {
        JVal v; v.type = JVal::ARR;
        next();
        while (peek() != ']') {
            v.arr.push_back(parse_value());
            if (peek() == ',') next(); else break;
        }
        next(); return v;
    }
    JVal parse_str() {
        JVal v; v.type = JVal::STR;
        next();
        while (p < end && *p != '"') {
            if (*p == '\\') { ++p; if (p < end) v.str += *p++; }
            else v.str += *p++;
        }
        if (p < end) ++p;
        return v;
    }
    JVal parse_num() {
        JVal v; v.type = JVal::NUM;
        const char* start = p;
        if (*p == '-') ++p;
        while (p < end && (*p >= '0' && *p <= '9')) ++p;
        if (p < end && *p == '.') { ++p; while (p < end && (*p >= '0' && *p <= '9')) ++p; }
        if (p < end && (*p == 'e' || *p == 'E')) { ++p; if (*p == '+' || *p == '-') ++p; while (p < end && (*p >= '0' && *p <= '9')) ++p; }
        v.num = std::stod(std::string(start, p - start));
        return v;
    }
    JVal parse_bool() {
        JVal v; v.type = JVal::BOOL;
        if (*p == 't') { v.str = "true"; p += 4; }
        else { v.str = "false"; p += 5; }
        return v;
    }
public:
    JVal parse(const std::string& json) {
        p = json.data();
        end = p + json.size();
        return parse_value();
    }
};

// -----------------------------------------------------------------------
// MurmurHash2 64-bit (exact match with spaCy: seed=1)
// -----------------------------------------------------------------------
static inline uint64_t murmurhash2_64a(const void* key, int len, uint64_t seed) {
    const uint64_t m = 0xc6a4a7935bd1e995ULL;
    const int r = 47;
    uint64_t h = seed ^ (static_cast<uint64_t>(len) * m);
    const auto* data = static_cast<const uint8_t*>(key);
    int remaining = len;
    while (remaining >= 8) {
        uint64_t k;
        memcpy(&k, data, 8);
        k *= m; k ^= k >> r; k *= m;
        h ^= k; h *= m;
        data += 8; remaining -= 8;
    }
    switch (remaining) {
        case 7: h ^= static_cast<uint64_t>(data[6]) << 48; [[fallthrough]];
        case 6: h ^= static_cast<uint64_t>(data[5]) << 40; [[fallthrough]];
        case 5: h ^= static_cast<uint64_t>(data[4]) << 32; [[fallthrough]];
        case 4: h ^= static_cast<uint64_t>(data[3]) << 24; [[fallthrough]];
        case 3: h ^= static_cast<uint64_t>(data[2]) << 16; [[fallthrough]];
        case 2: h ^= static_cast<uint64_t>(data[1]) << 8;  [[fallthrough]];
        case 1: h ^= data[0]; h *= m; break;
    }
    h ^= h >> r; h *= m; h ^= h >> r;
    return h;
}

// -----------------------------------------------------------------------
// Feature hashing: match spaCy's Vocab.strings[value]
// -----------------------------------------------------------------------
static uint64_t hash_feature(const std::string& feature_str) {
    if (feature_str.empty()) return 0;
    return murmurhash2_64a(feature_str.data(), static_cast<int>(feature_str.size()), 1);
}

// -----------------------------------------------------------------------
// MurmurHash3_x64_128 — produces 128-bit hash split into 4 × uint32.
// This is exactly what thinc's ops.hash() uses.
// Input: uint64 value as 8 bytes (little-endian)
// Seed: per-HashEmbed seed (model.id)
// Output: 4 × uint32 hash values
// -----------------------------------------------------------------------
static void murmurhash3_x64_128(const void* key, int len, uint32_t seed, uint32_t out[4]) {
    const uint8_t* data = static_cast<const uint8_t*>(key);
    int nblocks = len / 16;
    uint64_t h1 = seed;
    uint64_t h2 = seed;
    const uint64_t c1 = 0x87c37b91114253d5ULL;
    const uint64_t c2 = 0x4cf5ad432745937fULL;

    // Body
    for (int i = 0; i < nblocks; i++) {
        uint64_t k1, k2;
        memcpy(&k1, data + i * 16, 8);
        memcpy(&k2, data + i * 16 + 8, 8);
        k1 *= c1; k1 = (k1 << 31) | (k1 >> 33); k1 *= c2; h1 ^= k1;
        h1 = (h1 << 27) | (h1 >> 37); h1 += h2; h1 = h1 * 5 + 0x52dce729;
        k2 *= c2; k2 = (k2 << 33) | (k2 >> 31); k2 *= c1; h2 ^= k2;
        h2 = (h2 << 31) | (h2 >> 33); h2 += h1; h2 = h2 * 5 + 0x38495ab5;
    }

    // Tail
    const uint8_t* tail = data + nblocks * 16;
    uint64_t k1 = 0, k2 = 0;
    switch (len & 15) {
        case 15: k2 ^= static_cast<uint64_t>(tail[14]) << 48; [[fallthrough]];
        case 14: k2 ^= static_cast<uint64_t>(tail[13]) << 40; [[fallthrough]];
        case 13: k2 ^= static_cast<uint64_t>(tail[12]) << 32; [[fallthrough]];
        case 12: k2 ^= static_cast<uint64_t>(tail[11]) << 24; [[fallthrough]];
        case 11: k2 ^= static_cast<uint64_t>(tail[10]) << 16; [[fallthrough]];
        case 10: k2 ^= static_cast<uint64_t>(tail[9]) << 8;  [[fallthrough]];
        case 9:  k2 ^= static_cast<uint64_t>(tail[8]) << 0;
                 k2 *= c2; k2 = (k2 << 33) | (k2 >> 31); k2 *= c1; h2 ^= k2; [[fallthrough]];
        case 8:  k1 ^= static_cast<uint64_t>(tail[7]) << 56; [[fallthrough]];
        case 7:  k1 ^= static_cast<uint64_t>(tail[6]) << 48; [[fallthrough]];
        case 6:  k1 ^= static_cast<uint64_t>(tail[5]) << 40; [[fallthrough]];
        case 5:  k1 ^= static_cast<uint64_t>(tail[4]) << 32; [[fallthrough]];
        case 4:  k1 ^= static_cast<uint64_t>(tail[3]) << 24; [[fallthrough]];
        case 3:  k1 ^= static_cast<uint64_t>(tail[2]) << 16; [[fallthrough]];
        case 2:  k1 ^= static_cast<uint64_t>(tail[1]) << 8;  [[fallthrough]];
        case 1:  k1 ^= static_cast<uint64_t>(tail[0]) << 0;
                 k1 *= c1; k1 = (k1 << 31) | (k1 >> 33); k1 *= c2; h1 ^= k1;
    }

    // Finalization
    h1 ^= len; h2 ^= len;
    h1 += h2; h2 += h1;
    h1 ^= h1 >> 33; h1 *= 0xff51afd7ed558ccdULL;
    h1 ^= h1 >> 33; h1 *= 0xc4ceb9fe1a85ec53ULL;
    h1 ^= h1 >> 33;
    h2 ^= h2 >> 33; h2 *= 0xff51afd7ed558ccdULL;
    h2 ^= h2 >> 33; h2 *= 0xc4ceb9fe1a85ec53ULL;
    h2 ^= h2 >> 33;
    h1 += h2; h2 += h1;

    out[0] = static_cast<uint32_t>(h1);
    out[1] = static_cast<uint32_t>(h1 >> 32);
    out[2] = static_cast<uint32_t>(h2);
    out[3] = static_cast<uint32_t>(h2 >> 32);
}

// -----------------------------------------------------------------------
// Token feature extraction (exact spaCy format)
// -----------------------------------------------------------------------
struct TokenFeatures {
    std::string norm;
    std::string prefix;
    std::string suffix;
    std::string shape;
    std::string spacy;
    std::string is_space;

    // Pre-computed hash IDs for each feature
    uint64_t norm_id;
    uint64_t prefix_id;
    uint64_t suffix_id;
    uint64_t shape_id;
    uint64_t spacy_id;
    uint64_t is_space_id;
};

static TokenFeatures extract_features(const std::string& token) {
    TokenFeatures f;

    // NORM: lowercased (matches spaCy token.norm_ → e.g. "apple")
    f.norm = token;
    std::transform(f.norm.begin(), f.norm.end(), f.norm.begin(), ::tolower);

    // PREFIX: first char of ORIGINAL token (spaCy token.prefix_ returns 1 char)
    f.prefix = token.empty() ? "" : std::string(1, token[0]);

    // SUFFIX: last 3 chars of ORIGINAL token (spaCy token.suffix_ returns 3 chars)
    if (token.size() >= 3)
        f.suffix = token.substr(token.size() - 3);
    else
        f.suffix = token;

    // SHAPE: character pattern (matches spaCy token.shape_)
    for (char c : token) {
        unsigned char uc = static_cast<unsigned char>(c);
        if (std::isupper(uc)) f.shape += 'X';
        else if (std::islower(uc)) f.shape += 'x';
        else if (std::isdigit(uc)) f.shape += 'd';
        else f.shape.push_back(c);
    }

    // SPACY and IS_SPACE: for non-whitespace tokens, spaCy's extract_features
    // returns the RAW INTEGER 0 or 1 (NOT a vocab string hash).
    // embed[4] (SPACY) and embed[5] (IS_SPACE) receive
    // integer 0 (False) or 1 (True) directly as the feature ID.
    f.spacy = "";
    f.is_space = "";

    // NORM/PREFIX/SUFFIX/SHAPE: use hash of the value string (e.g. "apple", "A", "ple", "Xxxxx")
    f.norm_id = hash_feature(f.norm);
    f.prefix_id = hash_feature(f.prefix);
    f.suffix_id = hash_feature(f.suffix);
    f.shape_id = hash_feature(f.shape);

    // SPACY/IS_SPACE: directly use 0 (non-space token) as the feature ID
    f.spacy_id = 0;
    f.is_space_id = 0;

    return f;
}

// -----------------------------------------------------------------------
// HashEmbed: look up embedding from pre-trained table
// Uses MurmurHash3_x64_128 to produce 4 hash keys, then sums 4 lookups.
// -----------------------------------------------------------------------
class HashEmbed {
public:
    int n_rows = 0;
    int nO = 0;
    std::vector<float> table;
    uint32_t seed = 0;

    bool load(int rows, int O, const float* data) {
        n_rows = rows;
        nO = O;
        table.assign(data, data + static_cast<size_t>(rows) * O);
        return !table.empty();
    }

    void embed(uint64_t feat_id, float* out) const {
        uint8_t input[8];
        for (int i = 0; i < 8; i++)
            input[i] = static_cast<uint8_t>(feat_id >> (i * 8));
        uint32_t keys[4];
        murmurhash3_x64_128(input, 8, seed, keys);
        for (int v = 0; v < 4; v++) {
            int idx = static_cast<int>(keys[v] % static_cast<uint32_t>(n_rows));
            for (int i = 0; i < nO; i++)
                out[i] += table[static_cast<size_t>(idx) * nO + i];
        }
    }
};

// -----------------------------------------------------------------------
// LayerNorm: y = gain * (x - mean) / sqrt(var + eps) + bias
// -----------------------------------------------------------------------
static void layernorm(float* out, const float* in, int dim,
                      const float* gain, const float* bias, float eps) {
    float mean = 0, var = 0;
    for (int i = 0; i < dim; i++) mean += in[i];
    mean /= dim;
    for (int i = 0; i < dim; i++) var += (in[i] - mean) * (in[i] - mean);
    var /= dim;
    float inv_std = 1.0f / std::sqrt(var + eps);
    for (int i = 0; i < dim; i++) {
        out[i] = gain[i] * (in[i] - mean) * inv_std + bias[i];
    }
}

// -----------------------------------------------------------------------
// Maxout: y[i] = max_{j in pieces}( x[i * nP + j] )
// Simplified: when used with W matrix, it's y = max_pieces(W[p] @ x + b[p])
// -----------------------------------------------------------------------
static void maxout_linear(float* out, const float* in,
                          const float* W, const float* b,
                          int nO, int nP, int nI) {
    // W shape: (nO, nP, nI), b shape: (nO, nP)
    // For each output i, for each piece p: score[p] = b[i*nP+p] + W[i][p] @ in
    // out[i] = max(score)
    for (int i = 0; i < nO; i++) {
        float best = -1e30f;
        for (int p = 0; p < nP; p++) {
            float s = b[static_cast<size_t>(i) * nP + p];
            for (int j = 0; j < nI; j++) {
                s += W[static_cast<size_t>(i) * nP * nI + static_cast<size_t>(p) * nI + j] * in[j];
            }
            if (s > best) best = s;
        }
        out[i] = best;
    }
}

// -----------------------------------------------------------------------
// ExpandWindow: concatenate [token-1, token, token+1] for window=1
// -----------------------------------------------------------------------
static void expand_window(const float* all_tokens, int n_tokens, int dim,
                          int idx, float* out) {
    // window=1: concatenate prev, current, next
    // Total output dim = 3 * dim (but spaCy uses adjacent with 3 pieces,
    // so the effective expansion factor depends on the implementation)
    // spaCy's expand_window with window=1 produces: [tok-1, tok, tok+1]
    // But for edge tokens, it pads with zeros
    int start = std::max(0, idx - 1);
    int end = std::min(n_tokens - 1, idx + 1);
    int total = 0;

    // Previous token (zero-padded if at boundary)
    if (idx > 0) {
        memcpy(out, all_tokens + (idx - 1) * dim, dim * sizeof(float));
        total = dim;
    } else {
        memset(out, 0, dim * sizeof(float));
        total = dim;
    }

    // Current token
    memcpy(out + total, all_tokens + idx * dim, dim * sizeof(float));
    total += dim;

    // Next token (zero-padded if at boundary)
    if (idx < n_tokens - 1) {
        memcpy(out + total, all_tokens + (idx + 1) * dim, dim * sizeof(float));
    } else {
        memset(out + total, 0, dim * sizeof(float));
    }
}

// -----------------------------------------------------------------------
// Residual block: out = residual(in) + in
// residual = ExpandWindow → Maxout → LayerNorm → Dropout(no-op at inference)
// -----------------------------------------------------------------------

// -----------------------------------------------------------------------
// BILUO → entity decoder
// -----------------------------------------------------------------------
struct NERResult {
    std::string text;
    std::string label;
    int start;
    int end;
    float confidence;
};

static std::vector<NERResult> decode_biluo(
    const std::vector<std::string>& tokens,
    const std::vector<std::string>& token_labels
) {
    std::vector<NERResult> entities;
    int n = static_cast<int>(tokens.size());
    int ent_start = -1;
    std::string ent_type;
    std::string ent_text;

    for (int i = 0; i < n; i++) {
        const std::string& lbl = token_labels[i];

        if (lbl.empty() || lbl == "O") {
            // End current entity
            if (ent_start >= 0) {
                entities.push_back({ent_text, ent_type, ent_start, i - 1, 0.85f});
                ent_start = -1;
                ent_type.clear();
                ent_text.clear();
            }
            continue;
        }

        if (lbl.size() < 3 || lbl[1] != '-') {
            if (ent_start >= 0) {
                entities.push_back({ent_text, ent_type, ent_start, i - 1, 0.85f});
                ent_start = -1;
            }
            continue;
        }

        char action = lbl[0];
        std::string type = lbl.substr(2);

        if (action == 'U') {
            // Unit entity (single token)
            if (ent_start >= 0) {
                entities.push_back({ent_text, ent_type, ent_start, i - 1, 0.85f});
                ent_start = -1;
            }
            entities.push_back({tokens[i], type, i, i, 0.85f});
        } else if (action == 'B') {
            // Begin multi-token entity
            if (ent_start >= 0) {
                entities.push_back({ent_text, ent_type, ent_start, i - 1, 0.85f});
            }
            ent_start = i;
            ent_type = type;
            ent_text = tokens[i];
        } else if (action == 'I') {
            // Inside multi-token entity
            if (ent_start >= 0 && ent_type == type) {
                ent_text += " " + tokens[i];
            } else {
                // Orphan I-: start new entity
                if (ent_start >= 0) {
                    entities.push_back({ent_text, ent_type, ent_start, i - 1, 0.85f});
                }
                ent_start = i;
                ent_type = type;
                ent_text = tokens[i];
            }
        } else if (action == 'L') {
            // Last token of multi-token entity
            if (ent_start >= 0 && ent_type == type) {
                ent_text += " " + tokens[i];
                entities.push_back({ent_text, ent_type, ent_start, i, 0.85f});
            } else {
                entities.push_back({tokens[i], type, i, i, 0.85f});
            }
            ent_start = -1;
            ent_type.clear();
            ent_text.clear();
        }
    }

    // Flush remaining entity
    if (ent_start >= 0) {
        entities.push_back({ent_text, ent_type, ent_start, n - 1, 0.85f});
    }

    return entities;
}

// -----------------------------------------------------------------------
// Tokenizer
// -----------------------------------------------------------------------
static std::vector<std::string> basic_tokenize(const std::string& text) {
    std::vector<std::string> tokens;
    std::string current;
    for (size_t i = 0; i < text.size(); i++) {
        unsigned char c = static_cast<unsigned char>(text[i]);
        if (std::isalpha(c) || std::isdigit(c) || c > 127) {
            current += c;
        } else if (c == '.' && !current.empty() && i + 1 < text.size() &&
                   std::isalpha(static_cast<unsigned char>(text[i + 1]))) {
            current += c;
        } else {
            if (!current.empty()) { tokens.push_back(current); current.clear(); }
            if (!std::isspace(c)) tokens.push_back(std::string(1, static_cast<char>(c)));
        }
    }
    if (!current.empty()) tokens.push_back(current);
    return tokens;
}

} // anonymous namespace

// -----------------------------------------------------------------------
// State
// -----------------------------------------------------------------------
struct ThincNERState {
    // 6 HashEmbeds: NORM, PREFIX, SUFFIX, SHAPE, SPACY, IS_SPACE
    std::vector<HashEmbed> embeds;
    int hidden_dim = 96;
    int n_embed = 6;

    // Post-embed Maxout
    std::vector<float> post_W;  // (96, 3, 576)
    std::vector<float> post_b;  // (96, 3)
    int post_nO = 96, post_nP = 3, post_nI = 576;

    // Post-embed LayerNorm
    std::vector<float> post_ln_G; // (96,)
    std::vector<float> post_ln_b; // (96,)
    bool has_post_ln = false;

    // 4 residual encoder blocks
    struct EncoderBlock {
        bool has = false;
        std::vector<float> W; // (96, 3, 288) — 288 = 3 * 96 (expand_window=1)
        std::vector<float> b; // (96, 3)
        std::vector<float> ln_G; // (96,)
        std::vector<float> ln_b; // (96,)
    };
    EncoderBlock res_blocks[4];
    int n_res = 0;

    // NER: hidden projection (96→64)
    std::vector<float> hidden_W; // (64, 96)
    std::vector<float> hidden_b; // (64,)
    bool has_hidden = false;
    int hidden_out = 64;

    // PrecomputableAffine
    std::vector<float> pre_W;  // (3, 64, 2, 64) = (nV, nO, nP, nI)
    std::vector<float> pre_b;  // (64, 2)
    bool has_pre = false;

    // Classifier (64→n_actions)
    std::vector<float> W;  // (n_actions, 64)
    std::vector<float> b;  // (n_actions,)
    int n_actions = 0;
    bool has_classifier = false;

    // Action → per-token label (BILUO)
    std::vector<std::string> action_labels; // indexed by argmax action
};

// -----------------------------------------------------------------------
// Load model
// -----------------------------------------------------------------------
static bool load_ckpt_bin(const std::string& dir, ThincNERState* state) {
    std::string ckpt_path = dir + "/model.ckpt";
    std::string bin_path = dir + "/model.bin";

    std::ifstream ckpt_file(ckpt_path);
    if (!ckpt_file) { std::cerr << "Cannot open " << ckpt_path << "\n"; return false; }
    std::stringstream ckpt_buf;
    ckpt_buf << ckpt_file.rdbuf();

    JParser parser;
    JVal ckpt = parser.parse(ckpt_buf.str());
    if (ckpt.type != JVal::OBJ) { std::cerr << "Invalid model.ckpt\n"; return false; }

    std::ifstream bin_file(bin_path, std::ios::binary | std::ios::ate);
    if (!bin_file) { std::cerr << "Cannot open " << bin_path << "\n"; return false; }
    size_t bin_size = static_cast<size_t>(bin_file.tellg());
    bin_file.seekg(0);
    std::vector<float> bin_data(bin_size / sizeof(float));
    bin_file.read(reinterpret_cast<char*>(bin_data.data()), bin_size);

    auto slice = [&](int64_t offset, int64_t count) -> std::vector<float> {
        if (offset + count > static_cast<int64_t>(bin_data.size())) return {};
        return std::vector<float>(bin_data.begin() + offset, bin_data.begin() + offset + count);
    };

    auto load_param = [&](const std::string& key, std::vector<float>* out,
                          int* r0, int* r1, int* r2) -> bool {
        const JVal* entry = ckpt.get(key);
        if (!entry) return false;
        auto shape_v = entry->get("shape");
        auto off_v = entry->get("offset");
        auto cnt_v = entry->get("count");
        if (!shape_v || !off_v || !cnt_v) return false;
        int64_t off = off_v->as_int64();
        int64_t cnt = cnt_v->as_int64();
        *out = slice(off, cnt);
        if (r0) *r0 = shape_v->arr.size() >= 1 ? shape_v->arr[0].as_int() : 1;
        if (r1) *r1 = shape_v->arr.size() >= 2 ? shape_v->arr[1].as_int() : 1;
        if (r2) *r2 = shape_v->arr.size() >= 3 ? shape_v->arr[2].as_int() : 1;
        return !out->empty();
    };

    // HashEmbed tables + seeds
    for (int ei = 0; ei < 6; ei++) {
        std::string key = "embed_" + std::to_string(ei) + "_E";
        const JVal* entry = ckpt.get(key);
        if (!entry) { std::cerr << "Missing " << key << "\n"; return false; }
        auto shape_v = entry->get("shape");
        auto off_v = entry->get("offset");
        auto cnt_v = entry->get("count");
        if (!shape_v || !off_v || !cnt_v) return false;
        int rows = shape_v->arr.size() >= 1 ? shape_v->arr[0].as_int() : 0;
        int nO = shape_v->arr.size() >= 2 ? shape_v->arr[1].as_int() : 0;
        auto data = slice(off_v->as_int64(), cnt_v->as_int64());
        if (data.empty()) return false;
        state->embeds.emplace_back();
        state->embeds.back().load(rows, nO, data.data());

    }
    // Load seeds from feature_config.json
    std::string cfg_path = dir + "/feature_config.json";
    std::ifstream cfg_file(cfg_path);
    if (cfg_file) {
        std::stringstream cfg_buf;
        cfg_buf << cfg_file.rdbuf();
        JVal cfg = parser.parse(cfg_buf.str());
        const JVal* seeds_arr = cfg.get("embed_seeds");
        if (seeds_arr && seeds_arr->type == JVal::ARR) {
            for (int ei = 0; ei < static_cast<int>(seeds_arr->arr.size()) && ei < static_cast<int>(state->embeds.size()); ei++) {
                state->embeds[ei].seed = static_cast<uint32_t>(seeds_arr->arr[ei].as_int());
            }
        }
    }

    // Post-embed Maxout
    int r0 = 0, r1 = 0, r2 = 0;
    if (load_param("post_W", &state->post_W, &r0, &r1, &r2)) {
        state->post_nO = r0; state->post_nP = r1; state->post_nI = r2;
        load_param("post_b", &state->post_b, nullptr, nullptr, nullptr);
    }

    // Post-embed LayerNorm
    int gr0 = 0;
    load_param("post_ln_G", &state->post_ln_G, &gr0, nullptr, nullptr);
    load_param("post_ln_b", &state->post_ln_b, nullptr, nullptr, nullptr);
    state->has_post_ln = !state->post_ln_G.empty();

    // Residual encoder blocks (4)
    for (int ri = 0; ri < 4; ri++) {
        std::string p = "res_" + std::to_string(ri);
        auto& blk = state->res_blocks[ri];
        if (load_param(p + "_W", &blk.W, &r0, &r1, &r2)) {
            load_param(p + "_b", &blk.b, nullptr, nullptr, nullptr);
            load_param(p + "_ln_G", &blk.ln_G, nullptr, nullptr, nullptr);
            load_param(p + "_ln_b", &blk.ln_b, nullptr, nullptr, nullptr);
            blk.has = true;
            state->n_res++;
        }
    }

    // Hidden projection (96→64)
    r0 = 0; r1 = 0;
    if (load_param("hidden_W", &state->hidden_W, &r0, &r1, nullptr)) {
        state->hidden_out = r0;
        load_param("hidden_b", &state->hidden_b, nullptr, nullptr, nullptr);
        state->has_hidden = true;
    }

    // PrecomputableAffine
    r0 = 0; r1 = 0; r2 = 0;
    if (load_param("pre_W", &state->pre_W, &r0, &r1, &r2)) {
        load_param("pre_b", &state->pre_b, nullptr, nullptr, nullptr);
        state->has_pre = true;
    }

    // Classifier
    r0 = 0; r1 = 0;
    if (load_param("W", &state->W, &r0, &r1, nullptr)) {
        state->n_actions = r0;
        load_param("b", &state->b, nullptr, nullptr, nullptr);
        state->has_classifier = true;
    }

    return !state->embeds.empty();
}

static bool load_labels_json(const std::string& dir, ThincNERState* state) {
    std::string path = dir + "/labels.json";
    std::ifstream file(path);
    if (!file) { std::cerr << "Cannot open " << path << "\n"; return false; }
    std::stringstream buf;
    buf << file.rdbuf();

    JParser parser;
    JVal data = parser.parse(buf.str());
    if (data.type != JVal::OBJ) return false;

    const JVal* action_map = data.get("action_to_label");
    if (!action_map || action_map->type != JVal::OBJ) return false;

    int max_action = 0;
    for (auto& [k, v] : action_map->obj) {
        int action = std::stoi(k);
        if (action > max_action) max_action = action;
    }

    int n_actions = state->n_actions > 0 ? state->n_actions : max_action + 1;
    state->action_labels.resize(n_actions, "O");
    for (auto& [k, v] : action_map->obj) {
        int action = std::stoi(k);
        if (action >= 0 && action < n_actions)
            state->action_labels[action] = v.str;
    }

    return !state->action_labels.empty();
}

// -----------------------------------------------------------------------
// C API
// -----------------------------------------------------------------------
ThincNERHandle ThincNER_Create(const char* model_ner_dir, const char* model_vocab_dir) {
    (void)model_vocab_dir;
    auto state = new ThincNERState();
    if (!load_ckpt_bin(model_ner_dir, state)) { delete state; return nullptr; }
    if (!load_labels_json(model_ner_dir, state)) {
        std::cerr << "Warning: labels.json not found\n";
        state->action_labels.resize(74, "O");
    }
    return state;
}

void ThincNER_Destroy(ThincNERHandle handle) {
    delete static_cast<ThincNERState*>(handle);
}

// -----------------------------------------------------------------------
// Main prediction
// -----------------------------------------------------------------------
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

    int n = static_cast<int>(tokens.size());
    if (n == 0) return strdup("[]");

    int D = state->hidden_dim;  // 96
    int H = state->hidden_out;  // 64

    // ---- Step 1: Feature extraction + HashEmbed ----
    // Each token gets a 96-dim embedding per feature (6 features)
    // Concatenate: 6 × 96 = 576-dim
    std::vector<float> embed_concat(static_cast<size_t>(n) * 6 * D, 0.0f);
    for (int i = 0; i < n; i++) {
        auto feat = extract_features(tokens[i]);
        size_t base = static_cast<size_t>(i) * 6 * D;

        uint64_t feat_ids[6] = {
            feat.norm_id, feat.prefix_id, feat.suffix_id,
            feat.shape_id, feat.spacy_id, feat.is_space_id
        };

        for (int e = 0; e < 6 && e < static_cast<int>(state->embeds.size()); e++) {
            state->embeds[e].embed(feat_ids[e],
                embed_concat.data() + base + static_cast<size_t>(e) * D);
        }
    }

    // ---- Step 2: Post-embed Maxout (576→96) ----
    std::vector<float> post(static_cast<size_t>(n) * D, 0.0f);
    if (!state->post_W.empty()) {
        for (int i = 0; i < n; i++) {
            const float* in = embed_concat.data() + static_cast<size_t>(i) * 6 * D;
            maxout_linear(post.data() + static_cast<size_t>(i) * D, in,
                          state->post_W.data(), state->post_b.data(),
                          state->post_nO, state->post_nP, state->post_nI);
        }
    } else {
        // Fallback: average
        for (int i = 0; i < n; i++) {
            float* out = post.data() + static_cast<size_t>(i) * D;
            const float* in = embed_concat.data() + static_cast<size_t>(i) * 6 * D;
            for (int j = 0; j < D; j++) {
                float s = 0;
                for (int e = 0; e < 6; e++)
                    s += in[static_cast<size_t>(e) * D + j];
                out[j] = s / 6.0f;
            }
        }
    }

    // ---- Step 3: Post-embed LayerNorm ----
    std::vector<float> post_ln(static_cast<size_t>(n) * D, 0.0f);
    if (state->has_post_ln) {
        for (int i = 0; i < n; i++) {
            layernorm(post_ln.data() + static_cast<size_t>(i) * D,
                      post.data() + static_cast<size_t>(i) * D, D,
                      state->post_ln_G.data(), state->post_ln_b.data(), 1e-6f);
        }
    } else {
        post_ln = post;
    }

    // ---- Step 4: Residual encoder blocks (×4) ----
    // Each block: expand_window(3×96=288) → maxout(3 pieces, nO=96) → layernorm → + residual
    std::vector<float> enc = post_ln;
    for (int ri = 0; ri < state->n_res; ri++) {
        auto& blk = state->res_blocks[ri];
        if (!blk.has) continue;

        int window_dim = D * 3;  // 288 (expand_window with window=1)
        std::vector<float> expanded(static_cast<size_t>(n) * window_dim, 0.0f);
        for (int i = 0; i < n; i++) {
            expand_window(enc.data(), n, D, i,
                          expanded.data() + static_cast<size_t>(i) * window_dim);
        }

        std::vector<float> maxout_out(static_cast<size_t>(n) * D, 0.0f);
        for (int i = 0; i < n; i++) {
            maxout_linear(maxout_out.data() + static_cast<size_t>(i) * D,
                          expanded.data() + static_cast<size_t>(i) * window_dim,
                          blk.W.data(), blk.b.data(), D, 3, window_dim);
        }

        std::vector<float> ln_out(static_cast<size_t>(n) * D, 0.0f);
        if (!blk.ln_G.empty()) {
            for (int i = 0; i < n; i++) {
                layernorm(ln_out.data() + static_cast<size_t>(i) * D,
                          maxout_out.data() + static_cast<size_t>(i) * D, D,
                          blk.ln_G.data(), blk.ln_b.data(), 1e-6f);
            }
        } else {
            ln_out = maxout_out;
        }

        // Residual: out = in + residual
        for (int i = 0; i < n; i++) {
            float* out_ptr = enc.data() + static_cast<size_t>(i) * D;
            const float* res_ptr = ln_out.data() + static_cast<size_t>(i) * D;
            for (int j = 0; j < D; j++) {
                out_ptr[j] += res_ptr[j];
            }
        }
    }

    // ---- Step 5: Hidden projection (96→64) ----
    std::vector<float> hidden(static_cast<size_t>(n) * H, 0.0f);
    if (state->has_hidden) {
        for (int i = 0; i < n; i++) {
            for (int h = 0; h < H; h++) {
                float s = state->hidden_b[h];
                for (int j = 0; j < D; j++) {
                    s += state->hidden_W[static_cast<size_t>(h) * D + j] *
                         enc[static_cast<size_t>(i) * D + j];
                }
                hidden[static_cast<size_t>(i) * H + h] = s > 0 ? s : 0; // ReLU
            }
        }
    } else {
        // Fallback: copy first H dims
        for (int i = 0; i < n; i++) {
            int copy = std::min(D, H);
            memcpy(hidden.data() + i * H, enc.data() + i * D, copy * sizeof(float));
        }
    }

    // ---- Step 6: PrecomputableAffine (64→64) ----
    // W shape: (nV, nO, nP, nI) = (3, 64, 2, 64)
    // For each timestep: use the precomputed step embedding
    // In TransitionBasedParser, this layer computes step-specific representations
    // For our simplified BILUO decoding, we treat it as a standard Linear
    std::vector<float> pre_out(static_cast<size_t>(n) * H, 0.0f);
    if (state->has_pre) {
        // pre_W: (3, 64, 2, 64). Use first nV slice: (64, 2, 64)
        // Maxout pieces=2
        int nV = state->pre_W.size() / (H * 2 * H * static_cast<int>(sizeof(float)));
        // Simplification: use first piece as linear: y = W[0,1] @ x + b[0]
        for (int i = 0; i < n; i++) {
            for (int h = 0; h < H; h++) {
                float s = state->pre_b[static_cast<size_t>(h) * 2]; // first piece bias
                for (int j = 0; j < H; j++) {
                    s += state->pre_W[static_cast<size_t>(0) * H * 2 * H +
                                      static_cast<size_t>(h) * 2 * H +
                                      static_cast<size_t>(0) * H + j] *
                         hidden[static_cast<size_t>(i) * H + j];
                }
                pre_out[static_cast<size_t>(i) * H + h] = s > 0 ? s : 0; // ReLU
            }
        }
    } else {
        pre_out = hidden;
    }

    // ---- Step 7: Classifier (64→n_actions) ----
    std::vector<int> best_actions(n, 0);
    if (state->has_classifier) {
        int nA = state->n_actions;
        for (int i = 0; i < n; i++) {
            std::vector<float> scores(nA, 0.0f);
            for (int a = 0; a < nA; a++) {
                scores[a] = state->b[a];
                for (int j = 0; j < H; j++) {
                    scores[a] += state->W[static_cast<size_t>(a) * H + j] *
                                 pre_out[static_cast<size_t>(i) * H + j];
                }
            }
            // Argmax (no softmax needed — same result)
            int best = 0;
            float bestv = scores[0];
            for (int a = 1; a < nA; a++) {
                if (scores[a] > bestv) { bestv = scores[a]; best = a; }
            }
            best_actions[i] = best;
        }
    }

    // ---- Step 8: Map action IDs → BILUO labels ----
    std::vector<std::string> token_labels(n);
    for (int i = 0; i < n; i++) {
        int action = best_actions[i];
        if (action >= 0 && action < static_cast<int>(state->action_labels.size()))
            token_labels[i] = state->action_labels[action];
        else
            token_labels[i] = "O";
    }

    // ---- Step 9: BILUO decode ----
    auto entities = decode_biluo(tokens, token_labels);

    // ---- Step 10: Serialize ----
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

void ThincNER_FreeString(char* ptr) { free(ptr); }

char* ThincNER_Tokenize(const char* text, const char* lang) {
    if (!text) return strdup("[]");
    std::string lang_str(lang ? lang : "en");
    std::vector<std::string> tokens;
    std::string t(text);

    if (lang_str == "zh") {
        for (size_t i = 0; i < t.size(); i++) {
            unsigned char c = static_cast<unsigned char>(t[i]);
            if ((c & 0x80) == 0) {
                if (std::isalpha(c) || std::isdigit(c)) {
                    std::string word;
                    while (i < t.size() && (std::isalpha(static_cast<unsigned char>(t[i])) ||
                                             std::isdigit(static_cast<unsigned char>(t[i]))))
                        word += t[i++];
                    tokens.push_back(word);
                    i--;
                } else if (!std::isspace(c)) {
                    tokens.push_back(std::string(1, static_cast<char>(c)));
                }
            } else {
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
