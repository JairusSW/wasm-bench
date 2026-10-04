#pragma once
#include "wavm_stdio.h"
#include "wavm_readonly.h"
#include <WAVM/Runtime/Runtime.h>
#include <WAVM/Runtime/Linker.h>
#include <WAVM/Runtime/Intrinsics.h>
#include <WAVM/WASI/WASI.h>
#include <stdexcept>

namespace wb_wasi {
using namespace WAVM;
WAVM_DEFINE_INTRINSIC_MODULE(wasmbench_readonly_wasi)
WAVM_DEFINE_INTRINSIC_FUNCTION(wasmbench_readonly_wasi,"sock_accept",U32,deny_sock_accept,U32,U32,U32){return 76;}
class ReadOnlyResolver final: public Runtime::Resolver {
    Runtime::Resolver& delegate;
    Runtime::Instance* denied;
public:
    ReadOnlyResolver(Runtime::Resolver& original,Runtime::Instance* deniedImports):delegate(original),denied(deniedImports){}
    bool resolve(const std::string& module,const std::string& name,IR::ExternType type,Runtime::Object*& out) override {
        if(delegate.resolve(module,name,type,out))return true;
        if(module!="wasi_snapshot_preview1"||name!="sock_accept")return false;
        out=Runtime::getInstanceExport(denied,name);return out&&Runtime::isA(out,type);
    }
};
// Public embedding APIs only. This owns the per-instance WASI lifecycle;
// compilation is supplied separately so timing boundaries can stay distinct.
class CommandInstance {
    WAVM::Runtime::GCPointer<WAVM::Runtime::Compartment> compartment;
    WAVM::Runtime::GCPointer<WAVM::Runtime::Context> context;
    WAVM::Runtime::GCPointer<WAVM::Runtime::Instance> instance;
    WAVM::Runtime::GCPointer<WAVM::Runtime::Instance> deniedNetwork;
    std::unique_ptr<ReadOnlyFS> filesystem;
    std::shared_ptr<WAVM::WASI::Process> process;
    std::shared_ptr<StreamBuffer> input,output,error;
    void release(){
        instance=nullptr;deniedNetwork=nullptr;context=nullptr;process.reset();filesystem.reset();
        if(compartment)WAVM::Runtime::tryCollectCompartment(std::move(compartment));
    }
    static void checked(const std::function<void()>& thunk){
        std::string message;
        WAVM::Runtime::catchRuntimeExceptions(thunk,[&](WAVM::Runtime::Exception* trap){
            message=WAVM::Runtime::describeException(trap);
            WAVM::Runtime::destroyException(trap);
        });
        if(!message.empty())throw std::runtime_error(message);
    }
public:
    CommandInstance(WAVM::Runtime::ModuleRef module,std::vector<std::string> args,
                    const std::string& root,const std::vector<WAVM::U8>& stdinBytes,size_t outputLimit){
        try {
        compartment=WAVM::Runtime::createCompartment("wasmbench-wasi");
        context=WAVM::Runtime::createContext(compartment,"wasmbench-wasi");
        filesystem=std::make_unique<ReadOnlyFS>(root);
        input=std::make_shared<StreamBuffer>();input->bytes=stdinBytes;
        output=std::make_shared<StreamBuffer>();output->limit=outputLimit;
        error=std::make_shared<StreamBuffer>();error->limit=outputLimit;
        checked([&]{
            process=WAVM::WASI::createProcess(compartment,std::move(args),{},filesystem.get(),
                     new MemoryFD(input,false),new MemoryFD(output,true),new MemoryFD(error,true));
            deniedNetwork=WAVM::Intrinsics::instantiateModule(compartment,{WAVM_INTRINSIC_MODULE_REF(wasmbench_readonly_wasi)},"wasmbench-readonly-network");
            ReadOnlyResolver resolver(WAVM::WASI::getProcessResolver(*process),deniedNetwork);
            auto linked=WAVM::Runtime::linkModule(WAVM::Runtime::getModuleIR(module),resolver);
            if(!linked.success){
                std::string missing;for(const auto& imported:linked.missingImports){if(!missing.empty())missing+=", ";missing+=imported.moduleName+"."+imported.exportName;}
                throw std::runtime_error("WASI imports failed to link: "+missing);
            }
            instance=WAVM::Runtime::instantiateModule(compartment,module,std::move(linked.resolvedImports),"wasmbench-wasi");
            auto memory=WAVM::Runtime::asMemoryNullable(WAVM::Runtime::getInstanceExport(instance,"memory"));
            if(!memory)throw std::runtime_error("WASI command must export memory");
            WAVM::WASI::setProcessMemory(*process,memory);
            if(auto start=WAVM::Runtime::getStartFunction(instance))WAVM::Runtime::invokeFunction(context,start);
        });
        }catch(...){release();throw;}
    }
    ~CommandInstance(){release();}
    int run(){
        int code=0;
        checked([&]{code=WAVM::WASI::catchExit([&]{
            auto function=WAVM::Runtime::asFunctionNullable(WAVM::Runtime::getInstanceExport(instance,"_start"));
            if(!function)throw std::runtime_error("missing WASI _start export");
            WAVM::Runtime::invokeFunction(context,function);return 0;
        });});
        if(output->overflow||error->overflow)throw std::runtime_error("WASI output limit exceeded");
        return code;
    }
    uint64_t memoryBytes() const {auto memory=WAVM::Runtime::asMemoryNullable(WAVM::Runtime::getInstanceExport(instance,"memory"));return uint64_t(WAVM::Runtime::getMemoryNumPages(memory))*65536;}
    const std::vector<WAVM::U8>& stdoutBytes() const{return output->bytes;}
    const std::vector<WAVM::U8>& stderrBytes() const{return error->bytes;}
};
}
