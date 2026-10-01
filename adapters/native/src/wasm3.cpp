// Qualified against wasm3 v0.5.0. No shell CLI timing and no WASI host stubs.
#include "embedding.h"
#include <wasm3.h>
#include <memory>
struct Engine {IM3Environment value=m3_NewEnvironment();~Engine(){if(value)m3_FreeEnvironment(value);}};
struct Module {
    std::vector<uint8_t> bytes;
    IM3Runtime runtime=nullptr;
    IM3Module module=nullptr;
    bool loaded=false;
    ~Module(){if(runtime)m3_FreeRuntime(runtime);if(module&&!loaded)m3_FreeModule(module);}
};
// wasm3 owns loaded modules in a runtime. An instance is a borrowed view;
// the owning Module must outlive it (enforced by the Rust adapter lifecycle).
static void check(M3Result r){if(r)throw std::runtime_error(r);}
static IM3Function function(Module* m,const char* name){IM3Function f=nullptr;check(m3_FindFunction(&f,m->runtime,name));return f;}
static uint8_t type(M3ValueType v){return v==c_m3Type_i32?0x7f:v==c_m3Type_i64?0x7e:0;}
extern "C" {
const char* wb_error(){return wb_last_error.c_str();}
const char* wb_version(){return WB_VERSION;}
void* wb_engine_new(){WB_TRY{auto e=std::make_unique<Engine>();if(!e->value)throw std::runtime_error("environment creation failed");return e.release();}WB_CATCH(nullptr)}
void wb_engine_delete(void* e){delete static_cast<Engine*>(e);}
void* wb_module_new(void* e,const uint8_t* bytes,size_t n){WB_TRY{
    if(n>UINT32_MAX)throw std::runtime_error("module too large");auto m=std::make_unique<Module>();m->bytes.assign(bytes,bytes+n);
    m->runtime=m3_NewRuntime(static_cast<Engine*>(e)->value,1<<20,nullptr);if(!m->runtime)throw std::runtime_error("runtime creation failed");
    check(m3_ParseModule(static_cast<Engine*>(e)->value,&m->module,m->bytes.data(),static_cast<uint32_t>(n)));
    check(m3_LoadModule(m->runtime,m->module));m->loaded=true;check(m3_CompileModule(m->module));return m.release();
}WB_CATCH(nullptr)}
void wb_module_delete(void* m){delete static_cast<Module*>(m);}
void* wb_instance_new(void*,void* m){WB_TRY{check(m3_RunStart(static_cast<Module*>(m)->module));return m;}WB_CATCH(nullptr)}
void wb_instance_delete(void*){}
int wb_signature(void* m,const char* name,uint8_t* params,size_t* np,uint8_t* results,size_t* nr){WB_TRY{
    auto f=function(static_cast<Module*>(m),name);const auto p=m3_GetArgCount(f),r=m3_GetRetCount(f);
    if(p>*np||r>*nr)throw std::runtime_error("unsupported: signature exceeds 32 values");*np=p;*nr=r;
    for(size_t k=0;k<p;k++)params[k]=type(m3_GetArgType(f,k));for(size_t k=0;k<r;k++)results[k]=type(m3_GetRetType(f,k));return 0;
}WB_CATCH(-1)}
int wb_call(void* m,const char* name,const uint64_t* args,size_t na,uint64_t* out,size_t nr){WB_TRY{
    auto f=function(static_cast<Module*>(m),name);if(na!=m3_GetArgCount(f)||nr!=m3_GetRetCount(f))throw std::runtime_error("signature arity mismatch");
    std::vector<uint32_t> small(na);std::vector<const void*> p(na),r(nr);
    for(size_t k=0;k<na;k++){const auto kind=type(m3_GetArgType(f,k));if(kind==0x7f){small[k]=static_cast<uint32_t>(args[k]);p[k]=&small[k];}else if(kind==0x7e)p[k]=&args[k];else throw std::runtime_error("unsupported: integer parameters only");}
    for(size_t k=0;k<nr;k++){if(!type(m3_GetRetType(f,k)))throw std::runtime_error("unsupported: integer results only");out[k]=0;r[k]=&out[k];}
    check(m3_Call(f,static_cast<uint32_t>(na),p.data()));check(m3_GetResults(f,static_cast<uint32_t>(nr),r.data()));
    for(size_t k=0;k<nr;k++)if(type(m3_GetRetType(f,k))==0x7f)out[k]=static_cast<uint32_t>(out[k]);return 0;
}WB_CATCH(-1)}
uint8_t* wb_memory(void* m,size_t* n){uint32_t size=0;auto p=m3_GetMemory(static_cast<Module*>(m)->runtime,&size,0);*n=size;return p;}
}
