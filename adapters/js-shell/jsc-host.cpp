// Raw JavaScriptCore embedding for macOS and Linux. Supplies a monotonic clock
// and keeps WebAssembly/JSON work in this PID for both native hosts.
#include <JavaScriptCore/JavaScript.h>
#include <chrono>
#include <fstream>
#include <iostream>
#include <sstream>
#include <string>
#include <vector>
#include <cstdlib>
#include <cstring>
#include <unistd.h>
static JSValueRef string(JSContextRef c,const std::string& s){auto text=JSStringCreateWithUTF8CString(s.c_str());auto v=JSValueMakeString(c,text);JSStringRelease(text);return v;}
static std::string text(JSContextRef c,JSValueRef v,JSValueRef* error){auto s=JSValueToStringCopy(c,v,error);if(!s)return {};std::string out(JSStringGetMaximumUTF8CStringSize(s),'\0');out.resize(JSStringGetUTF8CString(s,out.data(),out.size())-1);JSStringRelease(s);return out;}
static JSValueRef print(JSContextRef c,JSObjectRef,JSObjectRef,size_t n,const JSValueRef v[],JSValueRef* e){if(n)std::cout<<text(c,v[0],e)<<'\n'<<std::flush;return JSValueMakeUndefined(c);}
static JSValueRef readline(JSContextRef c,JSObjectRef,JSObjectRef,size_t,const JSValueRef[],JSValueRef*){std::string line;if(!std::getline(std::cin,line))return JSValueMakeNull(c);return string(c,line);}
static JSValueRef now(JSContextRef c,JSObjectRef,JSObjectRef,size_t,const JSValueRef[],JSValueRef*){static const auto epoch=std::chrono::steady_clock::now();return JSValueMakeNumber(c,std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-epoch).count());}
static void release(void* bytes,void*){std::free(bytes);}
static JSValueRef readBytes(JSContextRef c,JSObjectRef,JSObjectRef,size_t n,const JSValueRef v[],JSValueRef* e){
    if(!n){*e=string(c,"missing path");return JSValueMakeUndefined(c);}std::ifstream file(text(c,v[0],e),std::ios::binary);
    if(!file){*e=string(c,"cannot read artifact");return JSValueMakeUndefined(c);}std::ostringstream out;out<<file.rdbuf();const auto data=out.str();
    auto bytes=std::malloc(data.empty()?1:data.size());if(!bytes){*e=string(c,"allocation failure");return JSValueMakeUndefined(c);}std::memcpy(bytes,data.data(),data.size());
    return JSObjectMakeTypedArrayWithBytesNoCopy(c,kJSTypedArrayTypeUint8Array,bytes,data.size(),release,nullptr,e);
}
int main(int argc,char** argv){
    if(argc<2){std::cerr<<"usage: adapter-jsc SCRIPT runtime=jsc binary-sha256=...\n";return 2;}
    for(int i=2;i<argc;i++)if(std::string(argv[i])=="tier-mode=omg-eager"){
        setenv("JSC_validateOptions","true",1);
        setenv("JSC_thresholdForBBQOptimizeAfterWarmUp","1",1);
        setenv("JSC_thresholdForBBQOptimizeSoon","1",1);
        setenv("JSC_thresholdForOMGOptimizeAfterWarmUp","1",1);
        setenv("JSC_thresholdForOMGOptimizeSoon","1",1);
        setenv("JSC_useConcurrentJIT","false",1);
        setenv("JSC_numberOfWasmCompilerThreads","0",1);
        setenv("JSC_dumpOMGDisassembly","true",1);
        // JavaScriptCore reads option environment variables during process
        // initialization, before main(). Re-exec once so the requested tier
        // policy is active before the framework is loaded.
        if(std::getenv("WASMBENCH_JSC_OPTIONS_READY")==nullptr){
            setenv("WASMBENCH_JSC_OPTIONS_READY","1",1);
            execvp(argv[0],argv);
            std::perror("re-exec JSC with benchmark options");
            return 127;
        }
    }
    std::ifstream file(argv[1]);if(!file){std::cerr<<"cannot read script\n";return 2;}std::ostringstream source;source<<file.rdbuf();
    auto context=JSGlobalContextCreate(nullptr);auto global=JSContextGetGlobalObject(context);JSValueRef error=nullptr;
    const std::pair<const char*,JSObjectCallAsFunctionCallback> functions[]={{"print",print},{"readline",readline},{"read",readBytes},{"benchNow",now}};
    for(auto pair:functions){auto name=JSStringCreateWithUTF8CString(pair.first);auto f=JSObjectMakeFunctionWithCallback(context,name,pair.second);JSObjectSetProperty(context,global,name,f,kJSPropertyAttributeReadOnly,&error);JSStringRelease(name);}
    std::vector<JSValueRef> args;for(int i=2;i<argc;i++)if(std::string(argv[i])!="--")args.push_back(string(context,argv[i]));
    auto name=JSStringCreateWithUTF8CString("arguments");JSObjectSetProperty(context,global,name,JSObjectMakeArray(context,args.size(),args.data(),&error),kJSPropertyAttributeReadOnly,&error);JSStringRelease(name);
    auto script=JSStringCreateWithUTF8CString(source.str().c_str());JSEvaluateScript(context,script,nullptr,nullptr,1,&error);JSStringRelease(script);
    if(error)std::cerr<<text(context,error,nullptr)<<'\n';JSGlobalContextRelease(context);return error?1:0;
}
