#include "embedding.h"
#include <wasm_c_api.h>
#include <wasm_export.h>
#include <memory>
// WAMR's native ABI passes the execution environment as the hidden first arg.
static int32_t host_identity(wasm_exec_env_t, int32_t value){return value;}
static NativeSymbol host_symbols[]={{"identity",reinterpret_cast<void*>(host_identity),"(i)i",nullptr}};
// WAMR 2.4.5 classic interpreter. A compilation owns its C-API store.
struct Engine { wasm_engine_t* value=wasm_engine_new(); Engine(){wasm_runtime_set_log_level(WASM_LOG_LEVEL_FATAL);if(!wasm_runtime_register_natives("wasmbench",host_symbols,1))throw std::runtime_error("identity host function registration failed");} ~Engine(){if(value)wasm_engine_delete(value);} };
struct Module {
    wasm_store_t* store=nullptr; wasm_module_t* value=nullptr;
    ~Module(){if(value)wasm_module_delete(value);if(store)wasm_store_delete(store);}
};
struct Instance {
    Module* module=nullptr; wasm_instance_t* value=nullptr;
    wasm_extern_vec_t exports{}; wasm_exporttype_vec_t types{};
    ~Instance(){wasm_extern_vec_delete(&exports);wasm_exporttype_vec_delete(&types);if(value)wasm_instance_delete(value);}
};
static wasm_extern_t* find(Instance* i,const char* name){
    for(size_t k=0;k<i->types.num_elems;k++){
        auto n=wasm_exporttype_name(i->types.data[k]);
        size_t length=n->num_elems;if(length&&n->data[length-1]==0)--length;
        if(length==strlen(name)&&memcmp(n->data,name,length)==0)return i->exports.data[k];
    }throw std::runtime_error("missing export");
}
static wasm_func_t* function(Instance* i,const char* name){auto f=wasm_extern_as_func(find(i,name));if(!f)throw std::runtime_error("export is not a function");return f;}
static uint8_t type(const wasm_valtype_t* v){switch(wasm_valtype_kind(v)){case WASM_I32:return 0x7f;case WASM_I64:return 0x7e;default:return 0;}}
static void trap_error(wasm_trap_t* trap){wasm_message_t message{};wasm_trap_message(trap,&message);std::string text(message.data,message.size);wasm_byte_vec_delete(&message);wasm_trap_delete(trap);throw std::runtime_error(text);}
extern "C" {
const char* wb_error(){return wb_last_error.c_str();}
const char* wb_version(){return WB_VERSION;}
void* wb_engine_new(){WB_TRY{auto e=std::make_unique<Engine>();if(!e->value)throw std::runtime_error("engine creation failed");return e.release();}WB_CATCH(nullptr)}
void wb_engine_delete(void* e){delete static_cast<Engine*>(e);}
void* wb_module_new(void* e,const uint8_t* bytes,size_t n){WB_TRY{
    auto m=std::make_unique<Module>();m->store=wasm_store_new(static_cast<Engine*>(e)->value);if(!m->store)throw std::runtime_error("store creation failed");
    wasm_byte_vec_t input{n,const_cast<char*>(reinterpret_cast<const char*>(bytes)),n,1,nullptr};
    m->value=wasm_module_new(m->store,&input);if(!m->value)throw std::runtime_error("module validation/loading rejected input");return m.release();
}WB_CATCH(nullptr)}
void wb_module_delete(void* m){delete static_cast<Module*>(m);}
void* wb_instance_new(void*,void* m){WB_TRY{
    auto i=std::make_unique<Instance>();i->module=static_cast<Module*>(m);wasm_trap_t* trap=nullptr;wasm_extern_vec_t imports{};
    i->value=wasm_instance_new_with_args(i->module->store,i->module->value,&imports,&trap,1<<20,0);if(trap)trap_error(trap);if(!i->value)throw std::runtime_error("instantiation failed");
    wasm_instance_exports(i->value,&i->exports);wasm_module_exports(i->module->value,&i->types);if(i->exports.num_elems!=i->types.num_elems)throw std::runtime_error("export inventory mismatch");return i.release();
}WB_CATCH(nullptr)}
void wb_instance_delete(void* i){delete static_cast<Instance*>(i);}
int wb_signature(void* i,const char* name,uint8_t* params,size_t* np,uint8_t* results,size_t* nr){WB_TRY{
    std::unique_ptr<wasm_functype_t,decltype(&wasm_functype_delete)> t(wasm_func_type(function(static_cast<Instance*>(i),name)),wasm_functype_delete);
    auto p=wasm_functype_params(t.get()),r=wasm_functype_results(t.get());if(p->num_elems>*np||r->num_elems>*nr)throw std::runtime_error("unsupported: signature exceeds 32 values");*np=p->num_elems;*nr=r->num_elems;
    for(size_t k=0;k<p->num_elems;k++)params[k]=type(p->data[k]);for(size_t k=0;k<r->num_elems;k++)results[k]=type(r->data[k]);return 0;
}WB_CATCH(-1)}
int wb_call(void* i,const char* name,const uint64_t* args,size_t na,uint64_t* out,size_t nr){WB_TRY{
    auto f=function(static_cast<Instance*>(i),name);std::unique_ptr<wasm_functype_t,decltype(&wasm_functype_delete)> t(wasm_func_type(f),wasm_functype_delete);
    auto pt=wasm_functype_params(t.get()),rt=wasm_functype_results(t.get());if(pt->num_elems!=na||rt->num_elems!=nr)throw std::runtime_error("signature arity mismatch");
    std::vector<wasm_val_t> p(na),r(nr);for(size_t k=0;k<na;k++){p[k].kind=wasm_valtype_kind(pt->data[k]);if(p[k].kind==WASM_I32)p[k].of.i32=static_cast<int32_t>(args[k]);else if(p[k].kind==WASM_I64)p[k].of.i64=static_cast<int64_t>(args[k]);else throw std::runtime_error("unsupported: integer parameters only");}
    wasm_val_vec_t pv{na,p.data(),na,sizeof(wasm_val_t),nullptr},rv{nr,r.data(),nr,sizeof(wasm_val_t),nullptr};auto trap=wasm_func_call(f,&pv,&rv);if(trap)trap_error(trap);
    for(size_t k=0;k<nr;k++){if(r[k].kind==WASM_I32)out[k]=static_cast<uint32_t>(r[k].of.i32);else if(r[k].kind==WASM_I64)out[k]=static_cast<uint64_t>(r[k].of.i64);else throw std::runtime_error("unsupported: integer results only");}return 0;
}WB_CATCH(-1)}
uint8_t* wb_memory(void* i,size_t* n){WB_TRY{auto m=wasm_extern_as_memory(find(static_cast<Instance*>(i),"memory"));if(!m)throw std::runtime_error("missing exported memory");*n=wasm_memory_data_size(m);return reinterpret_cast<uint8_t*>(wasm_memory_data(m));}WB_CATCH(nullptr)}
}
