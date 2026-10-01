// WasmEdge interpreter embedding. Loading/validation is not AOT compilation.
#include "embedding.h"
#include <wasmedge/wasmedge.h>
#include <memory>
struct Engine {
    WasmEdge_ConfigureContext* config=WasmEdge_ConfigureCreate();
    WasmEdge_LoaderContext* loader=nullptr;
    WasmEdge_ValidatorContext* validator=nullptr;
    Engine(){if(!config)throw std::runtime_error("configuration creation failed");WasmEdge_ConfigureSetRunMode(config,WasmEdge_RunMode_Interpreter);loader=WasmEdge_LoaderCreate(config);validator=WasmEdge_ValidatorCreate(config);}
    ~Engine(){if(validator)WasmEdge_ValidatorDelete(validator);if(loader)WasmEdge_LoaderDelete(loader);if(config)WasmEdge_ConfigureDelete(config);}
};
struct Instance {
    WasmEdge_ExecutorContext* executor=nullptr;
    WasmEdge_StoreContext* store=nullptr;
    WasmEdge_ModuleInstanceContext* module=nullptr;
    ~Instance(){if(module)WasmEdge_ModuleInstanceDelete(module);if(store)WasmEdge_StoreDelete(store);if(executor)WasmEdge_ExecutorDelete(executor);}
};
static void check(WasmEdge_Result r){if(!WasmEdge_ResultOK(r))throw std::runtime_error(WasmEdge_ResultGetMessage(r));}
static const WasmEdge_FunctionInstanceContext* function(Instance* i,const char* name){auto f=WasmEdge_ModuleInstanceFindFunction(i->module,WasmEdge_StringWrap(name,static_cast<uint32_t>(strlen(name))));if(!f)throw std::runtime_error("missing export");return f;}
static uint8_t type(WasmEdge_ValType t){return WasmEdge_ValTypeIsI32(t)?0x7f:WasmEdge_ValTypeIsI64(t)?0x7e:0;}
static std::pair<std::vector<WasmEdge_ValType>,std::vector<WasmEdge_ValType>> signature(const WasmEdge_FunctionInstanceContext* f){
    auto t=WasmEdge_FunctionInstanceGetFunctionType(f);std::vector<WasmEdge_ValType> p(WasmEdge_FunctionTypeGetParametersLength(t)),r(WasmEdge_FunctionTypeGetReturnsLength(t));
    WasmEdge_FunctionTypeGetParameters(t,p.data(),static_cast<uint32_t>(p.size()));WasmEdge_FunctionTypeGetReturns(t,r.data(),static_cast<uint32_t>(r.size()));return {p,r};
}
extern "C" {
const char* wb_error(){return wb_last_error.c_str();}
const char* wb_version(){return WasmEdge_VersionGet();}
void* wb_engine_new(){WB_TRY{auto e=std::make_unique<Engine>();if(!e->loader||!e->validator)throw std::runtime_error("loader/validator creation failed");return e.release();}WB_CATCH(nullptr)}
void wb_engine_delete(void* e){delete static_cast<Engine*>(e);}
void* wb_module_new(void* value,const uint8_t* data,size_t size){WB_TRY{
    if(size>UINT32_MAX)throw std::runtime_error("module too large");auto e=static_cast<Engine*>(value);WasmEdge_ASTModuleContext* m=nullptr;
    auto result=WasmEdge_LoaderParseFromBuffer(e->loader,&m,data,static_cast<uint32_t>(size));
    if(!WasmEdge_ResultOK(result)){if(m)WasmEdge_ASTModuleDelete(m);check(result);}
    result=WasmEdge_ValidatorValidate(e->validator,m);if(!WasmEdge_ResultOK(result)){WasmEdge_ASTModuleDelete(m);check(result);}return m;
}WB_CATCH(nullptr)}
void wb_module_delete(void* m){WasmEdge_ASTModuleDelete(static_cast<WasmEdge_ASTModuleContext*>(m));}
void* wb_instance_new(void* value,void* m){WB_TRY{
    auto e=static_cast<Engine*>(value);auto i=std::make_unique<Instance>();i->executor=WasmEdge_ExecutorCreate(e->config,nullptr);i->store=WasmEdge_StoreCreate();if(!i->executor||!i->store)throw std::runtime_error("executor/store creation failed");
    check(WasmEdge_ExecutorInstantiate(i->executor,&i->module,i->store,static_cast<WasmEdge_ASTModuleContext*>(m)));return i.release();
}WB_CATCH(nullptr)}
void wb_instance_delete(void* i){delete static_cast<Instance*>(i);}
int wb_signature(void* value,const char* name,uint8_t* params,size_t* np,uint8_t* results,size_t* nr){WB_TRY{
    auto [p,r]=signature(function(static_cast<Instance*>(value),name));if(p.size()>*np||r.size()>*nr)throw std::runtime_error("unsupported: signature exceeds 32 values");*np=p.size();*nr=r.size();
    for(size_t k=0;k<p.size();k++)params[k]=type(p[k]);for(size_t k=0;k<r.size();k++)results[k]=type(r[k]);return 0;
}WB_CATCH(-1)}
int wb_call(void* value,const char* name,const uint64_t* args,size_t na,uint64_t* out,size_t nr){WB_TRY{
    auto i=static_cast<Instance*>(value);auto f=function(i,name);auto [p,r]=signature(f);if(na!=p.size()||nr!=r.size())throw std::runtime_error("signature arity mismatch");
    std::vector<WasmEdge_Value> params(na),results(nr);
    for(size_t k=0;k<na;k++){const auto kind=type(p[k]);if(kind==0x7f)params[k]=WasmEdge_ValueGenI32(static_cast<int32_t>(args[k]));else if(kind==0x7e)params[k]=WasmEdge_ValueGenI64(static_cast<int64_t>(args[k]));else throw std::runtime_error("unsupported: integer parameters only");}
    check(WasmEdge_ExecutorInvoke(i->executor,f,params.data(),static_cast<uint32_t>(na),results.data(),static_cast<uint32_t>(nr)));
    for(size_t k=0;k<nr;k++){const auto kind=type(r[k]);if(kind==0x7f)out[k]=static_cast<uint32_t>(WasmEdge_ValueGetI32(results[k]));else if(kind==0x7e)out[k]=static_cast<uint64_t>(WasmEdge_ValueGetI64(results[k]));else throw std::runtime_error("unsupported: integer results only");}return 0;
}WB_CATCH(-1)}
uint8_t* wb_memory(void* value,size_t* n){WB_TRY{
    auto i=static_cast<Instance*>(value);auto m=WasmEdge_ModuleInstanceFindMemory(i->module,WasmEdge_StringWrap("memory",6));if(!m)throw std::runtime_error("missing exported memory");
    const auto pages=WasmEdge_MemoryInstanceGetPageSize(m);const uint64_t size=static_cast<uint64_t>(pages)*65536;if(size>UINT32_MAX)throw std::runtime_error("unsupported: memory >=4GiB");*n=static_cast<size_t>(size);return WasmEdge_MemoryInstanceGetPointer(m,0,static_cast<uint32_t>(size));
}WB_CATCH(nullptr)}
}
