//! Linux process-level COW qualification and development timing worker, not a
//! registered product adapter. Never fork a multithreaded embedding.
#[cfg(not(target_os = "linux"))]
fn main() {
    eprintln!("process snapshot qualification requires Linux");
    std::process::exit(2);
}

#[cfg(target_os = "linux")]
mod linux {
    use anyhow::{Result, bail, ensure};
    use sha2::{Digest, Sha256};
    use std::io::{Read, Write};
    use std::net::Shutdown;
    use std::os::unix::net::UnixStream;
    use std::time::{Duration, Instant};
    use wasmtime::{Config, Engine, Instance, Module, Store, Strategy};

    mod timing {
        include!("../process_snapshot_timing.rs");
    }

    mod adapter {
        include!("../process_snapshot_adapter.rs");
    }

    mod density {
        include!("../process_snapshot_density.rs");
    }

    pub fn density_run(backend: &str, instances: usize) -> Result<()> {
        density::run(backend, instances)
    }

    pub fn density_failure_run(backend: &str, instances: usize, fail_after: usize) -> Result<()> {
        density::run_failure(backend, instances, fail_after)
    }

    pub fn serve(backend: &str) -> Result<()> {
        adapter::serve(backend)
    }

    pub fn timed_run(backend: &str, stage: &str, samples: usize) -> Result<()> {
        timing::run(backend, stage, samples)
    }

    unsafe extern "C" {
        fn fork() -> i32;
        fn waitpid(pid: i32, status: *mut i32, options: i32) -> i32;
        fn kill(pid: i32, signal: i32) -> i32;
        fn getpid() -> i32;
        fn getppid() -> i32;
        fn prctl(option: i32, ...) -> i32;
        fn _exit(status: i32) -> !;
    }

    // Hidden global/table, grown sizes and live passive segments distinguish
    // this from exported-memory copying or fresh re-instantiation.
    const FIXTURE: &str = r#"(module
      (memory (export "memory") 2 4)
      (global $g (mut i32) (i32.const 42))
      (type $result (func (result i32)))
      (func $a (type $result) (result i32) i32.const 7)
      (func $b (type $result) (result i32) i32.const 9)
      (table $t 2 4 funcref)
      (elem (i32.const 0) func $a $b)
      (elem $e func $a)
      (data $d "xyz")
      (func (export "seed")
        i32.const 0 i32.const 7 i32.store8
        i32.const 0 ref.func $b table.set $t
        i32.const 1 memory.grow drop
        ref.null func i32.const 1 table.grow $t drop)
      (func (export "mutate")
        i32.const 0 i32.const 11 i32.store8
        i32.const 99 global.set $g
        i32.const 0 ref.func $a table.set $t
        i32.const 1 memory.grow drop
        ref.null func i32.const 1 table.grow $t drop
        data.drop $d elem.drop $e)
      (func (export "check") (result i32)
        i32.const 0 i32.load8_u global.get $g i32.add
        i32.const 0 call_indirect $t (type $result) i32.add
        memory.size i32.add table.size $t i32.add)
      (func (export "probe") (result i32)
        i32.const 128 i32.const 0 i32.const 3 memory.init $d
        i32.const 2 i32.const 0 i32.const 1 table.init $t $e
        i32.const 128 i32.load8_u
        i32.const 2 call_indirect $t (type $result) i32.add)
      (func (export "pages") (result i32) memory.size)
      (func (export "elements") (result i32) table.size $t))"#;

    struct State {
        store: Store<()>,
        instance: Instance,
    }
    fn runtime_error(error: wasmtime::Error) -> anyhow::Error {
        anyhow::anyhow!("{error:#}")
    }
    impl State {
        fn call(&mut self, name: &str) -> Result<i32> {
            Ok(self
                .instance
                .get_typed_func::<(), i32>(&mut self.store, name)
                .map_err(runtime_error)?
                .call(&mut self.store, ())
                .map_err(runtime_error)?)
        }
        fn action(&mut self, name: &str) -> Result<()> {
            self.instance
                .get_typed_func::<(), ()>(&mut self.store, name)
                .map_err(runtime_error)?
                .call(&mut self.store, ())
                .map_err(runtime_error)?;
            Ok(())
        }
    }

    fn single_threaded() -> Result<()> {
        let count = std::fs::read_dir("/proc/self/task")?
            .collect::<std::io::Result<Vec<_>>>()?
            .len();
        ensure!(count == 1, "refuse fork: embedding has {count} threads");
        Ok(())
    }

    // Only direct unreaped children may be held here. No unrelated process or
    // process group is ever a cleanup target. EINTR does not abandon ownership.
    struct OwnedChild {
        pid: i32,
    }
    impl OwnedChild {
        fn poll(&mut self) -> Result<Option<i32>> {
            ensure!(self.pid > 0, "child already reaped");
            let mut status = 0;
            let result = unsafe { waitpid(self.pid, &mut status, 1) };
            if result == self.pid {
                self.pid = 0;
                return Ok(Some(status));
            }
            if result < 0 {
                let error = std::io::Error::last_os_error();
                if error.kind() == std::io::ErrorKind::Interrupted {
                    return Ok(None);
                }
                // ECHILD means ownership is gone; never signal a reused PID.
                if error.raw_os_error() == Some(10) {
                    self.pid = 0;
                }
                return Err(error.into());
            }
            Ok(None)
        }
        fn wait(&mut self, timeout: Duration) -> Result<i32> {
            let deadline = Instant::now() + timeout;
            loop {
                if let Some(status) = self.poll()? {
                    return Ok(status);
                }
                ensure!(Instant::now() < deadline, "child exit timed out");
                std::thread::sleep(Duration::from_millis(5));
            }
        }
        fn terminate(&mut self) -> Result<()> {
            if self.pid <= 0 || self.poll()?.is_some() {
                return Ok(());
            }
            // SAFETY: positive PID still names our unreaped direct child.
            unsafe {
                kill(self.pid, 9);
            }
            self.wait(Duration::from_secs(2))?;
            Ok(())
        }
    }
    impl Drop for OwnedChild {
        fn drop(&mut self) {
            let _ = self.terminate();
        }
    }

    fn bind_parent_lifetime(parent_pid: i32) -> Result<()> {
        // Linux clears this setting at fork, so every new child must arm it.
        // Check parent identity after arming to close the parent-exit race.
        ensure!(
            unsafe { prctl(1, 9_i64, 0_i64, 0_i64, 0_i64) } == 0,
            "cannot arm parent-death cleanup: {}",
            std::io::Error::last_os_error()
        );
        ensure!(
            unsafe { getppid() } == parent_pid,
            "parent exited before child setup"
        );
        Ok(())
    }

    struct Template {
        child: OwnedChild,
        control: UnixStream,
    }
    impl Template {
        fn finish(mut self) -> Result<()> {
            self.control.write_all(b"q")?;
            let status = self.child.wait(Duration::from_secs(2))?;
            ensure!(status == 0, "template failed: {status}");
            Ok(())
        }
    }
    impl Drop for Template {
        fn drop(&mut self) {
            let _ = self.control.write_all(b"q");
            let _ = self.control.shutdown(Shutdown::Both);
            if self.child.pid > 0 {
                let _ = self.child.wait(Duration::from_secs(2));
            }
            // OwnedChild drops next, terminating only if still unreaped.
        }
    }

    fn receive_proof(
        input: &mut UnixStream,
        child: &mut OwnedChild,
        timeout: Duration,
    ) -> Result<[u8; 40]> {
        input.set_read_timeout(Some(timeout))?;
        let mut proof = [0; 40];
        let result = (|| -> Result<()> {
            input.read_exact(&mut proof)?;
            let status = child.wait(timeout)?;
            ensure!(status == 0, "restored child failed: {status}");
            let values: Vec<i64> = proof
                .chunks_exact(8)
                .map(|b| i64::from_le_bytes(b.try_into().unwrap()))
                .collect();
            ensure!(values == [64, 3, 3, 127, 125], "invalid restored state");
            Ok(())
        })();
        if result.is_err() {
            child.terminate()?;
        }
        result?;
        Ok(proof)
    }

    fn parent_death_probe() -> Result<()> {
        single_threaded()?;
        // Test-process-only subreaper lets us reap the grandchild ourselves
        // after its direct parent exits; no external PID is observed/signaled.
        ensure!(
            unsafe { prctl(36, 1_i64, 0_i64, 0_i64, 0_i64) } == 0,
            "cannot enable cleanup-test subreaper"
        );
        let (mut input, mut control) = UnixStream::pair()?;
        input.set_read_timeout(Some(Duration::from_secs(2)))?;
        let root_pid = unsafe { getpid() };
        let pid = unsafe { fork() };
        ensure!(pid >= 0, "parent-death test fork failed");
        if pid == 0 {
            drop(input);
            let result = (|| -> Result<()> {
                bind_parent_lifetime(root_pid)?;
                let (mut ready, mut signal) = UnixStream::pair()?;
                ready.set_read_timeout(Some(Duration::from_secs(2)))?;
                let parent_pid = unsafe { getpid() };
                single_threaded()?;
                let grandchild = unsafe { fork() };
                ensure!(grandchild >= 0, "parent-death grandchild fork failed");
                if grandchild == 0 {
                    drop(control);
                    drop(ready);
                    if bind_parent_lifetime(parent_pid).is_err() || signal.write_all(b"s").is_err()
                    {
                        unsafe { _exit(102) }
                    }
                    loop {
                        std::thread::sleep(Duration::from_secs(1));
                    }
                }
                drop(signal);
                let mut ready_byte = [0];
                ready.read_exact(&mut ready_byte)?;
                ensure!(ready_byte == *b"s");
                control.write_all(&grandchild.to_le_bytes())?;
                let mut command = [0];
                control.read_exact(&mut command)?;
                ensure!(command == *b"q");
                Ok(())
            })();
            unsafe { _exit(if result.is_ok() { 0 } else { 104 }) }
        }
        drop(control);
        let mut parent = OwnedChild { pid };
        let mut encoded_pid = [0; 4];
        input.read_exact(&mut encoded_pid)?;
        let grandchild_pid = i32::from_le_bytes(encoded_pid);
        ensure!(grandchild_pid > 0 && grandchild_pid != pid && grandchild_pid != root_pid);
        input.write_all(b"q")?;
        ensure!(parent.wait(Duration::from_secs(2))? == 0);
        // The exited direct child transfers its child to this test subreaper.
        let mut adopted = OwnedChild {
            pid: grandchild_pid,
        };
        ensure!(
            adopted.wait(Duration::from_secs(2))? == 9,
            "parent death did not terminate child with SIGKILL"
        );
        ensure!(unsafe { prctl(36, 0_i64, 0_i64, 0_i64, 0_i64) } == 0);
        Ok(())
    }

    pub fn failure_probe() -> Result<()> {
        single_threaded()?;
        let mut cases = Vec::new();
        for mode in [
            "valid",
            "exit_without_proof",
            "partial_proof",
            "wrong_proof",
            "proof_then_failure",
            "proof_then_stall",
            "stall_without_proof",
        ] {
            let (mut input, mut output) = UnixStream::pair()?;
            let parent_pid = unsafe { getpid() };
            single_threaded()?;
            let pid = unsafe { fork() };
            ensure!(pid >= 0, "failure-probe fork failed");
            if pid == 0 {
                drop(input);
                if bind_parent_lifetime(parent_pid).is_err() {
                    unsafe { _exit(102) }
                }
                if mode == "exit_without_proof" {
                    unsafe { _exit(100) }
                }
                if mode == "partial_proof" {
                    let _ = output.write_all(&[0; 8]);
                    unsafe { _exit(0) }
                }
                if mode != "stall_without_proof" {
                    for value in [
                        if mode == "wrong_proof" { 65_i64 } else { 64 },
                        3,
                        3,
                        127,
                        125,
                    ] {
                        if output.write_all(&value.to_le_bytes()).is_err() {
                            unsafe { _exit(103) }
                        }
                    }
                }
                if mode == "proof_then_failure" {
                    unsafe { _exit(100) }
                }
                if mode == "proof_then_stall" || mode == "stall_without_proof" {
                    loop {
                        std::thread::sleep(Duration::from_secs(1));
                    }
                }
                unsafe { _exit(0) }
            }
            drop(output);
            let mut child = OwnedChild { pid };
            let result = receive_proof(&mut input, &mut child, Duration::from_millis(150));
            ensure!(
                result.is_ok() == (mode == "valid"),
                "wrong failure-probe outcome for {mode}"
            );
            ensure!(child.pid == 0, "failure-probe child not reaped: {mode}");
            let mut status = 0;
            ensure!(
                unsafe { waitpid(pid, &mut status, 1) } == -1
                    && std::io::Error::last_os_error().raw_os_error() == Some(10),
                "child remains waitable: {mode}"
            );
            cases.push(serde_json::json!({"mode":mode,"accepted":result.is_ok(),"reaped":true}));
        }
        parent_death_probe()?;
        println!(
            "{}",
            serde_json::json!({"version":"linux-process-snapshot-cleanup-v2","qualification_only":true,"cases":cases,"parent_death":{"signal":9,"reaped":true}})
        );
        Ok(())
    }

    pub fn guard_probe() -> Result<()> {
        let (ready, started) = std::sync::mpsc::channel();
        let (release, wait) = std::sync::mpsc::channel();
        let worker = std::thread::spawn(move || {
            ready.send(()).unwrap();
            let _ = wait.recv();
        });
        started.recv()?;
        let rejected = single_threaded().is_err();
        release.send(())?;
        worker
            .join()
            .map_err(|_| anyhow::anyhow!("guard worker failed"))?;
        ensure!(rejected, "multithreaded embedding was not rejected");
        single_threaded()?;
        println!(
            "{}",
            serde_json::json!({"qualification_only":true,"multithreaded_fork_rejected":true})
        );
        Ok(())
    }

    fn execute_restored(state: &mut State, mut output: UnixStream) -> ! {
        let result = (|| -> Result<()> {
            let before = state.call("check")?;
            let pages = state.call("pages")?;
            let elements = state.call("elements")?;
            let probe = state.call("probe")?;
            state.action("mutate")?;
            let after = state.call("check")?;
            ensure!((before, pages, elements, probe, after) == (64, 3, 3, 127, 125));
            for value in [before, pages, elements, probe, after] {
                output.write_all(&i64::from(value).to_le_bytes())?;
            }
            Ok(())
        })();
        // Do not unwind through inherited parent state or flush parent output.
        unsafe { _exit(if result.is_ok() { 0 } else { 100 }) }
    }

    fn template_loop(mut state: State, mut control: UnixStream) -> ! {
        let result = (|| -> Result<()> {
            control.write_all(b"s")?;
            loop {
                let mut command = [0];
                if control.read_exact(&mut command).is_err() || command[0] == b'q' {
                    break;
                }
                ensure!(command[0] == b'r');
                single_threaded()?;
                let (mut input, output) = UnixStream::pair()?;
                let parent_pid = unsafe { getpid() };
                // SAFETY: this private qualification executable owns its guest
                // and embedding; procfs proves only one thread at each fork.
                let pid = unsafe { fork() };
                if pid < 0 {
                    bail!("restore fork failed: {}", std::io::Error::last_os_error());
                }
                if pid == 0 {
                    if bind_parent_lifetime(parent_pid).is_err() {
                        unsafe { _exit(102) }
                    }
                    drop(control);
                    drop(input);
                    execute_restored(&mut state, output);
                }
                drop(output);
                // Wait before returning evidence; a child crash is not success.
                let mut restored = OwnedChild { pid };
                let proof = receive_proof(&mut input, &mut restored, Duration::from_secs(2))?;
                control.write_all(&proof)?;
            }
            Ok(())
        })();
        unsafe { _exit(if result.is_ok() { 0 } else { 101 }) }
    }

    pub fn run() -> Result<()> {
        ensure!(matches!(std::env::consts::ARCH, "aarch64" | "x86_64"));
        single_threaded()?;
        let wasm = wat::parse_str(FIXTURE)?;
        let wasm_sha256 = hex::encode(Sha256::digest(&wasm));
        let mut records = Vec::new();
        for (backend, strategy) in [
            ("cranelift", Strategy::Cranelift),
            ("winch", Strategy::Winch),
        ] {
            let mut config = Config::new();
            config.strategy(strategy).parallel_compilation(false);
            let engine = Engine::new(&config).map_err(runtime_error)?;
            let module = Module::new(&engine, &wasm).map_err(runtime_error)?;
            let mut store = Store::new(&engine, ());
            let instance = Instance::new(&mut store, &module, &[]).map_err(runtime_error)?;
            let mut state = State { store, instance };
            state.action("seed")?;
            ensure!(state.call("check")? == 64);
            // Clone all process state into a template, then mutate the source
            // independently before any restoration is requested.
            single_threaded()?;
            let (parent, child) = UnixStream::pair()?;
            parent.set_read_timeout(Some(Duration::from_secs(5)))?;
            let parent_pid = unsafe { getpid() };
            let pid = unsafe { fork() };
            if pid < 0 {
                bail!("capture fork failed");
            }
            if pid == 0 {
                if bind_parent_lifetime(parent_pid).is_err() {
                    unsafe { _exit(102) }
                }
                drop(parent);
                template_loop(state, child);
            }
            drop(child);
            let mut template = Template {
                child: OwnedChild { pid },
                control: parent,
            };
            let mut ready = [0];
            template.control.read_exact(&mut ready)?;
            ensure!(ready == *b"s");
            state.action("mutate")?;
            ensure!(state.call("check")? == 125);
            ensure!(
                state.call("probe").is_err(),
                "source dropped segments must trap"
            );
            drop(state);
            drop(module);
            drop(engine);
            for _ in 0..2 {
                template.control.write_all(b"r")?;
                let mut proof = [0; 40];
                template.control.read_exact(&mut proof)?;
                let values: Vec<i64> = proof
                    .chunks_exact(8)
                    .map(|b| i64::from_le_bytes(b.try_into().unwrap()))
                    .collect();
                ensure!(values == [64, 3, 3, 127, 125]);
            }
            template.finish()?;
            records.push(serde_json::json!({"backend":backend,"source_released_before_restore":true,"source_after_mutation":125,"restored_before_write":64,"restored_after_write":125,"passive_segment_probe":127,"restored_memory_pages":3,"restored_table_elements":3,"independent_restorations":2}));
        }
        println!(
            "{}",
            serde_json::json!({"scope":"linux_process_cow_clone","runtime":"wasmtime","version":"46.0.1","wasm_sha256":wasm_sha256,"architecture":std::env::consts::ARCH,"qualification_only":true,"headline_samples":false,"pre_fork_threads":1,"records":records})
        );
        Ok(())
    }
    pub fn write_fixture(path: &str) -> Result<()> {
        let wasm = wat::parse_str(FIXTURE)?;
        std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(path)?
            .write_all(&wasm)?;
        Ok(())
    }
}

#[cfg(target_os = "linux")]
fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.is_empty() {
        linux::run()
    } else if args.len() == 1 && args[0].starts_with("--adapter=") {
        linux::serve(args[0].trim_start_matches("--adapter="))
    } else if args == ["--test-thread-guard"] {
        linux::guard_probe()
    } else if args == ["--test-failure-cleanup"] {
        linux::failure_probe()
    } else if args.len() == 2 && args[0] == "--write-fixture" {
        linux::write_fixture(&args[1])
    } else if args.len() == 4 && args[0] == "--timed-stage" {
        linux::timed_run(&args[1], &args[2], args[3].parse()?)
    } else if args.len() == 3 && args[0] == "--density-worker" {
        linux::density_run(&args[1], args[2].parse()?)
    } else if args.len() == 4 && args[0] == "--density-failure-worker" {
        linux::density_failure_run(&args[1], args[2].parse()?, args[3].parse()?)
    } else {
        anyhow::bail!(
            "usage: qualify-process-snapshot [--test-thread-guard | --test-failure-cleanup | --write-fixture PATH | --timed-stage BACKEND STAGE SAMPLES | --density-worker BACKEND INSTANCES | --density-failure-worker BACKEND INSTANCES FAIL_AFTER]"
        )
    }
}
