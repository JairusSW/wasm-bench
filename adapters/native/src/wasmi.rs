use anyhow::{Result, ensure};
use wasmi::{CompilationMode, Config, Linker, Store, Val, ValType};
pub fn version() -> String {
    "2.0.0".into()
}
pub struct Engine(wasmi::Engine);
pub struct Module(wasmi::Module);
pub struct Instance {
    store: Store<()>,
    instance: wasmi::Instance,
}
impl Engine {
    pub fn new() -> Result<Self> {
        let mut config = Config::default();
        config.compilation_mode(CompilationMode::Eager);
        Ok(Self(wasmi::Engine::new(&config)))
    }
    pub fn compile(&self, bytes: &[u8]) -> Result<Module> {
        let module = wasmi::Module::new(&self.0, bytes)?;
        ensure!(
            module.imports().len() == 0,
            "unsupported: import-free modules only"
        );
        Ok(Module(module))
    }
    pub fn instantiate(&self, module: &Module) -> Result<Instance> {
        let mut store = Store::new(&self.0, ());
        let instance = Linker::new(&self.0).instantiate_and_start(&mut store, &module.0)?;
        Ok(Instance { store, instance })
    }
}
impl Instance {
    pub fn call(&mut self, name: &str, args: &[u64]) -> Result<Vec<u64>> {
        let f = self
            .instance
            .get_func(&self.store, name)
            .ok_or_else(|| anyhow::anyhow!("missing export {name}"))?;
        let ty = f.ty(&self.store);
        ensure!(args.len() == ty.params().len(), "argument count mismatch");
        let params: Result<Vec<_>> = args
            .iter()
            .zip(ty.params())
            .map(|(v, t)| match t {
                ValType::I32 => Ok(Val::I32(*v as i32)),
                ValType::I64 => Ok(Val::I64(*v as i64)),
                _ => anyhow::bail!("unsupported: integer signature required"),
            })
            .collect();
        let mut results: Vec<_> = ty
            .results()
            .iter()
            .map(|t| Val::default_for_ty(*t))
            .collect();
        ensure!(
            ty.results()
                .iter()
                .all(|t| matches!(t, ValType::I32 | ValType::I64)),
            "unsupported: integer results required"
        );
        f.call(&mut self.store, &params?, &mut results)?;
        Ok(results
            .iter()
            .map(|v| match v {
                Val::I32(x) => *x as u32 as u64,
                Val::I64(x) => *x as u64,
                _ => unreachable!(),
            })
            .collect())
    }
    pub fn memory(&mut self) -> Result<&mut [u8]> {
        let memory = self
            .instance
            .get_memory(&self.store, "memory")
            .ok_or_else(|| anyhow::anyhow!("missing exported memory"))?;
        Ok(memory.data_mut(&mut self.store))
    }
}
