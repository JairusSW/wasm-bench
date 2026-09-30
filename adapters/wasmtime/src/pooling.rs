use wasmtime::{Config, InstanceAllocationStrategy, PoolingAllocationConfig};

pub fn enabled() -> bool {
    std::env::args().any(|s| s == "--pooling")
}

// Fixed resource policy, not a hidden per-workload auto-tuning heuristic.
pub fn configure(config: &mut Config) {
    let mut pool = PoolingAllocationConfig::default();
    pool.total_core_instances(128)
        .total_memories(128)
        .total_tables(16)
        .table_elements(10000)
        .max_memory_size(16 * 1024 * 1024)
        .max_unused_warm_slots(128)
        .linear_memory_keep_resident(0)
        .decommit_batch_size(1);
    config
        .memory_reservation(16 * 1024 * 1024)
        .memory_guard_size(65536)
        .allocation_strategy(InstanceAllocationStrategy::Pooling(pool));
}

pub const POLICY: &str = "pooling-v1; core_instances=128; memories=128; tables=16; table_elements=10000; max_memory_bytes=16777216; memory_reservation=16777216; memory_guard=65536; max_unused_warm_slots=128; linear_memory_keep_resident=0; decommit_batch_size=1; memory-protection-keys feature disabled; other Wasmtime 46.0.1 defaults; fresh logical instances, not reused guest state";

#[cfg(test)]
mod tests {
    use super::*;
    use crate::Adapter;
    use wasmtime::{Instance, Module, Store};

    #[test]
    fn capacity_and_recycled_guest_state() -> wasmtime::Result<()> {
        let a = Adapter {
            prep: None,
            bytes: vec![],
            winch: false,
        };
        let engine = a.engine_with_allocator(true)?;
        let module = Module::new(
            &engine,
            include_bytes!("../../../corpus/testdata/density.wasm"),
        )?;
        let mut stores = Vec::new();
        for _ in 0..128 {
            let mut store = Store::new(&engine, ());
            let instance = Instance::new(&mut store, &module, &[])?;
            assert_eq!(
                instance
                    .get_typed_func::<i32, i32>(&mut store, "benchmark")?
                    .call(&mut store, 65536)?,
                65537
            );
            stores.push(store);
        }
        let mut extra = Store::new(&engine, ());
        assert!(
            Instance::new(&mut extra, &module, &[]).is_err(),
            "pool capacity must be enforced"
        );
        drop(extra);
        drop(stores);
        let mut recycled = Vec::new();
        for _ in 0..128 {
            let mut store = Store::new(&engine, ());
            let instance = Instance::new(&mut store, &module, &[])?;
            let memory = instance.get_memory(&mut store, "memory").unwrap();
            assert!(
                memory.data(&store).iter().all(|b| *b == 0),
                "dirty memory survived recycling"
            );
            assert_eq!(
                instance
                    .get_typed_func::<i32, i32>(&mut store, "benchmark")?
                    .call(&mut store, 0)?,
                1,
                "global state survived recycling"
            );
            recycled.push(store);
        }
        assert_eq!(recycled.len(), 128);
        assert!(POLICY.contains("max_memory_bytes=16777216"));
        Ok(())
    }
}
