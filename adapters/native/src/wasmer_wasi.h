// WASI command binding for the receipt-pinned Wasmer SDK.
// Included after Engine and last_error in wasmer.cpp.
#pragma once
#ifdef WASMER_WASI_ENABLED
extern "C" {
struct Streams;
wasi_config_t* wasmbench_wasi_config_new(const char*);
Streams* wasmbench_wasi_config_stdio(wasi_config_t*,const uint8_t*,size_t,size_t);
bool wasmbench_wasi_config_readonly_dir(wasi_config_t*,const char*);
void wasmbench_wasi_stdio_delete(Streams*);
bool wasmbench_wasi_stdio_overflow(const Streams*);
size_t wasmbench_wasi_stdio_copy(const Streams*,uint32_t,uint8_t*,size_t);
bool wasmbench_wasi_trap_exit_code(const wasm_trap_t*,uint32_t*);
bool wasmbench_wasi_initialize_instance(wasi_env_t*,wasm_store_t*,const wasm_instance_t*);
}
namespace wb_wasmer_wasi {
struct Module {
    void* engine=nullptr;
    wasm_module_t* module=nullptr;
    explicit Module(const uint8_t* bytes,size_t size) {
        engine=wb_engine_new();
        if(!engine)throw std::runtime_error(wb_error());
        module=static_cast<wasm_module_t*>(wb_module_new(engine,bytes,size));
        if(!module){wb_engine_delete(engine);engine=nullptr;throw std::runtime_error(wb_error());}
    }
    ~Module(){if(module)wasm_module_delete(module);if(engine)wb_engine_delete(engine);}
    Module(const Module&)=delete;
    Module& operator=(const Module&)=delete;
};
struct Command {
    wasm_store_t* store=nullptr;
    wasi_env_t* env=nullptr;
    wasm_instance_t* instance=nullptr;
    wasm_func_t* start=nullptr;
    Streams* streams=nullptr;
    wasm_extern_vec_t exports=WASM_EMPTY_VEC;
    std::vector<uint8_t> stdoutBytes,stderrBytes;
    Command()=default;
    Command(const Command&)=delete;
    Command& operator=(const Command&)=delete;
    ~Command(){
        if(start)wasm_func_delete(start);
        wasm_extern_vec_delete(&exports);
        if(instance)wasm_instance_delete(instance);
        if(env)wasi_env_delete(env);
        if(store)wasm_store_delete(store);
        if(streams)wasmbench_wasi_stdio_delete(streams);
    }
    static std::unique_ptr<Command> create(Module& module,const char* const* argv,size_t argc,
                                         const char* root,const uint8_t* input,size_t inputSize,size_t limit){
        if(!argc)throw std::runtime_error("WASI argv must include a program name");
        auto command=std::make_unique<Command>();
        command->store=wasm_store_new(static_cast<Engine*>(module.engine)->engine);
        if(!command->store)throw last_error("WASI store");
        // wasi_env_new consumes config even if environment construction fails.
        auto config=wasmbench_wasi_config_new(argv[0]);
        if(!config)throw std::runtime_error("WASI configuration failed");
        for(size_t index=1;index<argc;index++)wasi_config_arg(config,argv[index]);
        command->streams=wasmbench_wasi_config_stdio(config,input,inputSize,limit);
        const bool ready=command->streams && wasmbench_wasi_config_readonly_dir(config,root);
        command->env=wasi_env_new(command->store,config);
        if(!ready)throw std::runtime_error("WASI bounded streams or read-only preopen failed");
        if(!command->env)throw last_error("WASI environment");
        wasm_extern_vec_t imports=WASM_EMPTY_VEC;
        if(!wasi_get_imports(command->store,command->env,module.module,&imports)){
            wasm_extern_vec_delete(&imports);throw last_error("WASI imports");
        }
        wasm_trap_t* trap=nullptr;
        command->instance=wasm_instance_new(command->store,module.module,&imports,&trap);
        wasm_extern_vec_delete(&imports);
        if(trap)throw trap_error(trap);
        if(!command->instance)throw last_error("WASI instantiate");
        if(!wasmbench_wasi_initialize_instance(command->env,command->store,command->instance))
            throw last_error("WASI initialize");
        command->start=wasi_get_start_function(command->instance);
        if(!command->start)throw last_error("WASI _start");
        wasm_instance_exports(command->instance,&command->exports);
        return command;
    }
    void capture(uint32_t fd,std::vector<uint8_t>& bytes){
        const auto size=wasmbench_wasi_stdio_copy(streams,fd,nullptr,0);
        if(size==SIZE_MAX)throw std::runtime_error("WASI output size failed");
        bytes.resize(size);
        if(size && wasmbench_wasi_stdio_copy(streams,fd,bytes.data(),size)!=size)
            throw std::runtime_error("WASI output copy failed");
    }
    uint32_t run(){
        uint32_t exit=0;
        wasm_val_vec_t args=WASM_EMPTY_VEC,results=WASM_EMPTY_VEC;
        auto trap=wasm_func_call(start,&args,&results);
        if(trap){
            if(!wasmbench_wasi_trap_exit_code(trap,&exit))throw trap_error(trap);
            wasm_trap_delete(trap);
        }
        if(wasmbench_wasi_stdio_overflow(streams))throw std::runtime_error("WASI output budget exceeded");
        return exit;
    }
    uint64_t memoryBytes() const {
        uint64_t total=0;
        for(size_t index=0;index<exports.size;index++){
            auto memory=wasm_extern_as_memory(exports.data[index]);
            if(memory){auto size=wasm_memory_data_size(memory);if(size>UINT64_MAX-total)throw std::runtime_error("WASI memory size overflow");total+=size;}
        }
        return total;
    }
};
}
#endif
