#include "../src/wavm_stdio.h"
#include <cassert>
int main(){
 using namespace WAVM;using namespace WAVM::VFS;using namespace wb_wasi;
 auto input=std::make_shared<StreamBuffer>();input->bytes={1,2,3};
 auto* reader=new MemoryFD(input,false);U8 bytes[4]={};Uptr n=99;
 IOReadBuffer reads[]={{bytes,2},{bytes+2,2}};
 assert(reader->readv(reads,2,&n,nullptr)==Result::success&&n==3&&bytes[2]==3);
 assert(reader->readv(reads,2,&n,nullptr)==Result::success&&n==0);
 assert(reader->close()==Result::success);
 auto output=std::make_shared<StreamBuffer>();output->limit=3;
 auto* writer=new MemoryFD(output,true);U8 data[]={4,5,6};
 IOWriteBuffer writes[]={{data,1},{data+1,2}};
 assert(writer->writev(writes,2,&n,nullptr)==Result::success&&n==3);
 assert(writer->writev(writes,1,&n,nullptr)==Result::exceededFileSizeLimit&&n==0);
 assert(output->overflow&&output->bytes.size()==3);
 assert(writer->setFileSize(0)==Result::notPermitted);
 assert(writer->close()==Result::success&&output->bytes[2]==6);
}
