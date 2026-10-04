#pragma once
#include <WAVM/VFS/VFS.h>
#include <algorithm>
#include <cstring>
#include <memory>
#include <vector>

namespace wb_wasi {
struct StreamBuffer {
    std::vector<WAVM::U8> bytes;
    size_t cursor=0, limit=0;
    bool overflow=false;
};
// WASI owns the VFD and may close it before exit. The instance retains the
// shared buffer, so output validation remains possible after fd_close.
class MemoryFD final: public WAVM::VFS::VFD {
    std::shared_ptr<StreamBuffer> buffer;
    bool writable;
    WAVM::VFS::VFDFlags streamFlags{};
    using Result=WAVM::VFS::Result;
public:
    MemoryFD(std::shared_ptr<StreamBuffer> value,bool write):buffer(std::move(value)),writable(write){}
    Result close() override {delete this;return Result::success;}
    Result seek(WAVM::I64,WAVM::VFS::SeekOrigin,WAVM::U64*) override{return Result::notSeekable;}
    Result readv(const WAVM::VFS::IOReadBuffer* inputs,WAVM::Uptr count,WAVM::Uptr* read,const WAVM::U64* offset) override {
        if(read)*read=0;
        if(writable)return Result::notPermitted;
        if(offset)return Result::notSeekable;
        size_t total=0;
        for(size_t i=0;i<count;i++){
            size_t n=std::min<size_t>(inputs[i].numBytes,buffer->bytes.size()-buffer->cursor);
            if(n)std::memcpy(inputs[i].data,buffer->bytes.data()+buffer->cursor,n);
            buffer->cursor+=n;total+=n;
        }
        if(read)*read=total;
        return Result::success;
    }
    Result writev(const WAVM::VFS::IOWriteBuffer* outputs,WAVM::Uptr count,WAVM::Uptr* written,const WAVM::U64* offset) override {
        if(written)*written=0;
        if(!writable)return Result::notPermitted;
        if(offset)return Result::notSeekable;
        size_t total=0;
        for(size_t i=0;i<count;i++){
            if(buffer->bytes.size()>buffer->limit || total>buffer->limit-buffer->bytes.size() || outputs[i].numBytes>buffer->limit-buffer->bytes.size()-total){
                buffer->overflow=true;return Result::exceededFileSizeLimit;
            }
            total+=outputs[i].numBytes;
        }
        try {
            buffer->bytes.reserve(buffer->bytes.size()+total);
            for(size_t i=0;i<count;i++)if(outputs[i].numBytes){
                const auto* data=static_cast<const WAVM::U8*>(outputs[i].data);
                buffer->bytes.insert(buffer->bytes.end(),data,data+outputs[i].numBytes);
            }
        }catch(const std::bad_alloc&){buffer->overflow=true;return Result::outOfMemory;}
        if(written)*written=total;
        return Result::success;
    }
    Result sync(WAVM::VFS::SyncType) override{return Result::success;}
    Result getVFDInfo(WAVM::VFS::VFDInfo& info) override{info={};info.type=WAVM::VFS::FileType::pipe;info.flags=streamFlags;return Result::success;}
    Result getFileInfo(WAVM::VFS::FileInfo& info) override{info={};info.type=WAVM::VFS::FileType::pipe;info.numBytes=buffer->bytes.size();info.numLinks=1;return Result::success;}
    Result setVFDFlags(const WAVM::VFS::VFDFlags& flags) override{if(flags.append||flags.syncLevel!=WAVM::VFS::VFDSync::none)return Result::notSupported;streamFlags=flags;return Result::success;}
    Result setFileSize(WAVM::U64) override{return Result::notPermitted;}
    Result setFileTimes(bool,WAVM::Time,bool,WAVM::Time) override{return Result::notPermitted;}
    Result openDir(WAVM::VFS::DirEntStream*&) override{return Result::isNotDirectory;}
};
}
