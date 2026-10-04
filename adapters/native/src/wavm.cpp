#include "embedding.h"
#include <WAVM/wavm-c/wavm-c.h>
#include <memory>
#include <WAVM/Runtime/Runtime.h>
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
static wasm_trap_t* host_identity(const wasm_val_t args[],wasm_val_t results[]){results[0].i32=args[0].i32;return nullptr;}
static thread_local wasm_compartment_t* callback_compartment=nullptr;
struct CallbackScope {
    wasm_compartment_t* prior;
    explicit CallbackScope(wasm_compartment_t* current):prior(callback_compartment){callback_compartment=current;}
    ~CallbackScope(){callback_compartment=prior;}
};
static wasm_trap_t* assemblyscript_abort(const wasm_val_t[],wasm_val_t[]){
    static const char message[]="AssemblyScript abort";
    auto* trap=wasm_trap_new(callback_compartment,message,sizeof(message)-1);
    if(!trap)std::terminate();
    return trap;
}
extern "C" {
// The diagnostic owns its returned object separately from timed C API modules.
int wb_object_code(const uint8_t* bytes,size_t size,uint8_t** output,size_t* output_size){WB_TRY{
    WAVM::Runtime::ModuleRef module;
    if(!WAVM::Runtime::loadBinaryModule(bytes,size,module))throw std::runtime_error("object compilation failed");
    auto object=WAVM::Runtime::getObjectCode(module);
    auto result=std::make_unique<uint8_t[]>(object.size());
    memcpy(result.get(),object.data(),object.size());
    *output_size=object.size();*output=result.release();return 0;
}WB_CATCH(-1)}
void wb_object_delete(uint8_t* bytes){delete[] bytes;}
const char* wb_error(){return wb_last_error.c_str();}
const char* wb_version(){return WB_VERSION;}
void* wb_engine_new(){WB_TRY{auto e=std::make_unique<Engine>();if(!e->value)throw std::runtime_error("engine creation failed");return e.release();}WB_CATCH(nullptr)}
void wb_engine_delete(void* e){delete static_cast<Engine*>(e);}
void* wb_module_new(void* e,const uint8_t* data,size_t size){WB_TRY{
    auto m=wasm_module_new(static_cast<Engine*>(e)->value,reinterpret_cast<const char*>(data),size);
    if(!m)throw std::runtime_error("module compilation failed");return m;
}WB_CATCH(nullptr)}
void wb_module_delete(void* m){wasm_module_delete(static_cast<wasm_module_t*>(m));}
void* wb_instance_new(void* e,void* m){WB_TRY{
    auto i=std::make_unique<Instance>();i->module=static_cast<wasm_module_t*>(m);
    i->compartment=wasm_compartment_new(static_cast<Engine*>(e)->value,"wasmbench");if(!i->compartment)throw std::runtime_error("compartment creation failed");
    i->store=wasm_store_new(i->compartment,"wasmbench");if(!i->store)throw std::runtime_error("store creation failed");
    const size_t count=wasm_module_num_imports(i->module);std::vector<wasm_extern_t*> imports;std::vector<wasm_func_t*> functions;
    for(size_t k=0;k<count;k++){
        wasm_import_t imported{};wasm_module_import(i->module,k,&imported);
        const bool identity=imported.num_module_bytes==9&&!memcmp(imported.module,"wasmbench",9)&&imported.num_name_bytes==8&&!memcmp(imported.name,"identity",8);
        // The typed cast is borrowed from imported.type. Delete that owner once
        // below; deleting both the cast and its owner double-frees WAVM metadata.
        const auto* functionType=wasm_externtype_as_functype_const(imported.type);
        const bool abort=imported.num_module_bytes==3&&!memcmp(imported.module,"env",3)&&imported.num_name_bytes==5&&!memcmp(imported.name,"abort",5);
        bool validIdentity=identity&&functionType&&wasm_functype_num_params(functionType)==1&&wasm_functype_num_results(functionType)==1&&wasm_valtype_kind(wasm_functype_param(functionType,0))==WASM_I32&&wasm_valtype_kind(wasm_functype_result(functionType,0))==WASM_I32;
        bool validAbort=abort&&functionType&&wasm_functype_num_params(functionType)==4&&wasm_functype_num_results(functionType)==0;
        if(validAbort)for(size_t n=0;n<4;n++)validAbort=validAbort&&wasm_valtype_kind(wasm_functype_param(functionType,n))==WASM_I32;
        if(!validIdentity&&!validAbort){wasm_externtype_delete(imported.type);throw std::runtime_error("unsupported: host import must match identity(i32)->i32 or env.abort(i32,i32,i32,i32)");}
        auto host=validIdentity?wasm_func_new(i->compartment,functionType,host_identity,"wasmbench.identity"):wasm_func_new(i->compartment,functionType,assemblyscript_abort,"env.abort");
        wasm_externtype_delete(imported.type);if(!host)throw std::runtime_error("identity callback creation failed");functions.push_back(host);imports.push_back(wasm_func_as_extern(host));
    }
    CallbackScope callbackScope(i->compartment);
    wasm_trap_t* trap=nullptr;i->value=wasm_instance_new(i->store,i->module,imports.data(),&trap,"wasmbench");
    for(auto* function:functions)wasm_func_delete(function);
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
    CallbackScope callbackScope(i->compartment);
    auto trap=wasm_func_call(i->store,f,p.data(),r.data());if(trap){wasm_trap_delete(trap);throw std::runtime_error("Wasm invocation trap");}
    for(size_t k=0;k<nr;k++){const auto kind=type(wasm_functype_result(t.get(),k));if(kind==0x7f)out[k]=static_cast<uint32_t>(r[k].i32);else if(kind==0x7e)out[k]=static_cast<uint64_t>(r[k].i64);else throw std::runtime_error("unsupported: integer results only");}return 0;
}WB_CATCH(-1)}
uint8_t* wb_memory(void* value,size_t* size){WB_TRY{
    auto m=wasm_extern_as_memory(find(static_cast<Instance*>(value),"memory"));if(!m)throw std::runtime_error("missing exported memory");*size=wasm_memory_data_size(m);return reinterpret_cast<uint8_t*>(wasm_memory_data(m));
}WB_CATCH(nullptr)}
}
