// Public WAVM API only; compile-only helper, never instantiate or invoke guest code.
#include <WAVM/Runtime/Runtime.h>
#include <fstream>
#include <iterator>
#include <iostream>
int main(int argc,char** argv){
 if(argc!=3){std::cerr<<"usage: wavm-object INPUT.wasm OUTPUT.o\n";return 2;}
 std::ifstream in(argv[1],std::ios::binary);
 if(!in)return 3;
 std::vector<WAVM::U8> bytes((std::istreambuf_iterator<char>(in)),{});
 WAVM::Runtime::ModuleRef module;
 if(!WAVM::Runtime::loadBinaryModule(bytes.data(),bytes.size(),module))return 4;
 auto object=WAVM::Runtime::getObjectCode(module);
 std::ofstream out(argv[2],std::ios::binary);
 out.write(reinterpret_cast<const char*>(object.data()),object.size());
 std::cout<<object.size()<<" bytes in compiled object\n";
 return out?0:5;
}
