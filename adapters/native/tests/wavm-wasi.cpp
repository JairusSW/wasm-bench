#include "../src/wavm_wasi.h"
#include <cassert>
#include <unistd.h>
int main(){
 // Export memory and _start; _start calls wasi_snapshot_preview1.proc_exit(7).
 const unsigned char wasm[]={0,97,115,109,1,0,0,0,
 1,8,2,96,1,127,0,96,0,0,
 2,36,1,22,'w','a','s','i','_','s','n','a','p','s','h','o','t','_','p','r','e','v','i','e','w','1',9,'p','r','o','c','_','e','x','i','t',0,0,
 3,2,1,1,5,3,1,0,1,
 7,19,2,6,'m','e','m','o','r','y',2,0,6,'_','s','t','a','r','t',0,1,
 10,8,1,6,0,65,7,16,0,11};
 WAVM::Runtime::ModuleRef module;
 assert(WAVM::Runtime::loadBinaryModule(wasm,sizeof(wasm),module));
 char pattern[]="/tmp/wasmbench-wavm-wasi-XXXXXX";const auto* root=mkdtemp(pattern);assert(root);
 {wb_wasi::CommandInstance command(module,{"test"},root,{},1024);assert(command.run()==7);assert(command.stdoutBytes().empty());}
 std::filesystem::remove_all(root);
}
