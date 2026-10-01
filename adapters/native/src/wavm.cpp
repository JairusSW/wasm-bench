#include "embedding.h"
#include <WAVM/wavm-c/wavm-c.h>
#include <memory>
struct Engine { wasm_engine_t* value=wasm_engine_new(); ~Engine(){if(value)wasm_engine_delete(value);} };
struct Instance {
    wasm_compartment_t* compartment=nullptr;
    wasm_store_t* store=nullptr;
    wasm_instance_t* value=nullptr;
    const wasm_module_t* module=nullptr;
    ~Instance(){if(value)wasm_instance_delete(value);if(store)wasm_store_delete(store);if(compartment)wasm_compartment_delete(compartment);}
};
static wasm_extern_t* find(Instance* i,const char* name){
    for(size_t k=0;k<wasm_module_num_exports(i->module);k++){
        wasm_export_t e{};wasm_module_export(i->module,k,&e);
        const bool equal=e.num_name_bytes==strlen(name)&&memcmp(e.name,name,e.num_name_bytes)==0;
        wasm_externtype_delete(e.type);
        if(equal)return wasm_instance_export(i->value,k);
    }throw std::runtime_error("missing export");
}
static wasm_func_t* function(Instance* i,const char* name){auto f=wasm_extern_as_func(find(i,name));if(!f)throw std::runtime_error("export is not a function");return f;}
static uint8_t type(wasm_valtype_t* v){switch(wasm_valtype_kind(v)){case WASM_I32:return 0x7f;case WASM_I64:return 0x7e;default:return 0;}}
extern "C" {
const char* wb_error(){return wb_last_error.c_str();}
const char* wb_version(){return WB_VERSION;}
void* wb_engine_new(){WB_TRY{auto e=std::make_unique<Engine>();if(!e->value)throw std::runtime_error("engine creation failed");return e.release();}WB_CATCH(nullptr)}
void wb_engine_delete(void* e){delete static_cast<Engine*>(e);}
void* wb_module_new(void* e,const uint8_t* data,size_t size){WB_TRY{
    auto m=wasm_module_new(static_cast<Engine*>(e)->value,reinterpret_cast<const char*>(data),size);
    if(!m)throw std::runtime_error("module compilation failed");
    if(wasm_module_num_imports(m)){wasm_module_delete(m);throw std::runtime_error("unsupported: import-free modules only");}return m;
}WB_CATCH(nullptr)}
void wb_module_delete(void* m){wasm_module_delete(static_cast<wasm_module_t*>(m));}
void* wb_instance_new(void* e,void* m){WB_TRY{
    auto i=std::make_unique<Instance>();i->module=static_cast<wasm_module_t*>(m);
    i->compartment=wasm_compartment_new(static_cast<Engine*>(e)->value,"wasmbench");if(!i->compartment)throw std::runtime_error("compartment creation failed");
    i->store=wasm_store_new(i->compartment,"wasmbench");if(!i->store)throw std::runtime_error("store creation failed");
    wasm_trap_t* trap=nullptr;i->value=wasm_instance_new(i->store,i->module,nullptr,&trap,"wasmbench");
    if(trap){wasm_trap_delete(trap);throw std::runtime_error("Wasm instantiation trap");}if(!i->value)throw std::runtime_error("instantiation failed");return i.release();
}WB_CATCH(nullptr)}
void wb_instance_delete(void* i){delete static_cast<Instance*>(i);}
int wb_signature(void* value,const char* name,uint8_t* params,size_t* np,uint8_t* results,size_t* nr){WB_TRY{
    auto f=function(static_cast<Instance*>(value),name);
    std::unique_ptr<wasm_functype_t,decltype(&wasm_functype_delete)> t(wasm_func_type(f),wasm_functype_delete);
    const size_t p=wasm_functype_num_params(t.get()),r=wasm_functype_num_results(t.get());
    if(p>*np||r>*nr)throw std::runtime_error("unsupported: signature exceeds 32 values");*np=p;*nr=r;
    for(size_t k=0;k<p;k++)params[k]=type(wasm_functype_param(t.get(),k));for(size_t k=0;k<r;k++)results[k]=type(wasm_functype_result(t.get(),k));return 0;
}WB_CATCH(-1)}
int wb_call(void* value,const char* name,const uint64_t* args,size_t na,uint64_t* out,size_t nr){WB_TRY{
    auto i=static_cast<Instance*>(value);auto f=function(i,name);
    std::unique_ptr<wasm_functype_t,decltype(&wasm_functype_delete)> t(wasm_func_type(f),wasm_functype_delete);
    if(na!=wasm_functype_num_params(t.get())||nr!=wasm_functype_num_results(t.get()))throw std::runtime_error("signature arity mismatch");
    std::vector<wasm_val_t> p(na),r(nr);
    for(size_t k=0;k<na;k++){const auto kind=type(wasm_functype_param(t.get(),k));if(kind==0x7f)p[k].i32=static_cast<int32_t>(args[k]);else if(kind==0x7e)p[k].i64=static_cast<int64_t>(args[k]);else throw std::runtime_error("unsupported: integer parameters only");}
    auto trap=wasm_func_call(i->store,f,p.data(),r.data());if(trap){wasm_trap_delete(trap);throw std::runtime_error("Wasm invocation trap");}
    for(size_t k=0;k<nr;k++){const auto kind=type(wasm_functype_result(t.get(),k));if(kind==0x7f)out[k]=static_cast<uint32_t>(r[k].i32);else if(kind==0x7e)out[k]=static_cast<uint64_t>(r[k].i64);else throw std::runtime_error("unsupported: integer results only");}return 0;
}WB_CATCH(-1)}
uint8_t* wb_memory(void* value,size_t* size){WB_TRY{
    auto m=wasm_extern_as_memory(find(static_cast<Instance*>(value),"memory"));if(!m)throw std::runtime_error("missing exported memory");*size=wasm_memory_data_size(m);return reinterpret_cast<uint8_t*>(wasm_memory_data(m));
}WB_CATCH(nullptr)}
}
