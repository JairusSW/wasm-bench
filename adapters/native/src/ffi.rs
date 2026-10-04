use anyhow::{Result, anyhow, ensure};
use std::{
    ffi::{CStr, CString, c_char, c_void},
    ptr::NonNull,
};
unsafe extern "C" {
    fn wb_error() -> *const c_char;
    fn wb_version() -> *const c_char;
    fn wb_engine_new() -> *mut c_void;
    fn wb_engine_delete(p: *mut c_void);
    fn wb_module_new(e: *mut c_void, b: *const u8, n: usize) -> *mut c_void;
    fn wb_module_delete(p: *mut c_void);
    fn wb_instance_new(e: *mut c_void, m: *mut c_void) -> *mut c_void;
    fn wb_instance_delete(p: *mut c_void);
    fn wb_signature(
        i: *mut c_void,
        name: *const c_char,
        p: *mut u8,
        np: *mut usize,
        r: *mut u8,
        nr: *mut usize,
    ) -> i32;
    fn wb_call(
        i: *mut c_void,
        name: *const c_char,
        a: *const u64,
        na: usize,
        r: *mut u64,
        nr: usize,
    ) -> i32;
    fn wb_memory(i: *mut c_void, n: *mut usize) -> *mut u8;
}
fn error() -> anyhow::Error {
    unsafe { anyhow!("{}", CStr::from_ptr(wb_error()).to_string_lossy()) }
}
pub fn version() -> String {
    unsafe { CStr::from_ptr(wb_version()).to_string_lossy().into_owned() }
}
pub struct Engine(NonNull<c_void>);
pub struct Module(NonNull<c_void>);
pub struct Instance(NonNull<c_void>);
impl Drop for Engine {
    fn drop(&mut self) {
        unsafe { wb_engine_delete(self.0.as_ptr()) }
    }
}
impl Drop for Module {
    fn drop(&mut self) {
        unsafe { wb_module_delete(self.0.as_ptr()) }
    }
}
impl Drop for Instance {
    fn drop(&mut self) {
        unsafe { wb_instance_delete(self.0.as_ptr()) }
    }
}
impl Engine {
    pub fn new() -> Result<Self> {
        unsafe { NonNull::new(wb_engine_new()).map(Self).ok_or_else(error) }
    }
    pub fn compile(&self, bytes: &[u8]) -> Result<Module> {
        unsafe {
            NonNull::new(wb_module_new(self.0.as_ptr(), bytes.as_ptr(), bytes.len()))
                .map(Module)
                .ok_or_else(error)
        }
    }
    pub fn instantiate(&self, module: &Module) -> Result<Instance> {
        unsafe {
            NonNull::new(wb_instance_new(self.0.as_ptr(), module.0.as_ptr()))
                .map(Instance)
                .ok_or_else(error)
        }
    }
}
impl Instance {
    pub fn signature(&self, name: &str) -> Result<(Vec<u8>, Vec<u8>)> {
        let name = CString::new(name)?;
        let mut params = [0u8; 32];
        let mut results = [0u8; 32];
        let mut np = 32usize;
        let mut nr = 32usize;
        unsafe {
            ensure!(
                wb_signature(
                    self.0.as_ptr(),
                    name.as_ptr(),
                    params.as_mut_ptr(),
                    &mut np,
                    results.as_mut_ptr(),
                    &mut nr
                ) == 0,
                "{}",
                error()
            );
        }
        ensure!(np <= 32 && nr <= 32, "invalid signature bounds");
        Ok((params[..np].to_vec(), results[..nr].to_vec()))
    }
    pub fn call(&mut self, name: &str, args: &[u64]) -> Result<Vec<u64>> {
        let name = CString::new(name)?;
        let mut params = [0u8; 32];
        let mut results = [0u8; 32];
        let mut np = 32usize;
        let mut nr = 32usize;
        unsafe {
            ensure!(
                wb_signature(
                    self.0.as_ptr(),
                    name.as_ptr(),
                    params.as_mut_ptr(),
                    &mut np,
                    results.as_mut_ptr(),
                    &mut nr
                ) == 0,
                "{}",
                error()
            );
        }
        ensure!(np == args.len(), "argument count mismatch");
        ensure!(
            params[..np]
                .iter()
                .chain(results[..nr].iter())
                .all(|t| *t == 0x7f || *t == 0x7e),
            "unsupported: i32/i64 signatures only"
        );
        let mut out = vec![0; nr];
        unsafe {
            ensure!(
                wb_call(
                    self.0.as_ptr(),
                    name.as_ptr(),
                    args.as_ptr(),
                    args.len(),
                    out.as_mut_ptr(),
                    out.len()
                ) == 0,
                "{}",
                error()
            );
        }
        Ok(out)
    }
    pub fn memory(&mut self) -> Result<&mut [u8]> {
        let mut n = 0;
        let p = unsafe { wb_memory(self.0.as_ptr(), &mut n) };
        ensure!(!p.is_null(), "missing exported memory");
        Ok(unsafe { std::slice::from_raw_parts_mut(p, n) })
    }
}

#[cfg(feature = "wavm")]
pub fn object_code(bytes: &[u8]) -> Result<Vec<u8>> {
    unsafe extern "C" {
        fn wb_object_code(
            bytes: *const u8,
            size: usize,
            output: *mut *mut u8,
            output_size: *mut usize,
        ) -> i32;
        fn wb_object_delete(bytes: *mut u8);
    }
    let mut output = std::ptr::null_mut();
    let mut size = 0;
    unsafe {
        if wb_object_code(bytes.as_ptr(), bytes.len(), &mut output, &mut size) != 0 {
            return Err(error());
        }
        let object = std::slice::from_raw_parts(output, size).to_vec();
        wb_object_delete(output);
        Ok(object)
    }
}

#[cfg(feature = "wavm")]
mod wasi {
    use super::*;
    unsafe extern "C" {
        fn wb_wasi_module_new(bytes: *const u8, size: usize) -> *mut c_void;
        fn wb_wasi_module_delete(module: *mut c_void);
        fn wb_wasi_instance_new(
            module: *mut c_void,
            argv: *const *const c_char,
            argc: usize,
            root: *const c_char,
            input: *const u8,
            input_size: usize,
            limit: usize,
        ) -> *mut c_void;
        fn wb_wasi_instance_delete(instance: *mut c_void);
        fn wb_wasi_run(instance: *mut c_void, exit: *mut u32) -> i32;
        fn wb_wasi_memory_bytes(instance: *mut c_void) -> u64;
        fn wb_wasi_output(instance: *mut c_void, error: bool, size: *mut usize) -> *const u8;
    }
    pub struct WasiModule(NonNull<c_void>);
    pub struct WasiInstance(NonNull<c_void>);
    impl Drop for WasiModule {
        fn drop(&mut self) {
            unsafe { wb_wasi_module_delete(self.0.as_ptr()) }
        }
    }
    impl Drop for WasiInstance {
        fn drop(&mut self) {
            unsafe { wb_wasi_instance_delete(self.0.as_ptr()) }
        }
    }
    impl WasiModule {
        pub fn compile(bytes: &[u8]) -> Result<Self> {
            unsafe {
                NonNull::new(wb_wasi_module_new(bytes.as_ptr(), bytes.len()))
                    .map(Self)
                    .ok_or_else(error)
            }
        }
        pub fn instantiate(
            &self,
            argv: &[String],
            root: &str,
            input: &[u8],
            limit: usize,
        ) -> Result<WasiInstance> {
            let args: Vec<CString> = argv
                .iter()
                .map(|s| CString::new(s.as_str()))
                .collect::<std::result::Result<_, _>>()?;
            let pointers: Vec<_> = args.iter().map(|s| s.as_ptr()).collect();
            let root = CString::new(root)?;
            unsafe {
                NonNull::new(wb_wasi_instance_new(
                    self.0.as_ptr(),
                    pointers.as_ptr(),
                    pointers.len(),
                    root.as_ptr(),
                    input.as_ptr(),
                    input.len(),
                    limit,
                ))
                .map(WasiInstance)
                .ok_or_else(error)
            }
        }
    }
    impl WasiInstance {
        pub fn memory_bytes(&self) -> u64 {
            unsafe { wb_wasi_memory_bytes(self.0.as_ptr()) }
        }
        pub fn run(&self) -> Result<u32> {
            let mut exit = 0;
            unsafe {
                if wb_wasi_run(self.0.as_ptr(), &mut exit) != 0 {
                    return Err(error());
                }
            }
            Ok(exit)
        }
        pub fn output(&self, stderr: bool) -> Vec<u8> {
            let mut size = 0;
            unsafe {
                let ptr = wb_wasi_output(self.0.as_ptr(), stderr, &mut size);
                if size == 0 {
                    vec![]
                } else {
                    std::slice::from_raw_parts(ptr, size).to_vec()
                }
            }
        }
    }
}
#[cfg(feature = "wavm")]
pub use wasi::WasiModule;
