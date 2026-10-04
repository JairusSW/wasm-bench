#pragma once
#include <WAVM/Platform/File.h>
#include <WAVM/VFS/VFS.h>
#include <filesystem>

namespace wb_wasi {
class ReadOnlyFD final: public WAVM::VFS::VFD {
    WAVM::VFS::VFD* inner;
    using Result=WAVM::VFS::Result;
public:
    explicit ReadOnlyFD(WAVM::VFS::VFD* value):inner(value){}
    Result close() override {auto result=inner->close();delete this;return result;}
    Result seek(WAVM::I64 n,WAVM::VFS::SeekOrigin origin,WAVM::U64* out) override{return inner->seek(n,origin,out);}
    Result readv(const WAVM::VFS::IOReadBuffer* b,WAVM::Uptr n,WAVM::Uptr* out,const WAVM::U64* offset) override{return inner->readv(b,n,out,offset);}
    Result writev(const WAVM::VFS::IOWriteBuffer*,WAVM::Uptr,WAVM::Uptr* out,const WAVM::U64*) override{if(out)*out=0;return Result::notPermitted;}
    Result sync(WAVM::VFS::SyncType t) override{return inner->sync(t);}
    Result getVFDInfo(WAVM::VFS::VFDInfo& info) override{return inner->getVFDInfo(info);}
    Result getFileInfo(WAVM::VFS::FileInfo& info) override{return inner->getFileInfo(info);}
    Result setVFDFlags(const WAVM::VFS::VFDFlags& flags) override{return flags.append||flags.syncLevel!=WAVM::VFS::VFDSync::none?Result::notPermitted:inner->setVFDFlags(flags);}
    Result setFileSize(WAVM::U64) override{return Result::notPermitted;}
    Result setFileTimes(bool,WAVM::Time,bool,WAVM::Time) override{return Result::notPermitted;}
    Result openDir(WAVM::VFS::DirEntStream*& out) override{return inner->openDir(out);}
};
// Root is a private fixture directory, containing only verified regular files.
// Reject parent traversal and symlinks rather than relying on WAVM SandboxFS,
// which simply concatenates the root and the guest-provided path.
class ReadOnlyFS final: public WAVM::VFS::FileSystem {
    std::filesystem::path root;
    using Result=WAVM::VFS::Result;
    bool resolve(const std::string& guest,std::string& out) const {
        if(guest.find('\\')!=std::string::npos || guest.find('\0')!=std::string::npos)return false;
        std::filesystem::path relative;
        for(const auto& part:std::filesystem::path(guest)){
            if(part=="..")return false;
            if(part=="/"||part=="."||part.empty())continue;
            relative/=part;
        }
        auto path=root;
        for(const auto& part:relative){
            path/=part;
            std::error_code error;
            auto status=std::filesystem::symlink_status(path,error);
            if(!error&&std::filesystem::is_symlink(status))return false;
            if(error&&error!=std::errc::no_such_file_or_directory)return false;
        }
        out=path.string();return true;
    }
public:
    explicit ReadOnlyFS(const std::string& directory):root(std::filesystem::canonical(directory)){}
    Result open(const std::string& name,WAVM::VFS::FileAccessMode access,WAVM::VFS::FileCreateMode create,WAVM::VFS::VFD*& out,const WAVM::VFS::VFDFlags& flags={}) override {
        out=nullptr;std::string path;
        if(!resolve(name,path)||create!=WAVM::VFS::FileCreateMode::openExisting||access==WAVM::VFS::FileAccessMode::writeOnly||flags.append||flags.syncLevel!=WAVM::VFS::VFDSync::none)return Result::notPermitted;
        WAVM::VFS::VFD* fd=nullptr;
        // Some WASI clients request broad rights for a read-only open. Every
        // returned descriptor enforces read-only access independently.
        auto result=WAVM::Platform::getHostFS().open(path,WAVM::VFS::FileAccessMode::readOnly,create,fd,flags);
        if(result!=Result::success)return result;
        try{out=new ReadOnlyFD(fd);}catch(...){fd->close();throw;}
        return Result::success;
    }
    Result getFileInfo(const std::string& name,WAVM::VFS::FileInfo& info) override{std::string path;return resolve(name,path)?WAVM::Platform::getHostFS().getFileInfo(path,info):Result::notPermitted;}
    Result openDir(const std::string& name,WAVM::VFS::DirEntStream*& out) override{std::string path;return resolve(name,path)?WAVM::Platform::getHostFS().openDir(path,out):Result::notPermitted;}
    Result setFileTimes(const std::string&,bool,WAVM::Time,bool,WAVM::Time) override{return Result::notPermitted;}
    Result renameFile(const std::string&,const std::string&) override{return Result::notPermitted;}
    Result unlinkFile(const std::string&) override{return Result::notPermitted;}
    Result removeDir(const std::string&) override{return Result::notPermitted;}
    Result createDir(const std::string&) override{return Result::notPermitted;}
};
}
