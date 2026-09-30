// Package metrics owns definitions, not collectors. Missing is never zero.
package metrics

import (
	"fmt"
	"strings"
)

type Definition struct {
	Name          string `json:"name"`
	Version       int    `json:"version"`
	Unit          string `json:"unit"`
	Scope         string `json:"scope"`
	Boundary      string `json:"boundary"`
	MissingPolicy string `json:"missing_policy"`
}

var Registry = []Definition{
	{"process_group.boundary_pss_sum", 1, "bytes", "held_source_template_and_restored_process_group", "Sum of source, template and every simultaneously held restored child's smaps_rollup PSS at idle/touched/executed barriers. Sequential non-atomic readings; not physical memory peak, guest-only memory or summed RSS. Whole affected launch excluded when any group reading is unavailable.", "incomplete_smaps_coverage"},
	{"process_group.boundary_private_sum", 1, "bytes", "held_source_template_and_restored_process_group", "Sum of Private_Clean plus Private_Dirty from source, template and every held restored child at the declared barrier. Non-atomic boundary accounting, not peak residency or allocation volume. Whole affected launch excluded when any group reading is unavailable.", "incomplete_smaps_coverage"},
	{"snapshot.provision.elapsed", 1, "ns", "source_restore_request_to_all_live_ready_identity_frames", "Source-local monotonic restore request to all live ready identity frames for the declared restored process group. Memory-pass diagnostic includes process control/ready acknowledgments; not headline restore API-return latency. Inner groups reduce to one launch median.", "unavailable_without_complete_validated_group"},
	{"snapshot.child_touch.mean_elapsed", 1, "ns", "restored_child_local_region_mean", "Arithmetic mean of child-local elapsed regions writing the fixed fixture's three declared page-end bytes, one value per held group in the memory pass. Not summed group wall time, isolated COW faults or independent child replications. Inner groups reduce to one launch median.", "unavailable_without_complete_validated_group"},
	{"snapshot.child_execution.mean_elapsed", 1, "ns", "restored_child_local_region_mean", "Arithmetic mean of child-local fixed check execution regions after the declared memory touch. Memory-pass diagnostic, not headline execution latency, summed group wall time or independent child replications. Inner groups reduce to one launch median.", "unavailable_without_complete_validated_group"},
	{"host.rust.release.alloc.bytes", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Successful requested layout bytes during a separate logical-resource drop window after verification. Compile drops measured Module, engine held; instance/call scenarios drop measured Store state and prepared imports, module/engine held; steady only on final sample. Includes all Rust allocator threads, not mmap/foreign allocations or physical reclamation.", "not_applicable_when_store_retained"},
	{"host.rust.release.alloc.count", 1, "count", "allocations_routed_through_rust_global_allocator", "Successful alloc/zeroed/realloc requests during declared post-verification logical drop window, excluding observation encoding; not live object count.", "not_applicable_when_store_retained"},
	{"host.rust.release.freed.bytes", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Requested layout sizes logically freed during declared resource drop window; successful realloc includes old-size release. Not allocator purge, mmap unmapping or physical residency reduction.", "not_applicable_when_store_retained"},
	{"host.rust.release.outstanding.start", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Outstanding requested layout bytes after correctness verification, before logical resource drop; differs from API-return state because verification occurred in between.", "not_applicable_when_store_retained"},
	{"host.rust.release.outstanding.end", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Outstanding requested layout bytes after declared logical resource drop, before observation encoding/output buffering, while engine and declared module resources remain held. Not physical reclamation or a leak test.", "not_applicable_when_store_retained"},
	{"host.rust.release.outstanding.observed_peak", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Maximum outstanding requested layout bytes at serialized hook updates in separate logical drop window, including start baseline; excludes transient allocator-internal realloc copies and mmap/foreign memory.", "not_applicable_when_store_retained"},
	{"host.rust.release.elapsed", 1, "ns", "allocations_routed_through_rust_global_allocator", "Diagnostic elapsed time of declared logical resource drops after verification; includes instrumented hook overhead, excludes counter snapshots and observation encoding. Never headline teardown latency; no zero-valued pretend drop for retained steady samples.", "not_applicable_when_store_retained"},
	{"host.rust.alloc.bytes", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Successful Rust GlobalAlloc layout-request bytes during the single API-operation window, all threads. Successful realloc counts full new-size request even in-place. Excludes mmap, foreign allocators and usable-size/physical-residency accounting; not live bytes.", "unsupported"},
	{"host.rust.alloc.count", 1, "count", "allocations_routed_through_rust_global_allocator", "Successful alloc/alloc_zeroed/realloc calls during single API-operation window; not live object count or source-level allocation count (compiler may elide allocation).", "unsupported"},
	{"host.rust.freed.bytes", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Requested layout sizes logically released through dealloc or successful realloc during single API-operation window. Not allocator purge, physical reclamation or RSS reduction.", "unsupported"},
	{"host.rust.outstanding.start", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Process-wide outstanding requested layout bytes at API-window start; includes held adapter/runtime objects, not physical live heap or guest-only bytes.", "unsupported"},
	{"host.rust.outstanding.end", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Process-wide outstanding requested layout bytes at API-window end while operation output remains held, before verification and release; not physical retained memory.", "unsupported"},
	{"host.rust.outstanding.observed_peak", 1, "bytes", "allocations_routed_through_rust_global_allocator", "Maximum outstanding requested layout bytes at serialized hook-accounting updates during API window, including start baseline. Does not capture transient allocator-internal realloc copies; not a physical or mmap peak.", "unsupported"},
	{"work.throughput", 1, "declared_work_units/s", "workload_timed_regions", "Per independent successful timing launch: sum of non-warmup verified sample operations times declared units per invocation, divided by sum of their measured wall durations. Only first-call, steady and trajectory call scenarios. Excludes setup, verification and inter-sample gaps; sequence_call_sum includes only the timed calls. Median and bootstrap resample launch rates, not individual samples. Not sustained service throughput or an end-to-end rate.", "unavailable_for_ineligible_pass_invalid_sample_missing_work_contract_or_zero_total_duration"},
	{"density.guest_memory.logical", 1, "bytes", "instance_group_linear_memory", "Sum of accessible linear-memory lengths across all simultaneously held verified instances; not resident memory, host overhead or allocated bytes.", "unavailable"},
	{"source.build.cgroup.current", 1, "bytes", "tool_step_cgroup", "Charged cgroup memory after tool command wait returns, before descendant cleanup. Includes kernel/file charges; not RSS or retained heap. Fresh cgroup per recipe step; controller/staging/validation excluded.", "unavailable"},
	{"source.build.cgroup.peak", 1, "bytes", "tool_step_cgroup", "Fresh tool-step cgroup memory.peak from cgroup creation through post-wait snapshot, before descendant cleanup. Kernel-accounted lifetime peak; no reset or phase-peak inference; not RSS or heap.", "unavailable"},
	{"source.build.max_step_cgroup_peak", 1, "bytes", "sequential_tool_step_cgroups", "Maximum of all recipe-step fresh cgroup memory peaks. Not summed peaks, a simultaneous whole-build peak, RSS, allocation volume or retained heap. Cache charge ownership can differ across steps. Complete correctness-admitted builds only.", "unavailable_if_any_step_lacks_peak"},
	{"source.build.wait_cpu.user", 1, "ns", "os_waited_tool_process_usage", "Per-tool os.ProcessState.UserTime after exit. Descendant inclusion follows OS wait accounting, not guaranteed full process tree. Stored in CPU diagnostic step evidence; not compiler-pass-only CPU.", "unavailable"},
	{"source.build.wait_cpu.system", 1, "ns", "os_waited_tool_process_usage", "Per-tool os.ProcessState.SystemTime after exit. Descendant inclusion follows OS wait accounting, not guaranteed full process tree. Stored in CPU diagnostic step evidence; not compiler-pass-only CPU.", "unavailable"},
	{"source.build.wait_cpu", 1, "ns", "os_waited_tool_process_usage", "Sum of os.ProcessState user plus system CPU accounting across recipe steps after exit. Includes descendants only as accounted by OS wait semantics; not guaranteed whole-process-tree/cgroup coverage. Excludes controller, validation and oracle processes. Nanosecond storage, OS accounting precision. Dedicated CPU profile only.", "unavailable_if_any_step_lacks_accounting"},
	{"source.build.tool_wall", 1, "ns", "source_build_tool_processes", "Sum of sequential recipe-step exec.Cmd.Run wall durations per complete build, including process launch, output capture and wait. Excludes staging, hashing, independent Wasm validation, correctness checks and inter-step gaps; not whole-pipeline elapsed time. Only outputs identical to the correctness-admitted artifact are eligible.", "unavailable_for_failed_or_unadmitted_build"},
	{"time.cpu.total", 1, "ns", "adapter_cgroup_process_tree", "Delta of cpu.stat usage_usec across the diagnostic barrier window, converted to ns with microsecond source precision; all cgroup processes/threads and descendants, includes transport/background work, not guest-only CPU.", "unavailable"},
	{"time.cpu.user", 1, "ns", "adapter_cgroup_process_tree", "Delta of cpu.stat user_usec across the diagnostic barrier window; converted to ns, microsecond source precision.", "unavailable"},
	{"time.cpu.system", 1, "ns", "adapter_cgroup_process_tree", "Delta of cpu.stat system_usec across the diagnostic barrier window; converted to ns, microsecond source precision.", "unavailable"},
	{"cgroup.cpu.periods", 1, "count", "adapter_cgroup_local_bandwidth", "Delta of cpu.stat nr_periods; local fair-scheduler bandwidth periods, not inherited ancestor throttling.", "unavailable"},
	{"cgroup.cpu.throttled_periods", 1, "count", "adapter_cgroup_local_bandwidth", "Delta of cpu.stat nr_throttled; periods throttled by this cgroup's own bandwidth limit, not inherited ancestor throttling.", "unavailable"},
	{"cgroup.cpu.throttled_time", 1, "ns", "adapter_cgroup_local_bandwidth", "Delta of cpu.stat throttled_usec converted to ns; local bandwidth throttling duration, not executed CPU work or inherited ancestor throttling.", "unavailable"},
	{"cgroup.memory.anon", 1, "bytes", "adapter_cgroup", "memory.stat anon snapshot: anonymous mappings charged to cgroup; not guest-only or host-allocator-only memory.", "unavailable"},
	{"cgroup.memory.file", 1, "bytes", "adapter_cgroup", "memory.stat file snapshot: cached filesystem data including tmpfs/shared memory.", "unavailable"},
	{"cgroup.memory.kernel", 1, "bytes", "adapter_cgroup", "memory.stat kernel snapshot: total kernel accounting, overlaps page tables and slab; do not sum all breakdown fields.", "unavailable"},
	{"cgroup.memory.sock", 1, "bytes", "adapter_cgroup", "memory.stat sock snapshot: network transmission buffers.", "unavailable"},
	{"cgroup.memory.pagetables", 1, "bytes", "adapter_cgroup", "memory.stat pagetables snapshot: page tables; overlaps kernel accounting.", "unavailable"},
	{"cgroup.memory.slab", 1, "bytes", "adapter_cgroup", "memory.stat slab snapshot: in-kernel data structures; overlaps kernel accounting.", "unavailable"},
	{"cgroup.memory.phase_peak", 1, "bytes", "adapter_cgroup", "Peak read from the same memory.peak descriptor reset at the declared scenario start barrier and read at its end barrier; includes diagnostic transport and runtime background work. Compile, instantiate and app-init exclude subsequent verification/release; teardown measures release after verification.", "unavailable"},
	{"cgroup.memory.current", 1, "bytes", "adapter_cgroup", "Cgroup v2 memory.current at response end, includes descendant and kernel accounting; not process RSS or heap.", "unavailable"},
	{"cgroup.memory.peak", 1, "bytes", "adapter_cgroup", "Cgroup v2 memory.peak since fresh cgroup creation through response end, includes startup; not a reset phase peak.", "unavailable"},
	{"native.code_image", 1, "bytes", "compiled_module", "Native code image including guest instructions, wrappers and embedded data; not guest-only instructions.", "unsupported"},
	{"native.code_export", 1, "bytes", "compiled_module", "Availability of complete raw native code-image export; size limits never yield a truncated image.", "unavailable"},
	{"artifact.serialized", 1, "bytes", "compiled_module", "Complete serialized compiled artifact including code and metadata; not native instruction size.", "unsupported"},
	{"host.js_heap.start", 1, "bytes", "adapter_process_v8_heap", "V8 heapUsed before phase, without forced collection; excludes external and executable memory.", "unsupported"},
	{"host.js_heap.end", 1, "bytes", "adapter_process_v8_heap", "V8 heapUsed after phase, without forced collection; net change is not allocation volume.", "unsupported"},
	{"time.wall", 1, "ns", "embedding_api", "Monotonic elapsed time for the declared local operation batch; divide only by recorded operation count.", "unavailable"},
	{"process.rss", 1, "bytes", "adapter_process", "Resident set at a /proc snapshot; includes shared pages in full.", "unsupported"},
	{"process.peak_rss", 1, "bytes", "adapter_process", "Maximum resident set of the adapter child process over its whole trial lifetime from wait4 rusage; includes startup, setup and verification. Not a stage-only peak or cgroup peak.", "unavailable"},
	{"process.pss", 1, "bytes", "adapter_process", "Proportional resident set at a /proc/smaps_rollup snapshot.", "permission_denied_or_unsupported"},
	{"process.private", 1, "bytes", "adapter_process", "Sum of private clean and private dirty resident pages.", "permission_denied_or_unsupported"},
	{"process.virtual", 1, "bytes", "adapter_process", "VmSize at a /proc/status snapshot; not physical footprint.", "unsupported"},
	{"host.alloc.bytes", 1, "bytes", "adapter_process_go_heap", "Delta of cumulative TotalAlloc; all Go threads including adapter work, excludes native allocations.", "unsupported"},
	{"host.alloc.count", 1, "count", "adapter_process_go_heap", "Delta of cumulative Mallocs; not live object count.", "unsupported"},
	{"host.heap.end", 1, "bytes", "adapter_process_go_heap", "HeapAlloc after phase without forced collection; not a retained-live-heap claim.", "unsupported"},
	{"host.heap.start", 1, "bytes", "adapter_process_go_heap", "HeapAlloc before phase without forced collection.", "unsupported"},
	{"host.gc.cycles", 1, "count", "adapter_process_go_heap", "Delta of NumGC during the phase.", "unsupported"},
	{"host.gc.forced_cycles", 1, "count", "adapter_process_go_heap", "Delta of NumForcedGC: completed GC cycles forced by the application, a subset of host.gc.cycles. Collector never forces GC.", "unsupported"},
	{"host.gc.pause_time", 1, "ns", "adapter_process_go_heap", "Delta of PauseTotalNs between declared snapshots: cumulative GC stop-the-world pause accounting. Not concurrent GC CPU, API wall time, a pause percentile, or guest-only time; boundary-straddling GC accounting is not clipped to the API timer.", "unsupported"},
	{"guest.memory.logical", 1, "bytes", "guest_linear_memory", "Declared accessible memory length; not resident memory or guest allocations.", "not_applicable"},
	{"native.guest_code", 1, "bytes", "guest_function_code", "Emitted machine instructions belonging to guest functions, excluding stubs and metadata.", "unsupported_or_not_applicable"},
	{"native.function_range_bytes", 1, "bytes", "guest_function_ranges", "Sum of engine-reported compiled guest function range lengths; may include embedded constants and padding, not an instruction-only size.", "unsupported_or_not_applicable"},
	{"checkpoint.payload_bytes", 1, "bytes", "guest_state_checkpoint", "Eager checkpoint payload: entire fixed linear memory plus one 4-byte scalar global. Excludes host object headers/capacity, runtime state, code, active stacks, files and COW metadata; not serialized whole-instance size.", "unsupported"},
}

type Scenario struct {
	ID       string `json:"id"`
	Boundary string `json:"boundary"`
	Scope    string `json:"scope"`
}

var Scenarios = []Scenario{
	{"process-snapshot-density", "Memory-profile inspection of a simultaneously held restored process group at template, idle, three-Wasm-page-end write and execution barriers. Exact process lineage and state proofs; footprints include verification and embedding work. Individual raw RSS/PSS/private/virtual readings are not phase peaks, tree RSS, physical COW costs or headline latency.", "per_process_boundary_snapshot"},
	{"process-snapshot-capture", "Dedicated single-threaded Linux source fork to template-ready acknowledgment. Includes scheduling and readiness transport, not fork API-return latency. Compile/instantiate/seed, source mutation/release, restoration, full-state verification and cleanup excluded. Quiescent import-free fixed fixture; not an engine serializer or active guest-stack snapshot.", "process_control_roundtrip"},
	{"process-snapshot-restore", "Source restore-request dispatch to restored-child-ready acknowledgment through a retained template. Includes control transport, process creation and scheduling; not an engine restore API. Source Store/Module/Engine already released; capture/setup, first write, verification and child cleanup excluded.", "process_control_roundtrip"},
	{"process-snapshot-first-write", "One single-byte embedding memory write at offset 65535 in the restored child, using its local monotonic clock. Capture, restoration, lookup, state verification, passive-segment probe and cleanup excluded. Includes embedding write overhead; not isolated physical COW-fault latency or copied-page volume.", "embedding_api"},
	{"process-snapshot-execute", "One typed check call in a restored child after the declared first write, using that child's local monotonic clock. Fixed fixture checks memory/global/table behavior and grown sizes; full-memory hashes and passive-segment probe verified outside timing. Setup, capture/restore/write and cleanup excluded.", "embedding_api"},
	{"continuation-create", "Native wazero execution-stack Snapshot method entry to return inside a host callback at declared recursive depth. Excludes engine/module/instance setup, lookup, verification and release. Not memory/global/whole-instance/COW restoration.", "embedding_api"},
	{"continuation-resume", "Native wazero execution-stack Restore entry to first resumed guest host-marker entry. Restore does not return normally; includes control transfer and marker transition, not restore API-return latency. Capture, preparation, verification and release excluded.", "native_continuation_resumption"},
	{"continuation-first-write", "First embedding guest-write call after native stack restoration; updates last byte and global. Capture, restore, setup, verification and release excluded. No memory restoration or COW fault attribution.", "embedding_api"},
	{"continuation-execute", "Embedding full-memory checksum call after native stack restoration and declared first write; setup, capture, restoration, write, verification and release excluded. Memory/global changes survive native stack restoration.", "embedding_api"},
	{"sustained", "Fixed local batches on one fresh initialized instance retained across all samples and explicit warmup. Time only embedding calls; cumulative non-warmup API time must reach locked duration target. Monotonic session clocks preserve diagnostic/verification gaps. Release outside timers records runtime close or JS reference dropping, never physical reclamation. Go allocator and V8 heap snapshots are distinct memory-pass domains. Optional post-Go-collection is a separately locked memory-only diagnostic.", "embedding_api"},
	{"checkpoint-create", "Allocate and eagerly copy the entire fixed guest memory and one mutable i32 from a fresh initialized source. Exact generated fixture only; excludes engine/module/instance setup, verification, target restoration and release. Not a whole-instance or COW snapshot.", "embedding_api"},
	{"checkpoint-restore", "Copy an eager guest-state checkpoint into a fresh, already instantiated target memory and set its scalar global. Instantiation, checkpoint creation, verification and release excluded. Exact generated fixture only; no whole-instance or COW snapshot claim.", "embedding_api"},
	{"checkpoint-first-write", "First guest write call after eager guest-state restoration: changes last memory byte and scalar global. Restore/instantiation/setup and verification excluded. Ordinary eager-copy write, not COW fault or page materialization cost.", "embedding_api"},
	{"checkpoint-execute", "One guest full-memory checksum call after eager guest-state restoration, including embedding call. Restore/instantiation/setup, correctness verification and release excluded. Exact fixed-memory scalar fixture only.", "embedding_api"},
	{"density-cycle", "Retain fresh engines/compiled modules for the batch; each sample creates, initializes and invokes a fresh simultaneous instance group. Excludes engine/module setup, result verification and Store release. Retains the initial allocation cycle, no hidden prewarm; later cycles may reuse allocator resources. Memory barriers surround provisioning/verification and Store release while engines/modules remain held.", "embedding_api"},
	{"density", "Provision a fresh simultaneous instance group: engine construction, compile, instantiate/start, initialization/input and one workload invocation per instance; excludes result verification and group release. One operation is one complete group, no warmup. Memory barrier window additionally includes verification and transport; release does not imply reclamation.", "embedding_api"},
	{"guest-density", "Instantiate a simultaneous fresh group sharing one retained engine/module, then initialize all fixed memory plus scalar or eagerly restore a shared guest-state copy; optionally write one byte and scalar. Source initialization, checkpoint creation, compile, full-state/checksum verification and release excluded. One operation is one group; ready barrier before verification, released barrier after close. No whole-instance/COW snapshot, physically untouched-page or reclamation claim.", "embedding_api"},
	{"engine-init", "Construct an engine until usable; release outside timing.", "embedding_api"},
	{"harness-calibration", "Empty adapter-local loop writing iteration+1 to preallocated slots; no Wasm/embedding calls in timer. Workload checked before sampling, bookkeeping checked after. Operations are harness iterations, not guest invocations; no subtraction or workload scoring.", "adapter_local_harness"},
	{"compile", "Resident Wasm bytes to successful compile return, including mandatory validation; release outside timing.", "embedding_api"},
	{"compile-materialized", "One synchronous Wasmtime core compile from resident bytes to API return, including mandatory validation; complete defined-function native range coverage verified after the timer against independent input analysis. Dedicated code pass, diagnostic timer only. Engine construction, range inspection/export, instantiation/oracle and release excluded; no code retirement or instruction-only byte claim.", "embedding_api"},
	{"code-lifetime", "Dedicated code diagnostic: compile and bind one native image to its actual executable-text publication, verify a stateless call, drop Module handles, verify Store-owned code remains callable, then drop Store and engine. Native callback events and ownership checkpoints only; no timing samples, compiler emission or physical reclamation claim.", "native_code_publication"},
	{"instantiate", "Compiled module and prepared imports to returned instance; includes Wasm start function, excludes explicit _start.", "embedding_api"},
	{"app-init", "One explicit no-argument void initializer on a fresh instance; excludes compilation, instantiation/start function, input installation, workload verification and release; no warmup.", "embedding_api"},
	{"first-call", "First requested workload on a fresh instance following declared initialization, without warmup. Scalars time one call; exact_vectors time the sum of ordered calls (sequence_call_sum). Vector memory barriers span first-call entry through last-call return, including intercase input and verification but excluding the final oracle and release.", "embedding_api"},
	{"steady", "Local call batch after retained warmup observations; state policy from workload.", "embedding_api"},
	{"trajectory", "Sequential individual calls from invocation 1 on one fresh compiled-module instance, after declared initialization; no untimed benchmark pre-call. Warmup labels retained; verification between calls is outside timers. Tier remains unobserved unless separately reported.", "embedding_api"},
	{"cold-process", "Process launch to first correct result, including adapter and protocol setup.", "process_end_to_end"},
	{"aot-produce", "Wasm input to completed native artifact.", "embedding_api"},
	{"aot-load", "Existing native artifact to declared loaded state.", "embedding_api"},
	{"teardown", "One fresh instance per sample, initialized and workload-verified before timing resource release under the declared adapter policy. No warmup or implied allocator reclamation; setup and verification excluded.", "embedding_api"},
}

// Validate rejects ambiguous or incomplete metric and lifecycle contracts
// before they can be exported or used to label observations.
func Validate() error {
	return ValidateDefinitions(Registry, Scenarios)
}

func ValidateDefinitions(definitions []Definition, scenarios []Scenario) error {
	metricIDs := make(map[string]struct{}, len(definitions))
	for i, d := range definitions {
		if strings.TrimSpace(d.Name) == "" || d.Version < 1 || strings.TrimSpace(d.Unit) == "" || strings.TrimSpace(d.Scope) == "" || strings.TrimSpace(d.Boundary) == "" || strings.TrimSpace(d.MissingPolicy) == "" {
			return fmt.Errorf("metric definition %d is incomplete", i)
		}
		if _, exists := metricIDs[d.Name]; exists {
			return fmt.Errorf("duplicate metric definition %q", d.Name)
		}
		metricIDs[d.Name] = struct{}{}
	}
	scenarioIDs := make(map[string]struct{}, len(scenarios))
	for i, s := range scenarios {
		if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.Boundary) == "" || strings.TrimSpace(s.Scope) == "" {
			return fmt.Errorf("scenario definition %d is incomplete", i)
		}
		if _, exists := scenarioIDs[s.ID]; exists {
			return fmt.Errorf("duplicate scenario definition %q", s.ID)
		}
		scenarioIDs[s.ID] = struct{}{}
	}
	if len(definitions) == 0 || len(scenarios) == 0 {
		return fmt.Errorf("metric and scenario registries must be nonempty")
	}
	return nil
}
