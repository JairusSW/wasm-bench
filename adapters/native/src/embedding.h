// Private adapter bridge, not a public runtime ABI. All work stays in this PID.
#pragma once
#include <cstddef>
#include <cstdint>
#include <string>
#include <vector>
#include <stdexcept>
#include <cstring>
inline thread_local std::string wb_last_error;
extern "C" {
const char* wb_error();
const char* wb_version();
void* wb_engine_new();
void wb_engine_delete(void*);
void* wb_module_new(void*, const uint8_t*, size_t);
void wb_module_delete(void*);
void* wb_instance_new(void*, void*);
void wb_instance_delete(void*);
int wb_signature(void*, const char*, uint8_t*, size_t*, uint8_t*, size_t*);
int wb_call(void*, const char*, const uint64_t*, size_t, uint64_t*, size_t);
uint8_t* wb_memory(void*, size_t*);
}
// Never let a C++ exception unwind through Rust.
#define WB_TRY try
#define WB_CATCH(value) catch (const std::exception& e) { wb_last_error=e.what(); return value; } catch (...) { wb_last_error="unknown native embedding exception"; return value; }
