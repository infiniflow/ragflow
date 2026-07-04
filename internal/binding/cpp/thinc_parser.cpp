#include "thinc_parser.h"

#include <algorithm>
#include <cmath>
#include <cstdlib>
#include <cstring>
#include <fstream>
#include <iostream>
#include <sstream>
#include <string>
#include <unordered_map>
#include <vector>

// =========================================================================
// Minimal JSON parser (replicated from thinc_ner.cpp)
// =========================================================================
namespace {
struct JVal; struct JObjMap;
struct JVal {
    enum Type{NUL,OBJ,ARR,STR,NUM,BOOL}type=NUL;
    std::string str; std::vector<JVal> arr; double num=0;
    JObjMap* obj=nullptr;
    JVal()=default;
    ~JVal();
    JVal(const JVal& o);
    JVal& operator=(const JVal& o);
    JVal(JVal&& o)noexcept;
    JVal& operator=(JVal&& o)noexcept;
    const JVal* get(const std::string& k)const;
    int as_int()const{return(int)num;}int64_t as_i64()const{return(int64_t)num;}
};
struct JObjMap{std::unordered_map<std::string,JVal>m;};
inline JVal::~JVal(){delete obj;}
inline JVal::JVal(const JVal& o):type(o.type),str(o.str),arr(o.arr),num(o.num){if(o.obj)obj=new JObjMap(*o.obj);}
inline JVal& JVal::operator=(const JVal& o){if(this!=&o){delete obj;type=o.type;str=o.str;arr=o.arr;num=o.num;obj=o.obj?new JObjMap(*o.obj):nullptr;}return*this;}
inline JVal::JVal(JVal&& o)noexcept:type(o.type),str(std::move(o.str)),arr(std::move(o.arr)),num(o.num),obj(o.obj){o.obj=nullptr;}
inline JVal& JVal::operator=(JVal&& o)noexcept{if(this!=&o){delete obj;type=o.type;str=std::move(o.str);arr=std::move(o.arr);num=o.num;obj=o.obj;o.obj=nullptr;}return*this;}
inline const JVal* JVal::get(const std::string& k)const{if(!obj)return nullptr;auto it=obj->m.find(k);return it!=obj->m.end()?&it->second:nullptr;}
struct JParser {
    const char *p,*e;
    char pk(){while(p<e&&(*p==' '||*p=='\t'||*p=='\n'||*p=='\r'))++p;return p<e?*p:0;}
    char nx(){while(p<e&&(*p==' '||*p=='\t'||*p=='\n'||*p=='\r'))++p;return p<e?*p++:0;}
    JVal pv(){char c=pk();if(c=='{')return po();if(c=='[')return pa();if(c=='"')return ps();if(c=='t'||c=='f')return pb();
        if(c=='n'){nx();nx();nx();nx();return JVal{};}return pn();}
    JVal po(){JVal v;v.type=JVal::OBJ;nx();if(!v.obj)v.obj=new JObjMap();while(p<e&&pk()!='}'){auto k=ps();nx();v.obj->m[k.str]=pv();if(p<e&&pk()==',')nx();else break;}if(p<e)nx();return v;}
    JVal pa(){JVal v;v.type=JVal::ARR;nx();while(p<e&&pk()!=']'){v.arr.push_back(pv());if(p<e&&pk()==',')nx();else break;}if(p<e)nx();return v;}
    JVal ps(){JVal v;v.type=JVal::STR;nx();while(p<e&&*p!='"'){if(*p=='\\'){++p;if(p<e){
        switch(*p){case'"':case'\\':case'/':v.str+=*p++;break;case'n':v.str+='\n';++p;break;case't':v.str+='\t';++p;break;case'r':v.str+='\r';++p;break;case'b':v.str+='\b';++p;break;case'f':v.str+='\f';++p;break;case'u':{if(p+4<e){char tmp[5]={p[1],p[2],p[3],p[4],0};v.str+=(char)strtol(tmp,nullptr,16);p+=5;}else{++p;}}break;default:v.str+=*p++;break;}
    }}else v.str+=*p++;}if(p<e)++p;return v;}
    JVal pn(){JVal v;v.type=JVal::NUM;auto s=p;if(p<e&&*p=='-')++p;while(p<e&&(*p>='0'&&*p<='9'))++p;
        if(p<e&&*p=='.'){++p;while(p<e&&(*p>='0'&&*p<='9'))++p;}
        if(p<e&&(*p=='e'||*p=='E')){++p;if(p<e&&(*p=='+'||*p=='-'))++p;while(p<e&&(*p>='0'&&*p<='9'))++p;}
        if(s<p){try{v.num=std::stod(std::string(s,p-s));}catch(...){v.num=0;}}return v;}
    JVal pb(){JVal v;v.type=JVal::BOOL;if(e-p>=4&&*p=='t'){v.str="true";p+=4;}else if(e-p>=5&&*p=='f'){v.str="false";p+=5;}return v;}
    JVal parse(const std::string& j){p=j.data();e=p+j.size();return pv();}
};

// =========================================================================
// HashEmbed + MurmurHash (copied from thinc_ner.cpp)
// =========================================================================
#define ROTL64(x,r) ((x << r) | (x >> (64 - r)))
static uint64_t getblock64(const uint64_t* p, size_t i) { uint64_t r; memcpy(&r, p+i, 8); return r; }
static uint64_t fmix64(uint64_t k) {
    k ^= k >> 33; k *= 0xff51afd7ed558ccdULL; k ^= k >> 33; k *= 0xc4ceb9fe1a85ec53ULL; k ^= k >> 33; return k;
}
static void mmh3_x64_128(const void* key, int len, uint32_t seed, uint32_t out[4]) {
    const uint8_t* data=(const uint8_t*)key; int nblocks=len/16;
    uint64_t h1=seed,h2=seed,c1=0x87c37b91114253d5ULL,c2=0x4cf5ad432745937fULL;
    const uint64_t* blocks=(const uint64_t*)data;
    for(int i=0;i<nblocks;i++){
        uint64_t k1=getblock64(blocks,i*2+0),k2=getblock64(blocks,i*2+1);
        k1*=c1;k1=ROTL64(k1,31);k1*=c2;h1^=k1;h1=ROTL64(h1,27);h1+=h2;h1=h1*5+0x52dce729;
        k2*=c2;k2=ROTL64(k2,33);k2*=c1;h2^=k2;h2=ROTL64(h2,31);h2+=h1;h2=h2*5+0x38495ab5;
    }
    const uint8_t* tail=(const uint8_t*)(data+nblocks*16);uint64_t k1=0,k2=0;
    switch(len&15){
        case 15:k2^=((uint64_t)tail[14])<<48;case 14:k2^=((uint64_t)tail[13])<<40;
        case 13:k2^=((uint64_t)tail[12])<<32;case 12:k2^=((uint64_t)tail[11])<<24;
        case 11:k2^=((uint64_t)tail[10])<<16;case 10:k2^=((uint64_t)tail[9])<<8;
        case 9:k2^=((uint64_t)tail[8])<<0;k2*=c2;k2=ROTL64(k2,33);k2*=c1;h2^=k2;
        case 8:k1^=((uint64_t)tail[7])<<56;case 7:k1^=((uint64_t)tail[6])<<48;
        case 6:k1^=((uint64_t)tail[5])<<40;case 5:k1^=((uint64_t)tail[4])<<32;
        case 4:k1^=((uint64_t)tail[3])<<24;case 3:k1^=((uint64_t)tail[2])<<16;
        case 2:k1^=((uint64_t)tail[1])<<8;case 1:k1^=((uint64_t)tail[0])<<0;k1*=c1;k1=ROTL64(k1,31);k1*=c2;h1^=k1;
    };h1^=len;h2^=len;h1+=h2;h2+=h1;h1=fmix64(h1);h2=fmix64(h2);h1+=h2;h2+=h1;
    out[0]=(uint32_t)h1;out[1]=(uint32_t)(h1>>32);out[2]=(uint32_t)h2;out[3]=(uint32_t)(h2>>32);
}

static uint64_t mh2_64a(const void* key, int len, uint64_t seed) {
    const uint64_t m=0xc6a4a7935bd1e995ULL;const int r=47;
    uint64_t h=seed^(uint64_t(len)*m);auto d=(const uint8_t*)key;int rm=len;
    while(rm>=8){uint64_t k;memcpy(&k,d,8);k*=m;k^=k>>r;k*=m;h^=k;h*=m;d+=8;rm-=8;}
    switch(rm){case 7:h^=uint64_t(d[6])<<48;case 6:h^=uint64_t(d[5])<<40;case 5:h^=uint64_t(d[4])<<32;
    case 4:h^=uint64_t(d[3])<<24;case 3:h^=uint64_t(d[2])<<16;case 2:h^=uint64_t(d[1])<<8;case 1:h^=d[0];h*=m;break;}
    h^=h>>r;h*=m;h^=h>>r;return h;
}
static uint64_t hash_feat(const std::string& s){return s.empty()?0:mh2_64a(s.data(),(int)s.size(),0);}

struct HashEmbed{
    int n_rows=0,nO=0;uint32_t seed=0;std::vector<float> table;
    bool load(int r,int o,const float*d){n_rows=r;nO=o;table.assign(d,d+(size_t)r*o);return!table.empty();}
    void embed(uint64_t fid,float* out)const{
        uint8_t in[8];for(int i=0;i<8;i++)in[i]=(uint8_t)(fid>>(i*8));
        uint32_t keys[4];mmh3_x64_128(in,8,seed,keys);
        for(int v=0;v<4;v++){int idx=(int)(keys[v]%(uint32_t)n_rows);for(int i=0;i<nO;i++)out[i]+=table[(size_t)idx*nO+i];}
    }
};

// =========================================================================
// Layer primitives
// =========================================================================
static void linear(float* out, const float* in, const float* W, const float* b, int nO, int nI) {
    for(int i=0;i<nO;i++){float s=b[i];for(int j=0;j<nI;j++)s+=W[(size_t)i*nI+j]*in[j];out[i]=s;}
}
static void maxout(float* out, const float* in, const float* W, const float* b, int nO, int nP, int nI) {
    for(int i=0;i<nO;i++){float best=-1e30f;for(int p=0;p<nP;p++){float s=b[(size_t)i*nP+p];for(int j=0;j<nI;j++)s+=W[((size_t)i*nP+p)*nI+j]*in[j];if(s>best)best=s;}out[i]=best;}
}
static void layernorm(float* out, const float* in, int d, const float* G, const float* b, float eps) {
    float mn=0,vr=0;for(int i=0;i<d;i++)mn+=in[i];mn/=d;for(int i=0;i<d;i++)vr+=(in[i]-mn)*(in[i]-mn);vr/=d;float is=1.0f/sqrtf(vr+eps);
    for(int i=0;i<d;i++)out[i]=G[i]*(in[i]-mn)*is+b[i];
}
static void expand_win(float* out, const float* all, int n, int dim, int idx) {
    int off=idx*dim;if(idx>0)memcpy(out,all+(idx-1)*dim,dim*sizeof(float));else memset(out,0,dim*sizeof(float));
    memcpy(out+dim,all+off,dim*sizeof(float));if(idx<n-1)memcpy(out+2*dim,all+(idx+1)*dim,dim*sizeof(float));else memset(out+2*dim,0,dim*sizeof(float));
}

// =========================================================================
// Feature extraction — UTF-8 aware, matching spaCy
// =========================================================================
static std::string u8_first(const std::string& s){
    if(s.empty())return"";unsigned char c=(unsigned char)s[0];int l=1;
    if((c&0xE0)==0xC0)l=2;else if((c&0xF0)==0xE0)l=3;else if((c&0xF8)==0xF0)l=4;
    return s.substr(0,(size_t)l<=s.size()?l:1);
}
static size_t u8_len(const std::string& s){
    size_t n=0;
    for(size_t i=0;i<s.size();n++){unsigned char c=(unsigned char)s[i];
        if((c&0x80)==0)i+=1;else if((c&0xE0)==0xC0)i+=2;else if((c&0xF0)==0xE0)i+=3;else if((c&0xF8)==0xF0)i+=4;else i+=1;}
    return n;
}
static std::string u8_last(const std::string& s, size_t count){
    size_t ul=u8_len(s);if(ul<=count)return s;size_t pos=0;
    for(size_t i=0;i<ul-count;i++){unsigned char c=(unsigned char)s[pos];
        if((c&0x80)==0)pos+=1;else if((c&0xE0)==0xC0)pos+=2;else if((c&0xF0)==0xE0)pos+=3;else if((c&0xF8)==0xF0)pos+=4;else pos+=1;}
    return s.substr(pos);
}
static std::vector<uint64_t> extract_features(const std::string& t, int n_embed){
    auto fn=[&](const std::string& s){return hash_feat(s);};
    auto fp=[&](const std::string& s){std::string p=s.empty()?"":u8_first(s);std::transform(p.begin(),p.end(),p.begin(),::tolower);return hash_feat(p);};
    auto fs=[&](const std::string& s){std::string su=u8_len(s)>=3?u8_last(s,3):s;std::transform(su.begin(),su.end(),su.begin(),::tolower);return hash_feat(su);};
    auto fsh=[&](const std::string& t2){std::string sh;for(unsigned char c:t2){if(c>0x7F)sh+='x';else if(std::isupper(c))sh+='X';else if(std::islower(c))sh+='x';else if(std::isdigit(c))sh+='d';else sh+=c;}return hash_feat(sh);};
    std::vector<uint64_t> ids;
    std::string lo=t;std::transform(lo.begin(),lo.end(),lo.begin(),::tolower);
    ids.push_back(fn(lo));ids.push_back(fp(t));ids.push_back(fs(t));ids.push_back(fsh(t));
    if(n_embed==6){ids.push_back(1);ids.push_back(0);}else{ids.push_back(0);}
    return ids;
}

// =========================================================================
// Tok2vec forward pass (shared with NER)
// =========================================================================
struct Tok2vecModel {
    std::vector<HashEmbed> embeds;
    std::vector<float> poW,poB,poG,poB2; bool has_poLN=false;
    int po_nO=96,po_nP=3,po_nI=576;
    struct ResBlk{bool has=false;std::vector<float>W,b,lnG,lnb;};
    ResBlk res[4]; int n_res=0;
    
    bool load(const std::string& dir) {
        std::ifstream cf(dir+"/model.ckpt"); if(!cf)return false;
        std::stringstream cb;cb<<cf.rdbuf();
        JVal ck=JParser().parse(cb.str()); if(ck.type!=JVal::OBJ)return false;
        std::ifstream bf(dir+"/model.bin",std::ios::binary|std::ios::ate); if(!bf)return false;
        size_t bz=bf.tellg();bf.seekg(0); if(bz%4!=0||bz==0)return false;
        std::vector<float> bin(bz/4); bf.read((char*)bin.data(),bz);
        auto sl=[&](int64_t o,int64_t c)->std::vector<float>{
            if(o+c>(int64_t)bin.size())return{}; return std::vector<float>(bin.begin()+o,bin.begin()+o+c);
        };
        auto ld=[&](const std::string& k, std::vector<float>* v,int* r0=nullptr,int* r1=nullptr,int* r2=nullptr)->bool{
            auto* e=ck.get(k);if(!e)return false;
            auto sv=e->get("shape"),ov=e->get("offset"),cv=e->get("count");
            if(!sv||!ov||!cv)return false;
            *v=sl(ov->as_i64(),cv->as_i64());
            if(r0)*r0=sv->arr.size()>=1?sv->arr[0].as_int():1;
            if(r1)*r1=sv->arr.size()>=2?sv->arr[1].as_int():1;
            if(r2)*r2=sv->arr.size()>=3?sv->arr[2].as_int():1;
            return!v->empty();
        };
        for(int ei=0;;ei++){
            auto* e=ck.get("embed_"+std::to_string(ei)+"_E");if(!e)break;
            auto sv=e->get("shape"),ov=e->get("offset"),cv=e->get("count");
            if(!sv||!ov||!cv)break;int rs=sv->arr[0].as_int(),nO=sv->arr[1].as_int();
            int64_t exp=(int64_t)rs*nO;if(cv->as_i64()<exp)break;
            auto d=sl(ov->as_i64(),cv->as_i64());if(d.empty())break;
            embeds.emplace_back();embeds.back().load(rs,nO,d.data());
        }
        std::ifstream ff(dir+"/feature_config.json");
        if(ff){std::stringstream fb;fb<<ff.rdbuf();auto cfg=JParser().parse(fb.str());auto* sa=cfg.get("embed_seeds");
            if(sa&&sa->type==JVal::ARR)for(int i=0;i<(int)sa->arr.size()&&i<(int)embeds.size();i++)embeds[i].seed=(uint32_t)sa->arr[i].as_int();}
        int r0=0,r1=0,r2=0;
        if(!ld("poW",&poW,&r0,&r1,&r2))return false;
        po_nO=r0;po_nP=r1;po_nI=r2;
        if(!ld("poB",&poB))return false;
        if(ld("poG",&poG)){ld("poB2",&poB2);has_poLN=true;}
        for(int ri=0;ri<4;ri++){auto pk="res"+std::to_string(ri);auto& rb=res[ri];
            if(ld(pk+"W",&rb.W,&r0,&r1,&r2)){ld(pk+"B",&rb.b);ld(pk+"lnG",&rb.lnG);ld(pk+"lnb",&rb.lnb);rb.has=true;n_res++;}}
        return!embeds.empty();
    }
    
    // Run tok2vec → (n_tokens, 96)
    void forward(const std::vector<std::string>& tokens, float* out) {
        int n=(int)tokens.size(),D=96,NE=(int)embeds.size(),EC=NE*D;
        std::vector<float> emb((size_t)n*EC,0);
        for(int i=0;i<n;i++){
            auto ids=extract_features(tokens[i],NE);
            size_t b=(size_t)i*EC;
            for(int e=0;e<NE;e++)embeds[e].embed(ids[e],emb.data()+b+(size_t)e*D);
        }
        std::vector<float> pe((size_t)n*D);
        for(int i=0;i<n;i++)maxout(pe.data()+(size_t)i*D,emb.data()+(size_t)i*EC,poW.data(),poB.data(),D,po_nP,EC);
        std::vector<float> pln((size_t)n*D,0);
        if(has_poLN)for(int i=0;i<n;i++)layernorm(pln.data()+(size_t)i*D,pe.data()+(size_t)i*D,D,poG.data(),poB2.data(),1e-6f);else pln=pe;
        std::vector<float> enc=pln;
        for(int ri=0;ri<n_res;ri++){if(!res[ri].has)continue;
            int wd=D*3;std::vector<float> exp((size_t)n*wd);
            for(int i=0;i<n;i++)expand_win(exp.data()+(size_t)i*wd,enc.data(),n,D,i);
            std::vector<float> mx((size_t)n*D);for(int i=0;i<n;i++)maxout(mx.data()+(size_t)i*D,exp.data()+(size_t)i*wd,res[ri].W.data(),res[ri].b.data(),D,3,wd);
            std::vector<float> ln((size_t)n*D);if(!res[ri].lnG.empty())for(int i=0;i<n;i++)layernorm(ln.data()+(size_t)i*D,mx.data()+(size_t)i*D,D,res[ri].lnG.data(),res[ri].lnb.data(),1e-6f);else ln=mx;
            for(int i=0;i<n;i++){float* op=enc.data()+(size_t)i*D;for(int j=0;j<D;j++)op[j]+=ln[(size_t)i*D+j];}
        }
        memcpy(out,enc.data(),(size_t)n*D*sizeof(float));
    }
};

// =========================================================================
// Arc-hybrid Parser
// =========================================================================
struct ParserModel {
    int nO=64,nP=8,nI=2,n_actions=0;
    std::vector<float> pW_hid,pb_hid; // 96→64
    std::vector<float> pW_pre,pb_pre,pad_pre; // preaffine
    std::vector<float> pW_cls,pb_cls; // classifier
    std::vector<std::string> move_names;
    
    bool load(const std::string& dir) {
        std::ifstream cf(dir+"/model.ckpt"); if(!cf)return false;
        std::stringstream cb;cb<<cf.rdbuf();
        JVal ck=JParser().parse(cb.str()); if(ck.type!=JVal::OBJ)return false;
        std::ifstream bf(dir+"/model.bin",std::ios::binary|std::ios::ate); if(!bf)return false;
        size_t bz=bf.tellg();bf.seekg(0); if(bz%4!=0||bz==0)return false;
        std::vector<float> bin(bz/4); bf.read((char*)bin.data(),bz);
        auto sl=[&](int64_t o,int64_t c)->std::vector<float>{
            if(o+c>(int64_t)bin.size())return{}; return std::vector<float>(bin.begin()+o,bin.begin()+o+c);
        };
        auto ld=[&](const std::string& k, std::vector<float>* v,int* r0=nullptr,int* r1=nullptr,int* r2=nullptr)->bool{
            auto* e=ck.get(k);if(!e)return false;
            auto sv=e->get("shape"),ov=e->get("offset"),cv=e->get("count");
            if(!sv||!ov||!cv)return false;
            *v=sl(ov->as_i64(),cv->as_i64());
            if(r0)*r0=sv->arr.size()>=1?sv->arr[0].as_int():1;
            if(r1)*r1=sv->arr.size()>=2?sv->arr[1].as_int():1;
            if(r2)*r2=sv->arr.size()>=3?sv->arr[2].as_int():1;
            return!v->empty();
        };
        int r0,r1,r2;
        if(!ld("pW_hid",&pW_hid,&r0,&r1))return false;
        nO=r0;ld("pb_hid",&pb_hid);
        if(!ld("pW_pre",&pW_pre,&r0,&r1,&r2))return false;
        nP=r0;nO=r1;nI=r2;
        ld("pb_pre",&pb_pre);ld("pad_pre",&pad_pre);
        if(!ld("pW_cls",&pW_cls,&r0,&r1))return false;
        n_actions=r0;ld("pb_cls",&pb_cls);
        std::ifstream mf(dir+"/meta.json");
        if(mf){std::stringstream mb;mb<<mf.rdbuf();auto meta=JParser().parse(mb.str());auto* mn=meta.get("move_names");
            if(mn&&mn->type==JVal::ARR)for(auto& v:mn->arr)move_names.push_back(v.str);}
        return!pW_hid.empty() && !pW_pre.empty() && !pW_cls.empty();
    }
    
    // Run parser forward + state machine → (heads, labels)
    void parse(const float* tokvecs, int n_tokens,
               std::vector<int>& out_heads, std::vector<std::string>& out_labels) {
        // 1. Hidden layer: 96→64
        std::vector<float> hidden((size_t)n_tokens*nO,0);
        for(int i=0;i<n_tokens;i++) linear(hidden.data()+(size_t)i*nO, tokvecs+(size_t)i*96,
                                           pW_hid.data(), pb_hid.data(), nO, 96);
        
        // 2. Pre-compute features
        std::vector<float> precomp((size_t)(n_tokens+1)*nP*nO*nI,0);
        // Pad token (index 0)
        memcpy(precomp.data(), pad_pre.data(), (size_t)nP*nO*nI*sizeof(float));
        // Real tokens
        for(int i=0;i<n_tokens;i++){
            size_t toff = (size_t)(i+1)*nP*nO*nI;
            for(int p=0;p<nP;p++){
                for(int w=0;w<nI;w++){
                    size_t base = toff + (size_t)p*nO*nI + (size_t)w;
                    float* out = precomp.data() + base;
                    // W[p][w][o][d] = [nP][nI][nO][nO]
                    for(int o=0;o<nO;o++){
                        float s = pb_pre[(size_t)w*nO + o];
                        for(int d=0;d<nO;d++) s += pW_pre[((size_t)p*nO*nI + (size_t)o*nI + w)*nO + d] * hidden[(size_t)i*nO + d];
                        out[(size_t)o*nI] = s;
                    }
                }
            }
        }
        
        // Helper: get feature value for a token at piece p, window w, output dim o
        auto feat = [&](int idx, int p, int w, int o) -> float {
            int ri = (idx < 0 || idx >= n_tokens) ? 0 : idx + 1;
            return precomp[(size_t)ri*nP*nO*nI + (size_t)p*nO*nI + (size_t)w + (size_t)o*nI];
        };
        
                // 3. Arc-hybrid state machine
        out_heads.assign(n_tokens, -1);
        out_labels.assign(n_tokens, "");
        std::vector<int> stack;
        std::vector<int> buffer(n_tokens);
        for(int i=0;i<n_tokens;i++) buffer[i]=i;
        
        // Validate move_names covers all actions before indexing
        if((int)move_names.size()!=n_actions){out_heads.clear();out_labels.clear();return;}
        int act_S=-1, act_D=-1;
        for(int i=0;i<(int)move_names.size();i++){
            if(move_names[i]=="S") act_S=i;
            if(move_names[i]=="D") act_D=i;
        }
        
        auto leftmost = [&](int idx)->int{
            for(int i=0;i<n_tokens;i++) if(out_heads[i]==idx) return i;
            return -1;
        };
        auto rightmost = [&](int idx)->int{
            int r=-1; for(int i=0;i<n_tokens;i++) if(out_heads[i]==idx) r=i;
            return r;
        };
        
        std::vector<float> scores(n_actions,0);
        std::vector<float> feats(nP*nI*nO,0);
        
        for(int step=0; step<n_tokens*4 && !(buffer.empty()&&stack.size()<=1); step++){
            int s0=stack.empty()?-1:stack.back();
            int s1=stack.size()<2?-1:stack[stack.size()-2];
            int s2=stack.size()<3?-1:stack[stack.size()-3];
            int b0=buffer.empty()?-1:buffer[0];
            int b1=buffer.size()<2?-1:buffer[1];
            
            // Build feature indices (same as verified Python implementation)
            int idxs[16]={s0,s1, b0,s0, s0,leftmost(s0), s0,rightmost(s0),
                          s1,leftmost(s1), s1,rightmost(s1), s2,b1, b0,b1};
            
            // Build feature vector: sum of precomputed features at each (idx, piece, window)
            for(int o=0;o<nO;o++) feats[o]=0;
            for(int p=0;p<nP;p++){
                for(int w=0;w<nI;w++){
                    int ti=idxs[p*2+w];
                    for(int o=0;o<nO;o++){
                        feats[(size_t)p*nI*nO + (size_t)w*nO + o] = feat(ti,p,w,o);
                    }
                }
            }
            
            // Classify
            for(int a=0;a<n_actions;a++){
                float s=pb_cls[a];
                // Use HIDDEN state directly (64-dim) as classifier input
                int cls_idx = b0 >= 0 ? b0 : (s0 >= 0 ? s0 : 0);
                for(int j=0;j<nO;j++) s += pW_cls[(size_t)a*nO+j] * hidden[(size_t)cls_idx*nO+j];
                scores[a]=s;
            }
            
            // Pick best VALID action
            int best=-1; float best_sc=-1e30f;
            for(int a=0;a<n_actions;a++){
                bool valid=false;
                const std::string& n=move_names[a];
                if(n=="S") valid=!buffer.empty() && (int)stack.size()<n_tokens;
                else if(n=="D") valid=!stack.empty();
                else if(n.size()>=2 && (n[0]=='L'||n[0]=='R')) valid=stack.size()>=2;
                if(valid && scores[a]>best_sc){best_sc=scores[a];best=a;}
            }
            if(best<0) break;
            
            const std::string& act=move_names[best];
            if(act=="S"){stack.push_back(buffer[0]);buffer.erase(buffer.begin());}
            else if(act=="D"){stack.pop_back();}
            else if(act.size()>=2){
                std::string lbl=act.substr(2);
                if(act[0]=='L'){out_heads[s0]=s1;out_labels[s0]=lbl;stack.erase(stack.end()-2);}
                else{out_heads[s1]=s0;out_labels[s1]=lbl;stack.pop_back();}
            }
        }
    }
};

// =========================================================================
// Combined state
// =========================================================================
struct ParserState {
    Tok2vecModel tok2vec;
    ParserModel parser;
    bool loaded=false;
};

struct TaggerState {
    Tok2vecModel tok2vec;
    std::vector<float> tW,tb; // (n_tags, 96), (n_tags,)
    std::vector<std::string> tags;
    bool loaded=false;
};

} // namespace

// =========================================================================
// C API — Parser
// =========================================================================
ThincParserHandle ThincParser_Create(const char* ner_dir, const char* parser_dir) {
    auto* s=new ParserState();
    if(!ner_dir||!parser_dir){delete s;return nullptr;}
    // Load PIPELINE tok2vec from <base>/tok2vec/ subdirectory (not NER's internal 4HE).
    // ner_dir is typically <model_base>/ner/.
    std::string base = std::string(ner_dir);
    if(base.size()>=4 && base.substr(base.size()-4)=="/ner") base.resize(base.size()-4);
    if(!s->tok2vec.load(base+"/tok2vec")){delete s;return nullptr;}
    if(!s->parser.load(std::string(parser_dir))){delete s;return nullptr;}
    s->loaded=true;
    return s;
}

void ThincParser_Destroy(ThincParserHandle h) { delete (ParserState*)h; }

char* ThincParser_Predict(ThincParserHandle h, const char* tokens_json) {
    auto* s=(ParserState*)h;
    if(!s||!s->loaded||!tokens_json) return strdup("[]");
    
    auto j=JParser().parse(std::string(tokens_json));
    if(j.type!=JVal::ARR) return strdup("[]");
    std::vector<std::string> tokens;
    for(auto& v:j.arr) tokens.push_back(v.str);
    int n=(int)tokens.size();
    if(!n) return strdup("[]");
    
    // Run tok2vec
    std::vector<float> tokvecs((size_t)n*96,0);
    s->tok2vec.forward(tokens, tokvecs.data());
    
    // Run parser
    std::vector<int> heads;
    std::vector<std::string> labels;
    s->parser.parse(tokvecs.data(), n, heads, labels);
    
    // Build JSON output
    std::string r="[";
    for(int i=0;i<n;i++){
        if(i)r+=",";
        r+="{\"text\":\""+tokens[i]+"\",\"head\":"+std::to_string(heads[i])+
           ",\"dep\":\""+labels[i]+"\",\"index\":"+std::to_string(i)+"}";
    }
    r+="]";
    return strdup(r.c_str());
}

void ThincParser_FreeString(char* p) { free(p); }

// =========================================================================
// C API — Tagger
// =========================================================================
ThincTaggerHandle ThincTagger_Create(const char* ner_dir, const char* tagger_dir) {
    auto* s=new TaggerState();
    if(!ner_dir||!tagger_dir){delete s;return nullptr;}
    // Load PIPELINE tok2vec from <base>/tok2vec/ (6HE, not NER's internal 4HE)
    std::string tbase = std::string(ner_dir);
    if(tbase.size()>=4 && tbase.substr(tbase.size()-4)=="/ner") tbase.resize(tbase.size()-4);
    if(!s->tok2vec.load(tbase+"/tok2vec")){delete s;return nullptr;}
    std::ifstream cf(std::string(tagger_dir)+"/model.ckpt"); if(!cf){delete s;return nullptr;}
    std::stringstream cb;cb<<cf.rdbuf();
    JVal ck=JParser().parse(cb.str()); if(ck.type!=JVal::OBJ){delete s;return nullptr;}
    std::ifstream bf(std::string(tagger_dir)+"/model.bin",std::ios::binary|std::ios::ate); if(!bf){delete s;return nullptr;}
    size_t bz=bf.tellg();bf.seekg(0); std::vector<float> bin(bz/4); bf.read((char*)bin.data(),bz);
    auto sl=[&](int64_t o,int64_t c)->std::vector<float>{
        if(o+c>(int64_t)bin.size())return{}; return std::vector<float>(bin.begin()+o,bin.begin()+o+c);
    };
    auto ld=[&](const std::string& k, std::vector<float>* v,int* r0=nullptr)->bool{
        auto* e=ck.get(k);if(!e)return false;
        auto sv=e->get("shape"),ov=e->get("offset"),cv=e->get("count");
        if(!sv||!ov||!cv)return false;
        *v=sl(ov->as_i64(),cv->as_i64());
        if(r0)*r0=sv->arr.size()>=1?sv->arr[0].as_int():1;
        return!v->empty();
    };
    int r0=0; ld("tW",&s->tW,&r0); ld("tb",&s->tb);
    std::ifstream mf(std::string(tagger_dir)+"/meta.json");
    if(mf){std::stringstream mb;mb<<mf.rdbuf();auto meta=JParser().parse(mb.str());auto* tg=meta.get("tags");
        if(tg&&tg->type==JVal::ARR)for(auto& v:tg->arr)s->tags.push_back(v.str);}
    s->loaded=!s->tW.empty();
    return s;
}

void ThincTagger_Destroy(ThincTaggerHandle h) { delete (TaggerState*)h; }

char* ThincTagger_Predict(ThincTaggerHandle h, const char* tokens_json) {
    auto* s=(TaggerState*)h;
    if(!s||!s->loaded||!tokens_json||s->tW.empty()) return strdup("[]");
    auto j=JParser().parse(std::string(tokens_json));
    if(j.type!=JVal::ARR)return strdup("[]");
    std::vector<std::string> tokens;
    for(auto& v:j.arr) tokens.push_back(v.str);
    int n=(int)tokens.size(), n_tags=(int)s->tW.size()/96;
    if(!n||!n_tags)return strdup("[]");
    
    // Run tok2vec to get 96-dim embeddings, then softmax + argmax
    std::vector<float> tokvecs((size_t)n*96,0);
    s->tok2vec.forward(tokens, tokvecs.data());
    
    std::vector<int> best_tags(n, 0);
    for(int i=0;i<n;i++){
        float best_sc=-1e30f;
        for(int t=0;t<n_tags;t++){
            float sc=s->tb[t];
            for(int j=0;j<96;j++) sc += s->tW[(size_t)t*96+j] * tokvecs[(size_t)i*96+j];
            if(sc>best_sc){best_sc=sc;best_tags[i]=t;}
        }
    }
    
    // Strip morphologizer output to just POS (e.g. "Gender=Masc|Number=Sing|POS=NOUN" → "NOUN")
    // For non-morphologizer models the tag string is used as-is.
    auto pos_only = [](const std::string& t) -> std::string {
        auto p = t.find("POS=");
        if(p==std::string::npos) return t;
        auto s = p+4;
        auto e = t.find_first_of("|;", s);
        if(e==std::string::npos) e = t.size();
        return t.substr(s, e-s);
    };
    std::string r="[";
    for(int i=0;i<n;i++){
        if(i)r+=",";
        std::string tag = best_tags[i] < (int)s->tags.size() ? s->tags[best_tags[i]] : "";
        r+="{\"text\":\""+tokens[i]+"\",\"tag\":\""+pos_only(tag)+"\",\"index\":"+std::to_string(i)+"}";
    }
    r+="]";
    return strdup(r.c_str());
}

void ThincTagger_FreeString(char* p) { free(p); }
