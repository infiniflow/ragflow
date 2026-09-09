#pragma STDC FP_CONTRACT OFF

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

// =========================================================================
// JSON parser (minimal)
// =========================================================================
namespace {
std::string trim(const std::string& s) {
    auto a = s.find_first_not_of(" \t\r\n");
    return a == std::string::npos ? "" : s.substr(a, s.find_last_not_of(" \t\r\n")-a+1);
}
struct JVal; struct JObjMap;
struct JVal {
    enum Type {NUL,OBJ,ARR,STR,NUM,BOOL} type=NUL;
    std::string str; std::vector<JVal> arr; double num=0;
    JObjMap* obj = nullptr;
    JVal() = default;
    ~JVal();
    JVal(const JVal& o);
    JVal& operator=(const JVal& o);
    JVal(JVal&& o) noexcept;
    JVal& operator=(JVal&& o) noexcept;
    const JVal* get(const std::string& k) const;
    int as_int() const { return (int)num; } int64_t as_i64() const { return (int64_t)num; }
};
struct JObjMap { std::unordered_map<std::string, JVal> m; };
inline JVal::~JVal() { delete obj; }
inline JVal::JVal(const JVal& o) : type(o.type), str(o.str), arr(o.arr), num(o.num) { if (o.obj) obj = new JObjMap(*o.obj); }
inline JVal& JVal::operator=(const JVal& o) { if (this != &o) { delete obj; type=o.type; str=o.str; arr=o.arr; num=o.num; obj=o.obj ? new JObjMap(*o.obj) : nullptr; } return *this; }
inline JVal::JVal(JVal&& o) noexcept : type(o.type), str(std::move(o.str)), arr(std::move(o.arr)), num(o.num), obj(o.obj) { o.obj = nullptr; }
inline JVal& JVal::operator=(JVal&& o) noexcept { if (this != &o) { delete obj; type=o.type; str=std::move(o.str); arr=std::move(o.arr); num=o.num; obj=o.obj; o.obj=nullptr; } return *this; }
inline const JVal* JVal::get(const std::string& k) const { if (!obj) return nullptr; auto it=obj->m.find(k); return it!=obj->m.end()?&it->second:nullptr; }
struct JParser {
    const char *p,*e; char pk() { while(p<e&&(*p==' '||*p=='\t'||*p=='\n'||*p=='\r'))++p; return p<e?*p:0; }
    char nx() { while(p<e&&(*p==' '||*p=='\t'||*p=='\n'||*p=='\r'))++p; return p<e?*p++:0; }
    JVal pv() { char c=pk(); if(c=='{')return po(); if(c=='[')return pa(); if(c=='"')return ps(); if(c=='t'||c=='f')return pb();
        if(c=='n'){nx();nx();nx();nx();return JVal{};} return pn(); }
    JVal po() { JVal v;v.type=JVal::OBJ; nx(); if(!v.obj)v.obj=new JObjMap(); while(pk()!='}'){auto k=ps();nx();v.obj->m[k.str]=pv();if(pk()==',')nx();else break;}nx();return v; }
    JVal pa() { JVal v;v.type=JVal::ARR; nx(); while(pk()!=']'){v.arr.push_back(pv());if(pk()==',')nx();else break;}nx();return v; }
    JVal ps() { JVal v;v.type=JVal::STR; nx();while(p<e&&*p!='"'){if(*p=='\\'){++p;if(p<e)v.str+=*p++;}else v.str+=*p++;}if(p<e)++p;return v; }
    JVal pn() { JVal v;v.type=JVal::NUM; auto s=p; if(p<e&&*p=='-')++p; while(p<e&&(*p>='0'&&*p<='9'))++p;
        if(p<e&&*p=='.'){++p;while(p<e&&(*p>='0'&&*p<='9'))++p;}
        if(p<e&&(*p=='e'||*p=='E')){++p;if(p<e&&(*p=='+'||*p=='-'))++p;while(p<e&&(*p>='0'&&*p<='9'))++p;}
        if(s<p){try{v.num=std::stod(std::string(s,p-s));}catch(...){v.num=0;}} return v; }
    JVal pb() { JVal v;v.type=JVal::BOOL; if(e-p>=4&&*p=='t'){v.str="true";p+=4;}else if(e-p>=5&&*p=='f'){v.str="false";p+=5;} return v; }
    JVal parse(const std::string& j) { p=j.data(); e=p+j.size(); return pv(); }
};

// =========================================================================
// MurmurHash2 64-bit (vocab string→ID, seed=0 matching spaCy StringStore)
// =========================================================================
static uint64_t mh2_64a(const void* key, int len, uint64_t seed) {
    const uint64_t m=0xc6a4a7935bd1e995ULL; const int r=47;
    uint64_t h=seed^(uint64_t(len)*m); auto d=(const uint8_t*)key; int rm=len;
    while(rm>=8){uint64_t k;memcpy(&k,d,8);k*=m;k^=k>>r;k*=m;h^=k;h*=m;d+=8;rm-=8;}
    switch(rm){case 7:h^=uint64_t(d[6])<<48;case 6:h^=uint64_t(d[5])<<40;
    case 5:h^=uint64_t(d[4])<<32;case 4:h^=uint64_t(d[3])<<24;
    case 3:h^=uint64_t(d[2])<<16;case 2:h^=uint64_t(d[1])<<8;
    case 1:h^=d[0];h*=m;break;}
    h^=h>>r;h*=m;h^=h>>r; return h;
}
static uint64_t hash_feat(const std::string& s) { return s.empty()?0:mh2_64a(s.data(),(int)s.size(),0); }

// =========================================================================
// MurmurHash3_x64_128 (exact copy from mmh3 package, verified against thinc)
// =========================================================================
#define ROTL64(x,r) ((x << r) | (x >> (64 - r)))
static uint64_t getblock64(const uint64_t* p, size_t i) { uint64_t r; memcpy(&r, p+i, 8); return r; }
static uint64_t fmix64(uint64_t k) {
    k ^= k >> 33; k *= 0xff51afd7ed558ccdULL;
    k ^= k >> 33; k *= 0xc4ceb9fe1a85ec53ULL;
    k ^= k >> 33; return k;
}
static void mmh3_x64_128(const void* key, int len, uint32_t seed, uint32_t out[4]) {
    const uint8_t* data = (const uint8_t*)key;
    int nblocks = len / 16;
    uint64_t h1 = seed, h2 = seed;
    const uint64_t c1 = 0x87c37b91114253d5ULL;
    const uint64_t c2 = 0x4cf5ad432745937fULL;
    const uint64_t* blocks = (const uint64_t*)(data);
    for (int i = 0; i < nblocks; i++) {
        uint64_t k1 = getblock64(blocks, i*2+0);
        uint64_t k2 = getblock64(blocks, i*2+1);
        k1 *= c1; k1 = ROTL64(k1,31); k1 *= c2; h1 ^= k1;
        h1 = ROTL64(h1,27); h1 += h2; h1 = h1 * 5 + 0x52dce729;
        k2 *= c2; k2 = ROTL64(k2,33); k2 *= c1; h2 ^= k2;
        h2 = ROTL64(h2,31); h2 += h1; h2 = h2 * 5 + 0x38495ab5;
    }
    const uint8_t* tail = (const uint8_t*)(data + nblocks * 16);
    uint64_t k1 = 0, k2 = 0;
    switch (len & 15) {
        case 15: k2 ^= ((uint64_t)tail[14]) << 48;
        case 14: k2 ^= ((uint64_t)tail[13]) << 40;
        case 13: k2 ^= ((uint64_t)tail[12]) << 32;
        case 12: k2 ^= ((uint64_t)tail[11]) << 24;
        case 11: k2 ^= ((uint64_t)tail[10]) << 16;
        case 10: k2 ^= ((uint64_t)tail[9]) << 8;
        case  9: k2 ^= ((uint64_t)tail[8]) << 0;
                 k2 *= c2; k2 = ROTL64(k2,33); k2 *= c1; h2 ^= k2;
        case  8: k1 ^= ((uint64_t)tail[7]) << 56;
        case  7: k1 ^= ((uint64_t)tail[6]) << 48;
        case  6: k1 ^= ((uint64_t)tail[5]) << 40;
        case  5: k1 ^= ((uint64_t)tail[4]) << 32;
        case  4: k1 ^= ((uint64_t)tail[3]) << 24;
        case  3: k1 ^= ((uint64_t)tail[2]) << 16;
        case  2: k1 ^= ((uint64_t)tail[1]) << 8;
        case  1: k1 ^= ((uint64_t)tail[0]) << 0;
                 k1 *= c1; k1 = ROTL64(k1,31); k1 *= c2; h1 ^= k1;
    };
    h1 ^= len; h2 ^= len;
    h1 += h2; h2 += h1;
    h1 = fmix64(h1); h2 = fmix64(h2);
    h1 += h2; h2 += h1;
    out[0] = (uint32_t)h1; out[1] = (uint32_t)(h1>>32);
    out[2] = (uint32_t)h2; out[3] = (uint32_t)(h2>>32);
}

// =========================================================================
// HashEmbed
// =========================================================================
struct HashEmbed {
    int n_rows=0,nO=0; uint32_t seed=0; std::vector<float> table;
    bool load(int r, int o, const float* d) { n_rows=r;nO=o;table.assign(d,d+(size_t)r*o);return!table.empty(); }
    void embed(uint64_t fid, float* out) const {
        uint8_t in[8]; for(int i=0;i<8;i++)in[i]=(uint8_t)(fid>>(i*8));
        uint32_t keys[4]; mmh3_x64_128(in,8,seed,keys);
        for(int v=0;v<4;v++){int idx=(int)(keys[v]%(uint32_t)n_rows);for(int i=0;i<nO;i++)out[i]+=table[(size_t)idx*nO+i];}
    }
};

// =========================================================================
// Features — dynamic based on n_embed
//   en (6): NORM, PREFIX, SUFFIX, SHAPE, SPACY, IS_SPACE
//   zh (5): NORM, PREFIX, SUFFIX, SHAPE, IS_SPACE
// =========================================================================
static uint64_t feat_norm(const std::string& t) {
    std::string lo=t; std::transform(lo.begin(),lo.end(),lo.begin(),::tolower);
    return hash_feat(lo);
}
// UTF-8 aware: get first Unicode codepoint as string
static std::string utf8_first(const std::string& s) {
    if(s.empty()) return "";
    unsigned char c=(unsigned char)s[0];
    int l=1;
    if((c&0xE0)==0xC0) l=2;
    else if((c&0xF0)==0xE0) l=3;
    else if((c&0xF8)==0xF0) l=4;
    return s.substr(0,(size_t)l<=s.size()?l:1);
}
// Count Unicode codepoints in a string
static size_t utf8_len(const std::string& s) {
    size_t n=0;
    for(size_t i=0;i<s.size();n++){
        unsigned char c=(unsigned char)s[i];
        if((c&0x80)==0) i+=1;
        else if((c&0xE0)==0xC0) i+=2;
        else if((c&0xF0)==0xE0) i+=3;
        else if((c&0xF8)==0xF0) i+=4;
        else i+=1;
    }
    return n;
}
// Get suffix: last `count` Unicode codepoints
static std::string utf8_last(const std::string& s, size_t count) {
    size_t ulen=utf8_len(s);
    if(ulen<=count) return s;
    // Find byte position of the (ulen-count)-th codepoint
    size_t pos=0;
    for(size_t i=0;i<ulen-count;i++){
        unsigned char c=(unsigned char)s[pos];
        if((c&0x80)==0) pos+=1;
        else if((c&0xE0)==0xC0) pos+=2;
        else if((c&0xF0)==0xE0) pos+=3;
        else if((c&0xF8)==0xF0) pos+=4;
        else pos+=1;
    }
    return s.substr(pos);
}
static uint64_t feat_prefix(const std::string& t) {
    // spaCy: string[:1].lower() → hash the lowercased prefix
    std::string p = t.empty() ? "" : utf8_first(t);
    std::transform(p.begin(), p.end(), p.begin(), ::tolower);
    return hash_feat(p);
}
static uint64_t feat_suffix(const std::string& t) {
    // spaCy: string[-3:].lower() → hash the lowercased suffix
    size_t ulen=utf8_len(t);
    std::string s = ulen>=3 ? utf8_last(t,3) : t;
    std::transform(s.begin(), s.end(), s.begin(), ::tolower);
    return hash_feat(s);
}
static uint64_t feat_shape(const std::string& t) {
    std::string sh;
    for(unsigned char c:t){
        if(c>0x7F)sh+='x';                    // CJK → 'x' (matches spaCy zh shape)
        else if(std::isupper(c))sh+='X';
        else if(std::islower(c))sh+='x';
        else if(std::isdigit(c))sh+='d';
        else sh+=c;
    }
    return hash_feat(sh);
}
// Extract features based on n_embed count. Returns vector of hash values.
// NER model's tok2vec uses 4 features: NORM, PREFIX, SUFFIX, SHAPE
// (The pipeline's standalone tok2vec uses 6 features including SPACY and IS_SPACE.)
// Feature order matches the HashEmbed table order in the model.
static std::vector<uint64_t> extract_features(const std::string& t, int n_embed) {
    std::vector<uint64_t> ids;
    ids.push_back(feat_norm(t));     // #0: NORM (all models)
    ids.push_back(feat_prefix(t));   // #1: PREFIX
    ids.push_back(feat_suffix(t));   // #2: SUFFIX
    ids.push_back(feat_shape(t));    // #3: SHAPE
    if(n_embed==5) {
        ids.push_back(0);            // #4: IS_SPACE (zh/ja: 5-embed models, no SPACY)
    } else if(n_embed>=6) {
        ids.push_back(1);            // #4: SPACY (en/de/fr/es/pt: 6-embed models)
        ids.push_back(0);            // #5: IS_SPACE
    }
    return ids;
}

// =========================================================================
// Layers
// =========================================================================

// Kahan compensated dot product: reduces floating-point accumulation error
// for long dot products (e.g. 576 terms in Maxout).
static float kahan_dot(const float* a, const float* b, int n) {
    float sum = 0.0f;
    float c = 0.0f;
    for (int i = 0; i < n; i++) {
        float y = a[i] * b[i] - c;
        float t = sum + y;
        c = (t - sum) - y;
        sum = t;
    }
    return sum;
}

static void linear(float* out, const float* in, const float* W, const float* b, int nO, int nI) {
    for(int i=0;i<nO;i++)out[i]=b[i]+kahan_dot(W+(size_t)i*nI,in,nI);
}
static void relu_inplace(float* x, int n) { for(int i=0;i<n;i++)x[i]=x[i]>0?x[i]:0; }

// Maxout: y[i] = max_p(b[i,p] + W[i,p,:] @ in)
static void maxout(float* out, const float* in, const float* W, const float* b, int nO, int nP, int nI) {
    for(int i=0;i<nO;i++){
        float best=-1e30f;
        for(int p=0;p<nP;p++){
            float s = b[(size_t)i*nP+p] + kahan_dot(W+(((size_t)i*nP+p)*nI), in, nI);
            if(s>best)best=s;
        }
        out[i]=best;
    }
}

// LayerNorm: y = G * (x-mean)/sqrt(var+eps) + b
static void layernorm(float* out, const float* in, int d, const float* G, const float* b, float eps) {
    float mn=0,vr=0; for(int i=0;i<d;i++)mn+=in[i]; mn/=d;
    for(int i=0;i<d;i++)vr+=(in[i]-mn)*(in[i]-mn); vr/=d;
    float is=1.0f/sqrtf(vr+eps);
    for(int i=0;i<d;i++)out[i]=G[i]*(in[i]-mn)*is+b[i];
}

// ExpandWindow: for token at index `idx` over all_tokens[n_tokens×dim], produce [t-1, t, t+1]
static void expand_win(float* out, const float* all, int n, int dim, int idx) {
    int off = idx*dim;
    if(idx>0)memcpy(out,all+(idx-1)*dim,dim*sizeof(float)); else memset(out,0,dim*sizeof(float));
    memcpy(out+dim,all+off,dim*sizeof(float));
    if(idx<n-1)memcpy(out+2*dim,all+(idx+1)*dim,dim*sizeof(float)); else memset(out+2*dim,0,dim*sizeof(float));
}

// BILUO decoder
struct Entity { std::string text,label; int start,end; float conf; };
static std::vector<Entity> decode_biluo(const std::vector<std::string>& tok, const std::vector<std::string>& lbl) {
    std::vector<Entity> ents; int n=(int)tok.size(),st=-1; std::string et,ex;
    for(int i=0;i<n;i++){
        auto& l=lbl[i];
        if(l.empty()||l=="O"){if(st>=0){ents.push_back({ex,et,st,i-1,0.85f});st=-1;et.clear();ex.clear();}continue;}
        if(l.size()<3||l[1]!='-'){if(st>=0){ents.push_back({ex,et,st,i-1,0.85f});st=-1;}continue;}
        char a=l[0]; std::string ty=l.substr(2);
        if(a=='U'){if(st>=0){ents.push_back({ex,et,st,i-1,0.85f});st=-1;}ents.push_back({tok[i],ty,i,i,0.85f});}
        else if(a=='B'){if(st>=0)ents.push_back({ex,et,st,i-1,0.85f});st=i;et=ty;ex=tok[i];}
        else if(a=='I'){if(st>=0&&et==ty)ex+=" "+tok[i];else{if(st>=0)ents.push_back({ex,et,st,i-1,0.85f});st=i;et=ty;ex=tok[i];}}
        else if(a=='L'){if(st>=0&&et==ty){ex+=" "+tok[i];ents.push_back({ex,et,st,i,0.85f});}else ents.push_back({tok[i],ty,i,i,0.85f});st=-1;et.clear();ex.clear();}
    }
    if(st>=0)ents.push_back({ex,et,st,n-1,0.85f});
    return ents;
}

// Tokenizer
static std::vector<std::string> tokenize_en(const std::string& t) {
    std::vector<std::string> r; std::string cur;
    for(size_t i=0;i<t.size();i++){unsigned char c=(unsigned char)t[i];
        if(std::isalpha(c)||std::isdigit(c)||c>127)cur+=c;
        else if(c=='.'&&!cur.empty()&&i+1<t.size()&&std::isalpha((unsigned char)t[i+1]))cur+='.';
        else{if(!cur.empty()){r.push_back(cur);cur.clear();}if(!std::isspace(c))r.push_back(std::string(1,(char)c));}}
    if(!cur.empty())r.push_back(cur); return r;
}
static std::vector<std::string> tokenize_zh(const std::string& t) {
    std::vector<std::string> r;
    for(size_t i=0;i<t.size();i++){unsigned char c=(unsigned char)t[i];
        if((c&0x80)==0){if(std::isalpha(c)||std::isdigit(c)){std::string w;while(i<t.size()&&(std::isalpha((unsigned char)t[i])||std::isdigit((unsigned char)t[i])))w+=t[i++];r.push_back(w);i--;}else if(!std::isspace(c))r.push_back(std::string(1,(char)c));}
        else{int l=1;if((c&0xE0)==0xC0)l=2;else if((c&0xF0)==0xE0)l=3;else if((c&0xF8)==0xF0)l=4;r.push_back(t.substr(i,l));i+=l-1;}}
    return r;
}
} // namespace

// =========================================================================
// State
// =========================================================================
struct State {
    // HashEmbeds
    std::vector<HashEmbed> embeds;
    // Post-embed Maxout (576→96)
    std::vector<float> poW,poB; int po_nO=96,po_nP=3,po_nI=576;
    // Post-embed LayerNorm
    std::vector<float> poG,poB2; bool has_poLN=false;
    // Residual encoder (4 blocks)
    struct ResBlk{bool has=false;std::vector<float>W,b,lnG,lnb;};
    ResBlk res[4]; int n_res=0;
    // NER hidden (96→64)
    std::vector<float> hW,hB; int hO=64; bool has_hid=false;
    // PrecomputableAffine: W_full[nP=3][nO=64][nI=2][nD=64], b_full[nO=64][nI=2]
    // We use f=0 (first feature only): pre_out[p][o] = sum_d W[p][o][0][d] * hid[d] + b[o][0]
    std::vector<float> pW_full; // flattened [3*64*2*64]
    std::vector<float> pB_full; // flattened [64*2]
    int p_nP=3, p_nO=64, p_nI=2, p_nD=64; bool has_pre=false;
    // Classifier (64→n_actions)
    std::vector<float> cW,cB; int nAct=0; bool has_cls=false;
    std::vector<std::string> actLbl;
};

// =========================================================================
// Load model
// =========================================================================
static bool load(const std::string& dir, State* s) {
    std::ifstream cf(dir+"/model.ckpt"); if(!cf){std::cerr<<"No model.ckpt\n";return false;}
    std::stringstream cb;cb<<cf.rdbuf();
    JVal ck=JParser().parse(cb.str()); if(ck.type!=JVal::OBJ)return false;

    std::ifstream bf(dir+"/model.bin",std::ios::binary|std::ios::ate); if(!bf)return false;
    size_t bz=bf.tellg();bf.seekg(0); std::vector<float> bin(bz/4); bf.read((char*)bin.data(),bz);

    auto sl=[&](int64_t o, int64_t c)->std::vector<float>{
        if(o+c>(int64_t)bin.size())return{}; return std::vector<float>(bin.begin()+o,bin.begin()+o+c);
    };
    auto ld=[&](const std::string& k, std::vector<float>* v, int* r0=nullptr,int* r1=nullptr,int* r2=nullptr)->bool{
        auto* e=ck.get(k); if(!e)return false;
        auto sv=e->get("shape"),ov=e->get("offset"),cv=e->get("count");
        if(!sv||!ov||!cv)return false;
        *v=sl(ov->as_i64(),cv->as_i64());
        if(r0)*r0=sv->arr.size()>=1?sv->arr[0].as_int():1;
        if(r1)*r1=sv->arr.size()>=2?sv->arr[1].as_int():1;
        if(r2)*r2=sv->arr.size()>=3?sv->arr[2].as_int():1;
        return!v->empty();
    };

    // HashEmbeds — dynamic count (6 for en, 5 for zh, etc.)
    for(int ei=0;;ei++){
        auto* e=ck.get("embed_"+std::to_string(ei)+"_E"); if(!e)break;
        auto sv=e->get("shape"),ov=e->get("offset"),cv=e->get("count");
        if(!sv||!ov||!cv)break;
        int rs=sv->arr[0].as_int(),nO=sv->arr[1].as_int();
        int64_t expected=(int64_t)rs*nO;
        if(cv->as_i64()<expected)break; // count too short → malformed
        auto d=sl(ov->as_i64(),cv->as_i64()); if(d.empty())break;
        s->embeds.emplace_back(); s->embeds.back().load(rs,nO,d.data());
    }
    // Seeds
    std::ifstream ff(dir+"/feature_config.json");
    if(ff){std::stringstream fb;fb<<ff.rdbuf();auto cfg=JParser().parse(fb.str());auto* sa=cfg.get("embed_seeds");
        if(sa&&sa->type==JVal::ARR)for(int i=0;i<(int)sa->arr.size()&&i<(int)s->embeds.size();i++)s->embeds[i].seed=(uint32_t)sa->arr[i].as_int();}

    int r0=0,r1=0,r2=0;
    // Post-embed
    if(ld("poW",&s->poW,&r0,&r1,&r2)){s->po_nO=r0;s->po_nP=r1;s->po_nI=r2;ld("poB",&s->poB);}
    if(ld("poG",&s->poG)){ld("poB2",&s->poB2);s->has_poLN=true;}
    // Residual
    for(int ri=0;ri<4;ri++){auto pk="res"+std::to_string(ri);auto& rb=s->res[ri];
        if(ld(pk+"W",&rb.W,&r0,&r1,&r2)){ld(pk+"B",&rb.b);ld(pk+"lnG",&rb.lnG);ld(pk+"lnb",&rb.lnb);rb.has=true;s->n_res++;}}
    // NER hidden
    if(ld("hW",&s->hW,&r0,&r1)){s->hO=r0;ld("hB",&s->hB);s->has_hid=true;}
    // PrecomputableAffine: load full 4D W and 2D b
    // has_pre is only set when ALL of weight buffer, bias buffer,
    // and hidden-dimension match (p_nD == hO) are satisfied.
    {
        auto* e=ck.get("pW_full"); if(e){
            auto sv=e->get("shape"),ov=e->get("offset"),cv=e->get("count");
            if(sv&&ov&&cv){
                int nP=sv->arr.size()>=1?sv->arr[0].as_int():1;
                int nO=sv->arr.size()>=2?sv->arr[1].as_int():1;
                int nI=sv->arr.size()>=3?sv->arr[2].as_int():1;
                int nD=sv->arr.size()>=4?sv->arr[3].as_int():1;
                s->p_nP=nP; s->p_nO=nO; s->p_nI=nI; s->p_nD=nD;
                size_t total = (size_t)nP * nO * nI * nD;
                s->pW_full = sl(ov->as_i64(), cv->as_i64());
                bool pw_ok = s->pW_full.size() >= total;

                // Load bias inside pW_full block to access dimension info
                bool pb_ok = false;
                if(auto* pb_e=ck.get("pB_full")){
                    auto pb_ov=pb_e->get("offset"),pb_cv=pb_e->get("count");
                    if(pb_ov&&pb_cv){
                        s->pB_full = sl(pb_ov->as_i64(), pb_cv->as_i64());
                        pb_ok = s->pB_full.size() >= (size_t)nO * nI;
                    }
                }

                bool dim_ok = (nD == s->hO);
                s->has_pre = pw_ok && pb_ok && dim_ok;
            }
        }
    }
    // Classifier
    if(ld("cW",&s->cW,&r0,&r1)){s->nAct=r0;ld("cB",&s->cB);s->has_cls=true;}

    return!s->embeds.empty();
}

static bool load_labels(const std::string& dir, State* s) {
    std::ifstream f(dir+"/labels.json"); if(!f)return false;
    std::stringstream b;b<<f.rdbuf();
    auto d=JParser().parse(b.str()); auto* am=d.get("action_to_label");
    if(!am||am->type!=JVal::OBJ)return false;
    int mx=0; for(auto&[k,v]:am->obj->m){try{int a=std::stoi(k);if(a>mx)mx=a;}catch(...){}};
    int n=s->nAct>0?s->nAct:mx+1; s->actLbl.resize(n,"O");
    for(auto&[k,v]:am->obj->m){try{int a=std::stoi(k);if(a>=0&&a<n)s->actLbl[a]=v.str;}catch(...){}}
    return!s->actLbl.empty();
}

// =========================================================================
// C API
// =========================================================================
ThincNERHandle ThincNER_Create(const char* d, const char*) {
    auto* s=new State(); if(!load(d,s)){delete s;return nullptr;}
    if(!load_labels(d,s))s->actLbl.resize(74,"O");
    return s;
}
void ThincNER_Destroy(ThincNERHandle h) { delete (State*)h; }

char* ThincNER_Predict(ThincNERHandle h, const char* tj) {
    auto* s=(State*)h; if(!s)return strdup("[]");
    if(!tj)return strdup("[]");

    // Parse tokens
    std::vector<std::string> tok; std::string j(tj); size_t p=0;
    while((p=j.find('"',p))!=std::string::npos){auto e=j.find('"',p+1);if(e==std::string::npos)break;std::string t=j.substr(p+1,e-p-1);if(!t.empty())tok.push_back(t);p=e+1;}
    int n=(int)tok.size(); if(!n)return strdup("[]");
    int NE=(int)s->embeds.size();
    // Derive per-embed dimension from loaded tensors (all embed tables share the same nO)
    int D = NE > 0 ? s->embeds[0].nO : 96;
    int EC = NE * D;

    // ---- Step 1: HashEmbed → concat (NER model: 4×96=384, pipe: 6×96=576) ----
    std::vector<float> emb((size_t)n*EC,0);
    for(int i=0;i<n;i++){
        auto ids=extract_features(tok[i],NE);
        size_t b=(size_t)i*EC;
        for(int e=0;e<NE&&e<(int)s->embeds.size();e++)
            s->embeds[e].embed(ids[e], emb.data()+b + (size_t)e*s->embeds[e].nO);
    }


    // ---- Step 2: Post-embed Maxout (576→96) ----
    std::vector<float> pe((size_t)n*D);
    for(int i=0;i<n;i++)maxout(pe.data()+(size_t)i*D,emb.data()+(size_t)i*EC,s->poW.data(),s->poB.data(),s->po_nO,s->po_nP,s->po_nI);

    // ---- Step 3: Post-embed LayerNorm ----
    std::vector<float> pln((size_t)n*D,0);
    if(s->has_poLN){for(int i=0;i<n;i++)layernorm(pln.data()+(size_t)i*D,pe.data()+(size_t)i*D,D,s->poG.data(),s->poB2.data(),1e-6f);}
    else pln=pe;

    // ---- Step 4: Residual encoder blocks ----
    std::vector<float> enc=pln;
    for(int ri=0;ri<s->n_res;ri++){
        auto& blk=s->res[ri]; if(!blk.has)continue;
        int wd=D*3;
        std::vector<float> exp((size_t)n*wd);
        for(int i=0;i<n;i++)expand_win(exp.data()+(size_t)i*wd,enc.data(),n,D,i);
        std::vector<float> mx((size_t)n*D);
        for(int i=0;i<n;i++)maxout(mx.data()+(size_t)i*D,exp.data()+(size_t)i*wd,blk.W.data(),blk.b.data(),D,3,wd);
        std::vector<float> ln((size_t)n*D);
        if(!blk.lnG.empty()){for(int i=0;i<n;i++)layernorm(ln.data()+(size_t)i*D,mx.data()+(size_t)i*D,D,blk.lnG.data(),blk.lnb.data(),1e-6f);}
        else ln=mx;
        for(int i=0;i<n;i++){float* op=enc.data()+(size_t)i*D;for(int j=0;j<D;j++)op[j]+=ln[(size_t)i*D+j];}
    }

	// ---- Step 5: NER tok2vec linear (96→64, no ReLU) ----
	// The NER model's layers[0] ends with a bare linear (no activation).
	// This produces the 64-dim token vectors that feed into the PrecomputableAffine.
	std::vector<float> hid((size_t)n*s->hO);
	if(s->has_hid){for(int i=0;i<n;i++){linear(hid.data()+(size_t)i*s->hO,enc.data()+(size_t)i*D,s->hW.data(),s->hB.data(),s->hO,D);}}
	else{for(int i=0;i<n;i++){int c=std::min(D,s->hO);memcpy(hid.data()+i*s->hO,enc.data()+i*D,c*4);}}

	// ---- Step 6: PrecomputableAffine → Maxout → Classifier → constrained decoding ----
	// Matches the spaCy ParserStepModel's predict_states formula:
	//   cached[t][f][o*nP+p] = W[f,o,p,:] @ hid[t] + b[o,p]  (f=0..nF-1, W=[nF,nO,nP,nI])
	//   unmaxed[o,nP+p] = sum_f cached[t][f][o*nP+p]  (sum over nF features)
	//   unmaxed += bias[o,p]  (add bias once, not nF times)
	//   hid_vec[o] = max(unmaxed[o*nP+0], unmaxed[o*nP+1])
	//   scores[a] = cW[a][:] @ hid_vec + cB[a]
	//
	// Feature token indices match spaCy's BiluoPushDown transition system:
	//   f=0: B(0) = buffer front = current token index
	//   f=1: S(0) = stack top = entity_start if in entity, else back-off to B(0)
	//   f=2: S(1) = stack second = back-off to B(0) (stack has ≤1 item in simple case)
	auto label_type = [](const std::string& lbl) -> char { return lbl.empty() ? 'O' : lbl[0]; };
	auto label_etype = [](const std::string& lbl) -> std::string { return lbl.size()<3?"":lbl.substr(2); };

	std::vector<std::string> tl(n, "O");
	if(s->has_cls && s->has_pre){
		int nF=s->p_nP, nO=s->p_nO, nP=s->p_nI, nD=s->p_nD; // W: [nF, nO, nP, nI]
		std::vector<float> unmaxed((size_t)nO * nP, 0);
		std::vector<float> hid_vec(nO, 0);
		std::vector<float> scores(s->nAct, 0);
		int entity_start = -1; // token index of current B-entity start, -1 = no entity

		for(int i=0;i<n;i++){
			// Determine feature token indices from transition state
			// (matches spaCy BiluoPushDown B(0)/S(0)/S(1) mapping)
			int ft[3];
			ft[0] = i;                          // B(0) = current token
			if(entity_start >= 0) {
				ft[1] = entity_start;               // S(0) = entity start
				ft[2] = i;                          // S(1) = back-off to B(0)
			} else {
				ft[1] = i;                          // S(0) = back-off to B(0)
				ft[2] = i;                          // S(1) = back-off to B(0)
			}

			// PrecomputableAffine: pre[f][o][p] = W[f][o][p][:] @ hid[ft[f]] + b[o][p]
			memset(unmaxed.data(), 0, (size_t)nO * nP * sizeof(float));
			for(int f=0;f<nF;f++){
				const float* hf = hid.data() + (size_t)ft[f] * nO;
				for(int o=0;o<nO;o++){
					for(int p=0;p<nP;p++){
						size_t base = (((size_t)f * nO + o) * nP + p) * nD;
						float val = 0;
						for(int d=0;d<nD;d++){
							val += s->pW_full[base + d] * hf[d];
						}
						unmaxed[(size_t)o * nP + p] += val;
					}
				}
			}

			// Add bias ONCE (not nF times)
			for(int o=0;o<nO;o++){
				for(int p=0;p<nP;p++){
					unmaxed[(size_t)o * nP + p] += s->pB_full[(size_t)o * nP + p];
				}
			}

			// Maxout: hid_vec[o] = max_p unmaxed[o*nP + p]
			for(int o=0;o<nO;o++){
				float best = unmaxed[(size_t)o * nP];
				for(int p=1;p<nP;p++){
					float v = unmaxed[(size_t)o * nP + p];
					if(v > best) best = v;
				}
				hid_vec[o] = best;
			}

			// Classifier: scores = cW @ hid_vec + cB
			linear(scores.data(), hid_vec.data(), s->cW.data(), s->cB.data(), s->nAct, nO);

			// Constrained greedy decoding
			char prev_type = i>0 ? label_type(tl[i-1]) : 'O';
			std::string prev_etype = i>0 ? label_etype(tl[i-1]) : "";
			int bst=-1; float bv=-1e30f;
			for(int a=0;a<s->nAct;a++){
				const std::string& lbl = (a<(int)s->actLbl.size()) ? s->actLbl[a] : "O";
				if(lbl.empty()) continue;
				char ct = label_type(lbl);
				std::string ce = label_etype(lbl);
				bool valid=false;
				if(prev_type=='O'||prev_type=='L'||prev_type=='U')
					valid = (ct=='O'||ct=='B'||ct=='U');
				else if(prev_type=='B'||prev_type=='I'){
					if(ct=='O') valid=true;
					else if((ct=='I'||ct=='L')&&ce==prev_etype) valid=true;
				}
				if(!valid) continue;
				if(scores[a]>bv){bv=scores[a];bst=a;}
			}
			if(bst>=0) {
				tl[i] = s->actLbl[bst];
				// Update entity_start for next token (BiluoPushDown stack tracking)
				char ct = label_type(tl[i]);
				if(ct == 'B') entity_start = i;
				else if(ct == 'I' || ct == 'L') { /* entity continues, keep entity_start */ }
				else entity_start = -1; // O or U → no entity open
			}
		}
	}

	// ---- Step 8: BILUO decode ----
	auto ents=decode_biluo(tok,tl);
    std::string r="["; for(size_t i=0;i<ents.size();i++){if(i)r+=",";r+="{\"text\":\""+ents[i].text+"\",\"label\":\""+ents[i].label+"\",\"start\":"+std::to_string(ents[i].start)+",\"end\":"+std::to_string(ents[i].end)+",\"confidence\":"+std::to_string(ents[i].conf)+"}";} r+="]";
    return strdup(r.c_str());
}

void ThincNER_FreeString(char* p) { free(p); }

char* ThincNER_Tokenize(const char* t, const char* l) {
    if(!t)return strdup("[]");
    std::string lang = l ? std::string(l) : "";
    // de, fr, es, pt: European Latin -> en tokenizer; zh, ja: CJK -> zh tokenizer
    bool is_cjk = (lang == "zh" || lang == "ja");
    auto tok = is_cjk ? tokenize_zh(t) : tokenize_en(t);
    std::string r="["; for(size_t i=0;i<tok.size();i++){if(i)r+=",";r+="\""+tok[i]+"\"";} r+="]";
    return strdup(r.c_str());
}
