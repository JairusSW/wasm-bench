#include "../src/wavm_readonly.h"
#include <cassert>
#include <fstream>
#include <unistd.h>
int main(){
 using namespace WAVM;using namespace WAVM::VFS;using namespace wb_wasi;
 char pattern[]="/tmp/wasmbench-wavm-readonly-XXXXXX";
 const auto* directory=mkdtemp(pattern);assert(directory);
 std::ofstream(std::string(directory)+"/input")<<"abc";
 std::filesystem::create_symlink("/",std::string(directory)+"/escape");
 ReadOnlyFS fs(directory);VFD* fd=nullptr;
 assert(fs.open("input",FileAccessMode::readWrite,FileCreateMode::openExisting,fd)==Result::success);
 char bytes[3];IOReadBuffer reads[]={{bytes,3}};Uptr n=0;
 assert(fd->readv(reads,1,&n,nullptr)==Result::success&&n==3&&bytes[2]=='c');
 IOWriteBuffer writes[]={{"x",1}};
 assert(fd->writev(writes,1,&n,nullptr)==Result::notPermitted&&n==0);
 assert(fd->setFileSize(0)==Result::notPermitted);fd->close();
 assert(fs.open("../input",FileAccessMode::readOnly,FileCreateMode::openExisting,fd)==Result::notPermitted);
 assert(fs.open("escape/etc/passwd",FileAccessMode::readOnly,FileCreateMode::openExisting,fd)==Result::notPermitted);
 assert(fs.open("input",FileAccessMode::readOnly,FileCreateMode::truncateExisting,fd)==Result::notPermitted);
 assert(fs.unlinkFile("input")==Result::notPermitted);
 std::filesystem::remove_all(directory);
}
