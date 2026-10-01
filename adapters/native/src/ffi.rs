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
