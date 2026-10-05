// Wasmer C API embedding: explicit compiler, fresh store for every instance.
#include "embedding.h"
#include <wasmer.h>
#include <memory>
#include <dlfcn.h>

static std::runtime_error last_error(const char* phase) {
    int length=wasmer_last_error_length();
    std::string message;
    if(length>0 && length<1024*1024) {
        message.resize(static_cast<size_t>(length));
        wasmer_last_error_message(message.data(),length);
        if(!message.empty() && message.back()=='\0')message.pop_back();
    }
    return std::runtime_error(std::string(phase)+": "+message);
}
static std::runtime_error trap_error(wasm_trap_t* trap) {
    wasm_message_t message;wasm_trap_message(trap,&message);
    std::string text(message.data,message.size);
    wasm_byte_vec_delete(&message);wasm_trap_delete(trap);
    return std::runtime_error("guest trap: "+text);
}
struct Engine {
    wasm_engine_t* engine=nullptr;
    ~Engine(){if(engine)wasm_engine_delete(engine);}
};
#include "wasmer_wasi.h"
struct Instance {
    wasm_store_t* store=nullptr;
    wasm_instance_t* instance=nullptr;
    wasm_extern_vec_t exports=WASM_EMPTY_VEC;
    std::vector<std::string> names;
    ~Instance(){wasm_extern_vec_delete(&exports);if(instance)wasm_instance_delete(instance);if(store)wasm_store_delete(store);}
};
static wasm_extern_t* lookup(Instance* i,const char* name) {
    for(size_t k=0;k<i->names.size();k++)if(i->names[k]==name)return i->exports.data[k];
    throw std::runtime_error("missing export: "+std::string(name));
}
static wasm_func_t* function(Instance* i,const char* name) {
    auto f=wasm_extern_as_func(lookup(i,name));if(!f)throw std::runtime_error("export is not a function");return f;
}
static uint8_t kind(const wasm_valtype_t* t) {
    auto k=wasm_valtype_kind(t);return k==WASM_I32?0x7f:k==WASM_I64?0x7e:0;
}
static wasm_trap_t* assemblyscript_abort(void* env,const wasm_val_vec_t*,wasm_val_vec_t*) {
    static const char text[]="AssemblyScript abort";
    const wasm_message_t message={sizeof(text)-1,const_cast<char*>(text)};
    auto trap=wasm_trap_new(static_cast<wasm_store_t*>(env),&message);
    if(!trap)std::abort();
    return trap;
}
static wasm_trap_t* identity(void*,const wasm_val_vec_t* args,wasm_val_vec_t* results) {
    if(args->size!=1||results->size!=1||args->data[0].kind!=WASM_I32)return nullptr;
    results->data[0].kind=WASM_I32;results->data[0].of.i32=args->data[0].of.i32;return nullptr;
}
using NativeSizeGetter=bool(*)(const wasm_module_t*,size_t*);
static NativeSizeGetter native_size_getter(){return reinterpret_cast<NativeSizeGetter>(dlsym(RTLD_DEFAULT,"wasmbench_module_native_function_size"));}
extern "C" {
bool wb_can_wasi(){
#ifdef WASMER_WASI_ENABLED
    return true;
#else
    return false;
#endif
}
#ifdef WASMER_WASI_ENABLED
void* wb_wasi_module_new(const uint8_t* bytes,size_t size){WB_TRY{return new wb_wasmer_wasi::Module(bytes,size);}WB_CATCH(nullptr)}
void wb_wasi_module_delete(void* module){delete static_cast<wb_wasmer_wasi::Module*>(module);}
void* wb_wasi_instance_new(void* module,const char* const* argv,size_t argc,const char* root,const uint8_t* input,size_t inputSize,size_t limit){WB_TRY{
    return wb_wasmer_wasi::Command::create(*static_cast<wb_wasmer_wasi::Module*>(module),argv,argc,root,input,inputSize,limit).release();
}WB_CATCH(nullptr)}
void wb_wasi_instance_delete(void* instance){delete static_cast<wb_wasmer_wasi::Command*>(instance);}
int wb_wasi_run(void* instance,uint32_t* exit){WB_TRY{*exit=static_cast<wb_wasmer_wasi::Command*>(instance)->run();return 0;}WB_CATCH(-1)}
uint64_t wb_wasi_memory_bytes(void* instance){return static_cast<wb_wasmer_wasi::Command*>(instance)->memoryBytes();}
const uint8_t* wb_wasi_output(void* instance,bool error,size_t* size){
    *size=SIZE_MAX;
    WB_TRY {
        auto& command=*static_cast<wb_wasmer_wasi::Command*>(instance);
        auto& bytes=error?command.stderrBytes:command.stdoutBytes;
        command.capture(error?2:1,bytes);
        *size=bytes.size();return bytes.data();
    } WB_CATCH(nullptr)
}
#else
void* wb_wasi_module_new(const uint8_t*,size_t){wb_last_error="unsupported: selected Wasmer SDK has no WASI command binding";return nullptr;}
void wb_wasi_module_delete(void*){}
void* wb_wasi_instance_new(void*,const char* const*,size_t,const char*,const uint8_t*,size_t,size_t){wb_last_error="unsupported: selected Wasmer SDK has no WASI command binding";return nullptr;}
void wb_wasi_instance_delete(void*){}
int wb_wasi_run(void*,uint32_t*){return -1;}
uint64_t wb_wasi_memory_bytes(void*){return 0;}
const uint8_t* wb_wasi_output(void*,bool,size_t* size){*size=0;return nullptr;}
#endif
bool wb_can_native_size(){return native_size_getter()!=nullptr;}
int wb_native_size(void* module,size_t* size){WB_TRY{
    auto getter=native_size_getter();
    if(!getter)throw std::runtime_error("unsupported: selected Wasmer SDK has no native-size getter");
    if(!getter(static_cast<const wasm_module_t*>(module),size))throw std::runtime_error("native function extents unavailable or incomplete");
    return 0;
}WB_CATCH(-1)}
const char* wb_error(){return wb_last_error.c_str();}
const char* wb_version(){return wasmer_version();}
void* wb_engine_new(){WB_TRY{
    const auto backend=WB_WASMER_LLVM?LLVM:SINGLEPASS;
    if(!wasmer_is_backend_available(backend))throw std::runtime_error("unsupported: selected Wasmer compiler is not built into SDK");
    auto e=std::make_unique<Engine>();auto config=wasm_config_new();
    if(!config)throw last_error("configuration");wasm_config_set_backend(config,backend);
    auto features=wasmer_features_new();if(!features)throw last_error("features");
    // Singlepass lacks SIMD lowering for the corpus operations. Select an
    // explicit conservative subset rather than claim its default SIMD flag.
    wasmer_features_simd(features,WB_WASMER_LLVM);
    wasmer_features_relaxed_simd(features,WB_WASMER_LLVM);
    wasmer_features_exceptions(features,WB_WASMER_LLVM);
    wasmer_features_tail_call(features,WB_WASMER_LLVM);
    wasmer_features_memory64(features,false);
    wasm_config_set_features(config,features);
    e->engine=wasm_engine_new_with_config(config);if(!e->engine)throw last_error("engine");return e.release();
}WB_CATCH(nullptr)}
void wb_engine_delete(void* e){delete static_cast<Engine*>(e);}
void* wb_module_new(void* value,const uint8_t* bytes,size_t size){WB_TRY{
    auto e=static_cast<Engine*>(value);
    std::unique_ptr<wasm_store_t,decltype(&wasm_store_delete)> store(wasm_store_new(e->engine),wasm_store_delete);
    if(!store)throw last_error("compilation store");
    const wasm_byte_vec_t binary={size,const_cast<char*>(reinterpret_cast<const char*>(bytes))};
    auto m=wasm_module_new(store.get(),&binary);if(!m)throw last_error("compile");return m;
}WB_CATCH(nullptr)}
void wb_module_delete(void* m){wasm_module_delete(static_cast<wasm_module_t*>(m));}
void* wb_instance_new(void* value,void* module){WB_TRY{
    auto e=static_cast<Engine*>(value);auto m=static_cast<wasm_module_t*>(module);auto i=std::make_unique<Instance>();
    i->store=wasm_store_new(e->engine);if(!i->store)throw last_error("instance store");
    wasm_importtype_vec_t imported;wasm_module_imports(m,&imported);
    std::unique_ptr<wasm_importtype_vec_t,decltype(&wasm_importtype_vec_delete)> imported_owner(&imported,wasm_importtype_vec_delete);
    std::vector<std::unique_ptr<wasm_func_t,decltype(&wasm_func_delete)>> functions;
    std::vector<wasm_extern_t*> values;
    for(size_t k=0;k<imported.size;k++){
        const auto name=wasm_importtype_name(imported.data[k]),space=wasm_importtype_module(imported.data[k]);
        const std::string moduleName(space->data,space->size),fieldName(name->data,name->size);
        const auto type=wasm_externtype_as_functype_const(wasm_importtype_type(imported.data[k]));
        if(!type)throw std::runtime_error("unsupported: host imports must be functions");
        const auto params=wasm_functype_params(type),results=wasm_functype_results(type);
        if(moduleName=="wasmbench" && fieldName=="identity") {
            if(params->size!=1||results->size!=1||wasm_valtype_kind(params->data[0])!=WASM_I32||wasm_valtype_kind(results->data[0])!=WASM_I32)throw std::runtime_error("unsupported: wasmbench.identity requires (i32)->i32");
            // Wasmer rejects a null callback environment; the identity callback
            // ignores its environment, so retain the owning store as a valid token.
            auto f=wasm_func_new_with_env(i->store,type,identity,i->store,nullptr);
            if(!f)throw last_error("identity callback");functions.emplace_back(f,wasm_func_delete);values.push_back(wasm_func_as_extern(f));continue;
        }
        if(moduleName!="env" || fieldName!="abort")throw std::runtime_error("unsupported: unknown host import");
        if(params->size!=4 || results->size!=0)throw std::runtime_error("unsupported: env.abort signature mismatch");
        for(size_t n=0;n<params->size;n++)if(wasm_valtype_kind(params->data[n])!=WASM_I32)throw std::runtime_error("unsupported: env.abort requires four i32 parameters");
        auto f=wasm_func_new_with_env(i->store,type,assemblyscript_abort,i->store,nullptr);
        if(!f)throw last_error("abort callback");functions.emplace_back(f,wasm_func_delete);values.push_back(wasm_func_as_extern(f));
    }
    wasm_extern_vec_t imports={values.size(),values.data()};wasm_trap_t* trap=nullptr;
    i->instance=wasm_instance_new(i->store,m,&imports,&trap);
    if(trap)throw trap_error(trap);if(!i->instance)throw last_error("instantiate");
    wasm_instance_exports(i->instance,&i->exports);
    wasm_exporttype_vec_t types;wasm_module_exports(m,&types);
    for(size_t k=0;k<types.size;k++){const auto name=wasm_exporttype_name(types.data[k]);i->names.emplace_back(name->data,name->size);}
    wasm_exporttype_vec_delete(&types);
    if(i->names.size()!=i->exports.size)throw std::runtime_error("export metadata mismatch");return i.release();
}WB_CATCH(nullptr)}
void wb_instance_delete(void* i){delete static_cast<Instance*>(i);}
int wb_signature(void* value,const char* name,uint8_t* params,size_t* np,uint8_t* results,size_t* nr){WB_TRY{
    std::unique_ptr<wasm_functype_t,decltype(&wasm_functype_delete)> t(wasm_func_type(function(static_cast<Instance*>(value),name)),wasm_functype_delete);
    const auto p=wasm_functype_params(t.get()),r=wasm_functype_results(t.get());
    if(p->size>*np||r->size>*nr)throw std::runtime_error("unsupported: signature exceeds 32 values");
    *np=p->size;*nr=r->size;for(size_t k=0;k<*np;k++)params[k]=kind(p->data[k]);for(size_t k=0;k<*nr;k++)results[k]=kind(r->data[k]);return 0;
}WB_CATCH(-1)}
int wb_call(void* value,const char* name,const uint64_t* args,size_t na,uint64_t* out,size_t nr){WB_TRY{
    auto f=function(static_cast<Instance*>(value),name);
    std::unique_ptr<wasm_functype_t,decltype(&wasm_functype_delete)> t(wasm_func_type(f),wasm_functype_delete);
    const auto p=wasm_functype_params(t.get()),r=wasm_functype_results(t.get());
    if(na!=p->size||nr!=r->size)throw std::runtime_error("signature arity mismatch");
    std::vector<wasm_val_t> params(na),results(nr);
    for(size_t k=0;k<na;k++){auto type=kind(p->data[k]);if(type==0x7f){params[k].kind=WASM_I32;params[k].of.i32=static_cast<int32_t>(args[k]);}else if(type==0x7e){params[k].kind=WASM_I64;params[k].of.i64=static_cast<int64_t>(args[k]);}else throw std::runtime_error("unsupported: integer parameters only");}
    wasm_val_vec_t pv={na,params.data()},rv={nr,results.data()};auto trap=wasm_func_call(f,&pv,&rv);if(trap)throw trap_error(trap);
    for(size_t k=0;k<nr;k++){if(results[k].kind==WASM_I32)out[k]=static_cast<uint32_t>(results[k].of.i32);else if(results[k].kind==WASM_I64)out[k]=static_cast<uint64_t>(results[k].of.i64);else throw std::runtime_error("unsupported: integer results only");}return 0;
}WB_CATCH(-1)}
uint8_t* wb_memory(void* value,size_t* n){WB_TRY{
    auto m=wasm_extern_as_memory(lookup(static_cast<Instance*>(value),"memory"));if(!m)throw std::runtime_error("export is not a memory");
    *n=wasm_memory_data_size(m);return reinterpret_cast<uint8_t*>(wasm_memory_data(m));
}WB_CATCH(nullptr)}
}
