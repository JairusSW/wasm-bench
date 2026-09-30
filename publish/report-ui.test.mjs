import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

// Exercise the shipped trajectory renderer with a small DOM contract double.
// These are deterministic rendering/state tests, not browser or layout tests.
const html = fs.readFileSync(new URL('./report.html', import.meta.url), 'utf8');

test('density process explorer keeps exact identities, role filters, all groups and failed prefixes',()=>{
  const root=new Element('div'),node=(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;};
  const source={group_index:0,stage:'idle',role:'source',child_index:null,process:{pid:99,start_time_ticks:'9007199254740993'},parent_pid:98,start_ns:'9007199254740994',end_ns:'9007199254740995',rss_bytes:'0',pss_bytes:null,private_bytes:null,virtual_bytes:'9007199254740996',smaps_status:'permission_denied',smaps_reason:'denied fixture',raw_record_index:1,raw_reading_index:0};
  const child={...source,role:'restored',child_index:0,raw_reading_index:2};
  const view={trial:'t',runtime:'native',status:'ok',raw_trial:'raw/trials/t.json',retained_boundaries:8,rows:[source,child,{...child,group_index:31,stage:'executed'}]};
  const context=vm.createContext({el:()=>root,node,fmtBytes:n=>n+' B'});
  vm.runInContext(html.slice(html.indexOf('function snapshotFootprintBytes('),html.indexOf('function snapshotBoundaryLabel(')),context);
  vm.runInContext(html.slice(html.indexOf('function openDensityFootprints('),html.indexOf('function drawSnapshotDensity(')),context);
  context.openDensityFootprints(view);
  const selects=descendants(root,n=>n.tag==='select');assert.equal(selects[0].children.length,2);assert.equal(selects[0].children[1].value,'31');
  assert.ok(descendants(root,n=>n.textContent==='0 B').length);
  assert.ok(descendants(root,n=>n.textContent==='9007199254740996 B').length);
  assert.ok(descendants(root,n=>n.textContent==='Unavailable').length);
  const buttons=descendants(root,n=>n.tag==='button');buttons[1].listeners.click();
  assert.ok(descendants(root,n=>/groups\[0\].records\[1\].readings\[2\]/.test(n.textContent)).length);
  assert.ok(descendants(root,n=>/9007199254740994 → 9007199254740995/.test(n.textContent)).length);
  selects[2].value='restored';selects[2].listeners.change();assert.equal(descendants(root,n=>n.tag==='button').length,1);assert.equal(descendants(root,n=>n.tag==='pre').length,0);
  selects[0].value=31;selects[1].value='executed';selects[1].listeners.change();assert.equal(descendants(root,n=>n.tag==='button').length,1);
  selects[1].value='template_after_source_release';selects[1].listeners.change();assert.equal(descendants(root,n=>n.tag==='button').length,0);assert.ok(descendants(root,n=>/Missing is not zero/.test(n.textContent)).length);
  context.openDensityFootprints({...view,status:'error',reason:'child failed'});assert.equal(descendants(root,n=>n.tag==='select').length,0);assert.equal(descendants(root,n=>n.tag==='a')[0].href,'raw/trials/t.json');assert.ok(descendants(root,n=>/Failed prefixes are raw diagnostics only/.test(n.textContent)).length);
});

test('restored density keeps missing gaps, zero, paired negative costs and exact raw links',()=>{
  const elements=new Map(['snapshot-density-chart','snapshot-density-detail','snapshot-density-choice'].map(id=>[id,new Element('div')]));elements.get('snapshot-density-choice').value=0;
  const point=(size,median)=>({size,median,workload:'w'+size,independent_launches:2,attempted_launches:2,status:median===null?'unavailable':'insufficient_launches',outcomes:{ok:2}});
  const curve={runtime:'native',scenario:'process-snapshot-density',profile:'memory',measurement:{phase:'idle',metric:'process_group.boundary_pss_sum'},points:[point(1,0),point(2,null),point(4,40)],marginal_costs:[{from_size:1,to_size:2,median_per_added_unit:-4,paired_blocks:2,status:'insufficient_blocks'}]};
  const trial={id:'raw exact',block:0,runtime_configuration:'native',workload:'w1',scenario:curve.scenario,profile:'memory',status:'ok'};
  const context=vm.createContext({data:{snapshot_density_curves:[curve],bundle:{trials:[trial,{...trial,id:'admission',block:-1}]}},el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},fmtBytes:n=>n+' B',fmt:n=>n+' ns',document:{createElementNS:(_,tag)=>new Element(tag)}});
  vm.runInContext(html.slice(html.indexOf('function densityPointTrials('),html.indexOf('if((data.snapshot_density_curves')),context);
  context.drawSnapshotDensity();const root=elements.get('snapshot-density-chart');
  assert.equal(descendants(root,n=>n.tag==='circle').length,2);
  assert.equal(descendants(root,n=>n.tag==='line').length,0,'must not bridge missing counts');
  assert.ok(descendants(root,n=>n.textContent==='0 B').length);
  assert.ok(descendants(root,n=>n.textContent==='−4 B').length);
  const dots=descendants(root,n=>n.tag==='circle');dots[0].listeners.keydown({key:'Enter',preventDefault(){}});
  const links=descendants(elements.get('snapshot-density-detail'),n=>n.tag==='a');assert.equal(links.length,1);assert.equal(links[0].href,'raw/trials/raw%20exact.json');
  curve.points=[];context.drawSnapshotDensity();assert.equal(elements.get('snapshot-density-detail').children.length,0);assert.equal(descendants(root,n=>n.tag==='circle').length,0);
  curve.measurement.unit='ns';curve.measurement.normalization_denominator='restored_child';curve.points=[point(1,0)];curve.marginal_costs[0].ci95_low=-8;curve.marginal_costs[0].ci95_high=2;context.drawSnapshotDensity();
  assert.ok(descendants(root,n=>/Diagnostic memory-pass clocks, not headline latency/.test(n.textContent)).length);
  assert.ok(descendants(root,n=>n.textContent==='0 ns').length);
  assert.ok(descendants(root,n=>n.textContent==='−8 ns – 2 ns').length,'marginal intervals preserve signs and units');
  assert.equal(descendants(root,n=>n.textContent==='Group footprint median').length,0);
});

test('process snapshots use explicit lifecycle names and distinct stage tones',()=>{
  const context=vm.createContext({});vm.runInContext(html.slice(html.indexOf('function overviewStages('),html.indexOf('function compositeEntries(')),context);
  const stages=context.overviewStages(['process-snapshot-execute','process-snapshot-first-write','process-snapshot-restore','process-snapshot-capture']);
  assert.deepEqual(Array.from(stages,s=>[s.label,s.tone]),[['Process capture','compile'],['Child restoration','instantiate'],['First write','first-call'],['Post-restore execution','execution']]);
  assert.ok(stages.every(s=>s.group==='Linux process snapshots'));
});

test('snapshot footprint viewer separates roles, exact identities, zero and denied smaps',()=>{
  const ids=['snapshot-process-chart','snapshot-process-detail','snapshot-process-trial','snapshot-process-sample','snapshot-process-role','snapshot-process-metric'];
  const elements=new Map(ids.map(id=>[id,new Element('div')]));
  elements.get('snapshot-process-metric').value='rss_bytes';
  const row={sample_index:0,stage:'restore_ready',restoration:0,role:'restored',process:{pid:103,start_time_ticks:'9007199254740993'},parent_pid:102,start_ns:'9007199254740994',end_ns:'9007199254740995',rss_bytes:'0',pss_bytes:null,private_bytes:null,virtual_bytes:'9007199254740996',smaps_status:'permission_denied',smaps_reason:'raw denial',raw_record_index:1,raw_reading_index:2};
  const view={run:'native',runtime:'r',trial:'one',scenario:'process-snapshot-restore',status:'ok',raw_trial:'raw-memory/trials/one.json',rows:[{...row,role:'source',rss_bytes:'4096'},row,{...row,sample_index:999,restoration:1,rss_bytes:'2048'}]};
  const context=vm.createContext({snapshotProcessViews:[view,{...view,trial:'bad',status:'error',reason:'child failed',retained_boundaries:3,raw_trial:'raw/trials/bad.json'}],el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},fmtBytes:n=>n+' B'});
  vm.runInContext(html.slice(html.indexOf('function snapshotFootprintBytes('),html.indexOf('const snapshotProcessViews=')),context);
  context.resetSnapshotFootprintSample();
  const root=elements.get('snapshot-process-chart');
  assert.equal(elements.get('snapshot-process-sample').children.length,2,'all sample indices must be reachable');
  assert.equal(descendants(root,n=>n.tag==='a')[0].href,'raw-memory/trials/one.json');
  assert.deepEqual(descendants(root,n=>n.className==='runtime-bar-fill').map(n=>n.style.width),['100%','0%'],'measured zero is not missing or inflated');
  assert.deepEqual(descendants(root,n=>n.className?.startsWith('runtime-bar snapshot-')).map(n=>n.className),['runtime-bar snapshot-source','runtime-bar snapshot-restored'],'roles have explicit color styling');
  assert.ok(descendants(root,n=>n.textContent.includes('9007199254740993')).length,'birth ticks remain exact strings');
  const buttons=descendants(root,n=>n.className==='runtime-bar-button');buttons[1].listeners.click();
  const detail=elements.get('snapshot-process-detail');assert.equal(detail.hidden,false);
  assert.ok(descendants(detail,n=>n.tag==='pre'&&n.textContent.includes('9007199254740995')).length,'collector brackets never pass through Number');
  assert.ok(descendants(detail,n=>n.textContent.includes('records[1].readings[2]')).length);
  elements.get('snapshot-process-metric').value='pss_bytes';context.drawSnapshotFootprints();
  assert.equal(detail.hidden,true,'changing selection clears stale details');
  assert.equal(descendants(root,n=>n.className==='runtime-bar-fill').length,0,'denied PSS has no invented bar');
  assert.ok(descendants(root,n=>n.title?.includes('raw denial')).length);
  elements.get('snapshot-process-metric').value='virtual_bytes';elements.get('snapshot-process-role').value='restored';context.drawSnapshotFootprints();
  assert.equal(descendants(root,n=>n.className==='runtime-bar-button').length,1,'process roles are not added together');
  assert.ok(descendants(root,n=>n.textContent==='9007199254740996 B').length,'large byte counts remain exact');
  elements.get('snapshot-process-sample').value=999;context.drawSnapshotFootprints();
  assert.ok(descendants(root,n=>n.textContent.includes('restoration 2')).length,'later restorations are reachable');
  elements.get('snapshot-process-trial').value=1;context.resetSnapshotFootprintSample();
  assert.equal(descendants(root,n=>n.className==='runtime-bar-fill').length,0,'failed prefixes never graph surviving numbers');
  assert.ok(descendants(root,n=>n.textContent.includes('child failed')&&n.textContent.includes('3 retained')).length);
  assert.equal(descendants(root,n=>n.tag==='a')[0].href,'raw/trials/bad.json');
  assert.equal(elements.get('snapshot-process-sample').disabled,true,'failed prefixes have no successful sample selector');
  elements.get('snapshot-process-role').value='unknown';elements.get('snapshot-process-trial').value=0;context.resetSnapshotFootprintSample();
  assert.ok(descendants(root,n=>n.textContent.includes('No process readings')).length,'empty filters clear prior charts');
});

test('code lifetime viewer retains exact addresses, zero retirement, raw links and missing outcomes',()=>{
  const root=new Element('div');
  const l={collector:'Wasmtime/CustomCodeMemory',collector_version:'46.0.1',scope:'published_executable_text_capacity',quality:'engine_callback',image_sha256:'image',publication:1,image_offset:0,page_size:4096,bti:true,result_before_drop:['7'],result_after_drop:['7'],events:[{kind:'published',sequence:0,elapsed_ns:1,publication:1,address:'9007199254740992',capacity:4096,active_capacity:4096,cumulative_published_capacity:4096},{kind:'unpublished',sequence:1,elapsed_ns:5,publication:1,address:'9007199254740992',capacity:4096,active_capacity:0,cumulative_published_capacity:4096}],checkpoints:[{stage:'engine_dropped',event_count:2,elapsed_ns:6,active_capacity:0,cumulative_published_capacity:4096}]};
  const trials=[{id:'good',runtime_configuration:'cranelift',workload:'w',scenario:'code-lifetime',block:0,status:'ok',code_lifetime:l},{id:'missing',runtime_configuration:'ordinary',workload:'w',scenario:'code-lifetime',block:0,status:'unsupported',reason:'capability missing'}];
  const context=vm.createContext({node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},document:{createElementNS:(_,tag)=>new Element(tag)},fmt:n=>n+' ns',fmtBytes:n=>n+' B'});
  vm.runInContext(html.slice(html.indexOf('function codeLifetimeEvidence('),html.indexOf('function engineTraceEvidence(')),context);
  context.codeLifetimeEvidence(root,trials);
  assert.equal(descendants(root,n=>n.tag==='a')[0].href,'raw/trials/good.json');
  assert.equal(descendants(root,n=>n.textContent==='9007199254740992').length,2,'addresses are never converted to imprecise numbers');
  assert.ok(descendants(root,n=>n.textContent==='0 B').length,'completed zero active capacity remains measured');
  assert.equal(descendants(root,n=>n.tag==='th'&&n.textContent==='Diagnostic elapsed').length,2,'formatted time columns do not claim a fixed ns unit');
  assert.equal(descendants(root,n=>n.tag==='td'&&n.title==='5 ns').length,1,'exact diagnostic nanoseconds remain available');
  const paths=descendants(root,n=>n.tag==='polyline');assert.equal(paths.length,2);
  assert.match(paths[0].attributes.points,/975,40$/);assert.match(paths[1].attributes.points,/975,155$/);
  const select=root.children[0];select.value=1;select.listeners.change();
  assert.ok(descendants(root,n=>/Missing is not zero/.test(n.textContent)).length);
  assert.equal(descendants(root,n=>n.tag==='polyline').length,0,'unavailable outcomes clear previous measured graph');
  assert.equal(descendants(root,n=>n.tag==='a')[0].href,'raw/trials/missing.json');
});

test('Rust allocator trial selection uses the sealed runtime_configuration field',()=>{
  const context=vm.createContext({});
  vm.runInContext(html.slice(html.indexOf('function selectRustAllocatorTrials('),html.indexOf('const rustAllocatorTrials=')),context);
  const runtimes=[{id:'native',description:{capabilities:{requires_memory_profile:true}}},{id:'ordinary',description:{capabilities:{}}}];
  const trials=[{id:'admission',block:-1,runtime_configuration:'native'},{id:'diagnostic',block:0,runtime_configuration:'native'},{id:'unsupported',block:0,runtime_configuration:'native',status:'unsupported'},{id:'ordinary',block:0,runtime_configuration:'ordinary'},{id:'incorrect-field',block:0,runtime:'native'}];
  assert.deepEqual(Array.from(context.selectRustAllocatorTrials(trials,runtimes),t=>t.id),['diagnostic','unsupported']);
});

test('Rust allocator viewer retains zero, gaps, raw evidence and bounded sample windows',()=>{
  const elements=new Map(['rust-allocator-chart','rust-allocator-trial','rust-allocator-window'].map(id=>[id,new Element('div')]));
  const trial={id:'rust-0',status:'ok',samples:Array.from({length:251},(_,index)=>({index,observations:[{metric:'host.rust.alloc.bytes',unit:'bytes',status:'available',value:0},{metric:'host.rust.alloc.count',unit:'count',status:'available',value:0}]}))};
  const context=vm.createContext({rustAllocatorTrials:[trial],el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);if(text!==undefined)n.textContent=text;return n;},fmtBytes:n=>n+' B'});
  vm.runInContext(html.slice(html.indexOf('function drawRustAllocator('),html.indexOf('if(rustAllocatorTrials.length)')),context);
  context.resetRustAllocatorWindow();
  const root=elements.get('rust-allocator-chart'),table=root.children.at(-1).children[0];
  assert.equal(table.children.length,251);
  assert.equal(table.children[1].children[1].textContent,'0 B');
  assert.equal(table.children[1].children[2].textContent,'0');
  assert.equal(table.children[1].children[3].textContent,'Unavailable');
  assert.equal(root.children[1].href,'raw/trials/rust-0.json');
  assert.equal(elements.get('rust-allocator-window').children.length,2);
  elements.get('rust-allocator-window').value=250;context.drawRustAllocator();
  assert.equal(root.children.at(-1).children[0].children[1].children[0].textContent,'251');
  trial.samples=[];trial.status='unsupported';context.resetRustAllocatorWindow();
  assert.match(root.children.at(-1).textContent,/Missing is not zero/);
});

test('Rust release viewer separates retained samples, completed zero values and diagnostic clocks',()=>{
  const elements=new Map(['rust-allocator-chart','rust-allocator-trial','rust-allocator-window'].map(id=>[id,new Element('div')]));
  const suffixes=['alloc.bytes','alloc.count','freed.bytes','outstanding.start','outstanding.end','outstanding.observed_peak','elapsed'];
  const samples=[false,true].map((performed,index)=>({index,observations:suffixes.map(suffix=>({metric:'host.rust.release.'+suffix,status:performed?'available':'not_applicable',...(performed?{value:0}:{}),unit:suffix==='elapsed'?'ns':suffix==='alloc.count'?'count':'bytes'}))}));
  const trial={id:'release-0',runtime_configuration:'native',status:'ok',samples};
  const context=vm.createContext({rustAllocatorTrials:[trial],manifest:{lock:{runtime_configurations:[{id:'native',description:{effective_configuration:{allocator_release_policy:'locked release policy'}}}]}},el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);if(text!==undefined)n.textContent=text;return n;},fmtBytes:n=>n+' B',fmt:n=>n+' ns'});
  vm.runInContext(html.slice(html.indexOf('function drawRustAllocator('),html.indexOf('if(rustAllocatorTrials.length)')),context);
  context.resetRustAllocatorWindow();
  const root=elements.get('rust-allocator-chart'),table=root.children.at(-1).children[0];
  assert.ok(root.children.some(n=>n.textContent==='locked release policy'));
  assert.deepEqual(table.children[1].children.slice(1).map(n=>n.textContent),Array(7).fill('Retained'));
  assert.deepEqual(table.children[2].children.slice(1).map(n=>n.textContent),['0 B','0 B','0 B','0 B','0','0 B','0 ns']);
});
const start = html.indexOf('function timingSamples(');

test('allocator process boundaries keep snapshot domains, retained stores, zero and unavailable outcomes',()=>{
  const elements=new Map(['rust-allocator-chart','rust-allocator-trial','rust-allocator-window'].map(id=>[id,new Element('div')]));
  const trial={id:'boundaries',status:'ok',samples:[{index:0,observations:[{metric:'host.rust.release.elapsed',status:'not_applicable'}]},{index:1,observations:[]}],phase_events:[{event:{sample_index:0,stage:'steady_release_decision_completed'},observations:[{metric:'process.rss',value:0,status:'available',scope:'adapter_process',quality:'boundary_snapshot_only'},{metric:'process.pss',status:'permission_denied',reason:'fixture permission',scope:'adapter_process',quality:'boundary_snapshot_only'}]},{event:{sample_index:1,stage:'steady_release_decision_completed'},observations:[]}]};
  const context=vm.createContext({rustAllocatorTrials:[trial],manifest:{lock:{runtime_configurations:[]}},el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);if(text!==undefined)n.textContent=text;return n;},fmtBytes:n=>n+' B',fmt:n=>n+' ns'});
  vm.runInContext(html.slice(html.indexOf('function drawRustAllocator('),html.indexOf('if(rustAllocatorTrials.length)')),context);
  context.resetRustAllocatorWindow();
  const table=elements.get('rust-allocator-chart').children.at(-1).children[0];
  assert.deepEqual(table.children[1].children.map(c=>c.textContent),['1','Store retained','0 B','permission denied','Not recorded','Not recorded']);
  assert.equal(table.children[1].children[3].title,'adapter_process · boundary_snapshot_only · fixture permission');
  assert.equal(table.children[2].children[1].textContent,'Release decision not recorded');
  elements.get('rust-allocator-window').value=250;context.drawRustAllocator();
  assert.match(elements.get('rust-allocator-chart').children.at(-1).textContent,/No accepted allocator samples/);
});
const end = html.indexOf('function detail(', start);
assert.ok(start >= 0 && end > start, 'trajectory renderer must exist');

class Element {
  constructor(tag) {
    this.tag = tag; this.children = []; this.attributes = {}; this.listeners = {}; this.style = {};
    this.textContent = ''; this.namespaceURI = 'http://www.w3.org/2000/svg';
  }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; this.selected = undefined; }
  setAttribute(key, value) { this.attributes[key] = String(value); }
  addEventListener(name, listener) { this.listeners[name] = listener; }
  get value() { return this.selected ?? this.children[0]?.value ?? ''; }
  set value(value) { this.selected = String(value); }
  select(value) { this.value = value; this.listeners.change(); }
}
function descendants(root, predicate) {
  return root.children.flatMap(child => [ ...(predicate(child) ? [child] : []), ...descendants(child, predicate) ]);
}

test('harness evidence preserves zero, failed prefixes, sacrificial checks, exact configuration and every sample window',()=>{
  const elements=new Map(['harness-view','harness-trial','harness-window','harness-evidence','overview-comparison','evidence-panel'].map(id=>[id,new Element('div')]));
  const sample={index:0,operations:1000,elapsed_ns:0,sample_type:'batch_average',verified:true,result:['1000']};
  const trial={id:'same/id',scenario:'harness-calibration',runtime_configuration:'r',workload:'w',block:1,status:'ok',profile:'timing',samples:Array.from({length:501},(_,index)=>({...sample,index}))};
  const trials=[trial,{...trial,id:'bad',status:'error',reason:'wrong oracle',samples:[sample]},{...trial,id:'sacrificial',block:-1,samples:[sample]},{...trial,id:'unsupported',status:'unsupported',samples:[]}];
  const config={id:'r',effective_configuration:{harness_calibration_policy:'exact policy'}};
  const context=vm.createContext({el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},manifest:{lock:{runtime_configurations:[config],options:{scenarios:['harness-calibration']}}}});
  vm.runInContext(html.slice(html.indexOf('function harnessSampleValue('),html.indexOf('function tierTrajectory(')),context);
  context.renderHarnessEvidence(trials);
  const root=elements.get('harness-evidence');
  assert.equal(elements.get('harness-view').hidden,false);
  assert.equal(elements.get('overview-comparison').hidden,true);
  assert.equal(elements.get('evidence-panel').open,true);
  assert.equal(elements.get('harness-trial').children.length,4);
  assert.equal(elements.get('harness-window').children.length,3);
  assert.ok(descendants(root,n=>n.textContent==='0.000').length);
  assert.equal(descendants(root,n=>n.tag==='a')[0].href,'raw/trials/same%2Fid.json');
  assert.equal(descendants(root,n=>n.tag==='a')[1].href,'#reproduce');
  assert.equal(descendants(root,n=>n.tag==='pre')[0].textContent,JSON.stringify(config,null,2));
  elements.get('harness-window').select(500);
  assert.equal(descendants(root,n=>n.tag==='tr').length,2);
  assert.ok(descendants(root,n=>n.textContent==='500 / batch_average').length);
  elements.get('harness-trial').select(1);
  assert.equal(elements.get('harness-window').value,'0');
  assert.ok(descendants(root,n=>n.textContent==='Not applicable / invalid diagnostic').length);
  assert.ok(descendants(root,n=>n.textContent.includes('wrong oracle')).length);
  elements.get('harness-trial').select(2);
  assert.ok(descendants(root,n=>n.textContent.includes('not calibration iterations')).length);
  assert.ok(descendants(root,n=>n.textContent==='Not applicable / invalid diagnostic').length);
  elements.get('harness-trial').select(3);
  assert.ok(descendants(root,n=>n.textContent==='No samples recorded. Missing is not zero.').length);
  for(const changed of [{operations:0},{elapsed_ns:null},{elapsed_ns:9007199254740992},{verified:false},{result:['999']},{warmup:true},{sample_type:'individual_operation'},{observations:[{}]}])assert.equal(context.harnessSampleValue(trial,{...sample,...changed}),null);
  context.renderHarnessEvidence([]);assert.equal(elements.get('harness-view').hidden,true);assert.equal(root.children.length,0);
  context.manifest.lock.options.scenarios=['compile','harness-calibration'];context.renderHarnessEvidence(trials);assert.equal(elements.get('overview-comparison').hidden,false);
  vm.runInContext(html.slice(html.indexOf('function overviewStages('),html.indexOf('function compositeEntries(')),context);
  assert.deepEqual(Array.from(context.overviewStages(['compile','harness-calibration']),s=>s.id),['compile']);
});

test('break-even points expose exact prefixes, configuration, coverage and raw evidence by click or keyboard',()=>{
  const elements=new Map(['break-even-chart','break-even-detail','break-even-choice','break-even-curves','break-even-runtime'].map(id=>[id,new Element('div')]));
  let revealed=0;elements.get('break-even-detail').scrollIntoView=()=>{revealed++;};
  elements.get('break-even-choice').value='w';elements.get('break-even-chart').clientWidth=800;
  const point=(n,total)=>({invocations:n,median_total_ns:total,ci95_low:null,ci95_high:null});
  const curve={runtime:'r',workload:'w',setup_scenarios:['compile','instantiate'],status:'insufficient_blocks',planned_blocks:3,complete_blocks:[0],source_trials:['compile / exact','instantiate','trajectory'],points:[point(0,0),point(1,10),point(2,null),point(5,30)]};
  const trials=curve.source_trials.map((id,i)=>({id,block:0,runtime_configuration:'r',workload:'w',scenario:['compile','instantiate','trajectory'][i]}));
  const configuration={id:'r',description:{backend:'precise-backend',effective_configuration:{cache:'disabled'}}};
  const context=vm.createContext({
    el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=String(text??'');return n;},fmt:n=>n===null?'Unavailable':n+' ns',
    breakEven:{curves:[curve],analysis_version:'observed-prefix-block-bootstrap-v2',method:'complete blocks'},
    manifest:{lock:{options:{warmup:2},runtime_configurations:[configuration]}},data:{bundle:{trials}},
    document:{createElementNS:(_,tag)=>new Element(tag)},
  });
  const start=html.indexOf('function drawBreakEven(');
  vm.runInContext(html.slice(start,html.indexOf('</script>',start)),context);
  context.drawBreakEven();const root=elements.get('break-even-chart'),detail=elements.get('break-even-detail');
  const svg=descendants(root,n=>n.tag==='svg')[0];assert.equal(svg.attributes.role,'group');
  const points=descendants(root,n=>n.tag==='circle');assert.equal(points.length,3,'missing points are not clickable zeroes');
  const buttons=descendants(elements.get('break-even-curves'),n=>n.tag==='button');assert.equal(buttons.length,3,'overlapping points retain independent inspection buttons');buttons[2].listeners.click();assert.ok(elements.get('break-even-detail').children.some(n=>/N=5/.test(n.textContent)));
  for(const p of points){assert.equal(p.attributes.role,'button');assert.equal(p.attributes.tabindex,'0');assert.match(p.attributes['aria-label'],/open point evidence/);}
  assert.equal(descendants(root,n=>n.tag==='line'&&n.attributes.stroke==='#70cbbb'&&n.attributes.x1!==n.attributes.x2).length,1,'missing points break the connecting line');
  points[0].listeners.click();assert.equal(detail.hidden,false);
  assert.equal(revealed,2,'activation brings point evidence into view');
  assert.ok(descendants(detail,n=>n.textContent==='Synthetic total: 0 ns').length,'measured zero remains inspectable');
  assert.ok(descendants(detail,n=>/No call time at N=0/.test(n.textContent)).length);
  assert.ok(descendants(detail,n=>/interval unavailable/.test(n.textContent)).length);
  let prevented=false;points[2].listeners.keydown({key:'Enter',preventDefault(){prevented=true;}});assert.equal(prevented,true);
  assert.ok(descendants(detail,n=>/first 5 observed calls, including 2 warmup/.test(n.textContent)).length);
  assert.ok(descendants(detail,n=>/Samples 0–4/.test(n.textContent)).length);
  assert.ok(descendants(detail,n=>/Complete blocks: 1 \/ 3/.test(n.textContent)).length);
  assert.deepEqual(descendants(detail,n=>n.tag==='a').map(n=>n.href),['raw/trials/compile%20%2F%20exact.json','raw/trials/instantiate.json','raw/trials/trajectory.json','#reproduce']);
  assert.equal(descendants(detail,n=>n.tag==='pre')[0].textContent,JSON.stringify(configuration,null,2));
  const retained=detail.children.slice();context.drawBreakEven({preserveDetail:true});assert.equal(detail.hidden,false);assert.deepEqual(detail.children,retained,'resize redraw preserves selected evidence');
  points[1].listeners.keydown({key:' ',preventDefault(){}});assert.ok(descendants(detail,n=>/first 1 observed calls, including 1 warmup/.test(n.textContent)).length);
  curve.points[1].ci95_low=8;curve.points[1].ci95_high=12;points[1].listeners.click();assert.ok(descendants(detail,n=>n.textContent==='Pointwise 95% interval: 8 ns – 12 ns').length);
  descendants(detail,n=>n.tag==='button')[0].listeners.click();assert.equal(detail.hidden,true);assert.equal(detail.children.length,0);
  points[0].listeners.click();elements.get('break-even-choice').value='missing';context.drawBreakEven();assert.equal(detail.hidden,true);assert.equal(detail.children.length,0,'changing workloads clears stale point evidence');assert.equal(descendants(root,n=>n.tag==='circle').length,0);
  elements.get('break-even-choice').value='w';elements.get('break-even-runtime').value='absent';context.drawBreakEven();assert.equal(descendants(root,n=>n.tag==='circle').length,0,'runtime filtering removes other configurations');assert.equal(descendants(elements.get('break-even-curves'),n=>n.tag==='button').length,0);
  elements.get('break-even-runtime').value='r';context.drawBreakEven();assert.equal(descendants(root,n=>n.tag==='circle').length,3);
});
function renderer() {
  const context = vm.createContext({
    document: {createElementNS: (_, tag) => new Element(tag)},
    node: (tag, text) => { const n = new Element(tag); n.textContent = text ?? ''; return n; },
    fmt: n => `${n} ns`,
  });
  vm.runInContext(html.slice(start, end), context);
  return context;
}
function sample(index, extra = {}) {
  return {index, elapsed_ns: index + 10, operations: 1, verified: true, sample_type: 'individual_operation', ...extra};
}
function trial(id, count, extra = {}) {
  return {id, block: 0, status: 'ok', profile: 'timing', samples: Array.from({length: count}, (_, i) => sample(i)), ...extra};
}
const byID = (root, id) => descendants(root, n => n.id === id)[0];
const dots = root => descendants(root, n => n.tag === 'circle');
const titles = root => descendants(root, n => n.tag === 'title').map(n => n.textContent);

test('engine trace viewer retains native zeros, incomplete outcomes, windows and raw downloads',()=>{
  const context=vm.createContext({node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},TextDecoder,Uint8Array,atob,document:{createElementNS:(_,tag)=>new Element(tag)}});
  vm.runInContext(html.slice(html.indexOf('function engineTimelineModel('),html.indexOf('function timingSamples(')),context);
  const events=Array.from({length:1001},(_,i)=>({pid:1,tid:2,ts:i,dur:0,ph:'X',cat:'v8.wasm',name:'event-'+i,args:{id:i}}));
  const trace={status:'incomplete',reason:'retained prefix',collector:'NodeTracing',collector_version:'test',scope:'process-wide',start_clock_ns:'9007199254740993',end_clock_ns:'9007199254741993',trajectory_epoch_ns:'9007199254741000',sha256:'a'.repeat(64),data:Buffer.from(JSON.stringify({traceEvents:events})).toString('base64')};
  const trials=[{id:'failed',runtime_configuration:'v8-tier-traced',workload:'core',status:'error',engine_trace:trace},{id:'missing',runtime_configuration:'v8-tier-traced',workload:'core',status:'ok',engine_trace:{...trace,status:'unavailable',reason:'start failed',data:undefined}}];
  const root=new Element('div');context.engineTraceEvidence(root,trials);
  const selects=descendants(root,n=>n.tag==='select');
  assert.equal(selects[1].children.length,3);
  assert.ok(descendants(root,n=>n.textContent.includes('retained prefix')).length);
  assert.ok(descendants(root,n=>n.textContent.includes('9007199254740993')).length);
  assert.equal(descendants(root,n=>n.tag==='tr').length,501);
  assert.ok(descendants(root,n=>n.tag==='a'&&n.href==='traces/'+trace.sha256+'.trace.json').length);
  selects[1].select(1000);assert.equal(descendants(root,n=>n.tag==='tr').length,2);
  assert.ok(descendants(root,n=>n.textContent==='1000 / event-1000').length);
  assert.ok(descendants(root,n=>n.tag==='td'&&n.textContent==='0').length);
  selects[0].select(1);assert.equal(descendants(root,n=>n.tag==='tr').length,0);
  assert.ok(descendants(root,n=>n.textContent==='No native events available.').length);
  assert.equal(descendants(root,n=>n.tag==='a'&&n.href?.startsWith('traces/')).length,0);
  context.engineTraceEvidence(root,[{profile:'profiling',status:'unsupported'}]);
  assert.ok(descendants(root,n=>n.textContent.includes('typed export retains profiling trial outcomes')).length);
  assert.ok(html.includes('href="engine-events.parquet" download'));
});

function engineTimelineRenderer(){
  const context=vm.createContext({node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},TextDecoder,Uint8Array,atob,document:{createElementNS:(_,tag)=>new Element(tag)}});
  vm.runInContext(html.slice(html.indexOf('function engineTimelineModel('),html.indexOf('function timingSamples(')),context);return context;
}
function engineTimelineFixture(){
  const sample={index:0,warmup:true,verified:false,elapsed_ns:10,result:[],tier_window:{invocation:1,invocation_outcome:'guest_trap',failure_reason:'unreachable',before:{start_ns:0,end_ns:2,state:'uncompiled'},operation_start_ns:3,operation_end_ns:13,after:{start_ns:14,end_ns:15,state:'unavailable',reason:'non-atomic mismatch'}}};
  const events=[{pid:1,tid:2,ts:9007199254740,cat:'v8.wasm',name:'outer',ph:'X',dur:3},{pid:1,tid:2,ts:9007199254740,cat:'v8.wasm',name:'inner',ph:'X',dur:1},{pid:1,tid:3,ts:9007199254741,cat:'v8.wasm',name:'wasm.TopTierCompilation',ph:'X',dur:0},{pid:1,tid:3,ts:9007199254741,cat:'v8.wasm',name:'unpaired-begin',ph:'B',dur:99},{pid:1,tid:2,ts:0,cat:'__metadata',name:'thread_name',ph:'M',args:{name:'JavaScriptMainThread'}}];
  const trial={id:'failed',status:'error',samples:[sample],engine_trace:{status:'incomplete',reason:'prefix',trajectory_epoch_ns:'9007199254740993',start_clock_ns:'9007199254740000',end_clock_ns:'9007199254750000'}};
  return {trial,events};
}
test('shared timeline subtracts exact origins, keeps overlaps separate and does not infer thread roles or phases',()=>{
  const context=engineTimelineRenderer(),{trial,events}=engineTimelineFixture(),before=JSON.stringify({trial,events});
  const model=context.engineTimelineModel(trial,events);
  assert.equal(model.origin,'9007199254740993');assert.equal(model.originLabel,'trajectory epoch');
  const native=model.marks.filter(m=>m.kind==='native');
  assert.equal(native[0].start,-993n);assert.equal(native[0].end,2007n);
  assert.equal(native[1].end,7n);assert.notEqual(native[0].lane,native[1].lane,'overlap is not hidden or summed');
  assert.equal(native[2].start,7n);assert.equal(native[2].end,7n);assert.equal(native[2].interval,true,'measured zero interval retained');
  assert.equal(native[3].interval,false);assert.equal(native[3].end,native[3].start,'do not invent B/E pairing or use unrelated duration');
  assert.ok(model.lanes[native[0].lane].includes('JavaScriptMainThread'));
  assert.ok(model.lanes[native[2].lane].includes('thread role unreported'));
  assert.equal(model.metadataOmitted,1);assert.equal(model.visibleCalls,1);assert.equal(model.marks.length,7);
  const call=model.marks.find(m=>m.kind==='call');assert.equal(call.start,3n);assert.equal(call.end,13n);assert.equal(call.failed,true);assert.match(call.detail,/Returned bits: None/);
  assert.equal(JSON.stringify({trial,events}),before);
  events.push({...events[4],args:{name:'other-name'}});
  assert.ok(context.engineTimelineModel(trial,events).lanes.some(l=>l.includes('conflicting thread metadata')));
});
test('shared timeline is windowed, native-only when needed, and refuses an absent or imprecise clock bridge',()=>{
  const context=engineTimelineRenderer(),{trial,events}=engineTimelineFixture();
  trial.samples=Array.from({length:1001},(_,i)=>({...trial.samples[0],index:i,tier_window:{...trial.samples[0].tier_window,invocation:i+1}}));
  let model=context.engineTimelineModel(trial,events,1000);assert.equal(model.totalCalls,1001);assert.equal(model.visibleCalls,1);assert.match(model.marks[0].title,/Invocation 1001/);
  delete trial.engine_trace.trajectory_epoch_ns;
  assert.equal(context.engineTimelineModel(trial,events).status,'unavailable');
  trial.samples=[];model=context.engineTimelineModel(trial,events);assert.equal(model.originLabel,'collection start');assert.equal(model.marks[0].start,0n);
  trial.engine_trace.status='unavailable';assert.equal(context.engineTimelineModel(trial,events).status,'unavailable');
  const fixture=engineTimelineFixture();fixture.trial.samples[0].tier_window.operation_start_ns=Number.MAX_SAFE_INTEGER+1;
  assert.equal(context.engineTimelineModel(fixture.trial,fixture.events).status,'unavailable');
});
test('shared timeline shows exact click/keyboard details, failed outlines, independent snapshots and point markers',()=>{
  const context=engineTimelineRenderer(),{trial,events}=engineTimelineFixture(),root=new Element('div');context.engineTimeline(root,trial,events);
  const markers=descendants(root,n=>n.attributes.role==='button');assert.equal(markers.length,7);
  assert.equal(descendants(root,n=>n.tag==='svg')[0].style.height,'auto','global chart height must not squash thread lanes');
  assert.equal(markers[0].attributes.stroke,'#ed7878');assert.equal(markers[0].attributes.fill,'#edb85f');
  markers[0].listeners.click();const detail=descendants(root,n=>n.tag==='pre')[0];assert.match(detail.textContent,/Call bracket: \[3, 13\] ns/);assert.match(detail.textContent,/guest_trap/);
  let prevented=false;markers[4].listeners.keydown({key:'Enter',preventDefault:()=>{prevented=true;}});assert.equal(prevented,true);assert.match(detail.textContent,/Relative bracket: \[-993, 7\] ns/);assert.match(detail.textContent,/No workload module\/function attribution/);
  assert.equal(markers[6].tag,'circle');assert.ok(descendants(root,n=>n.textContent.includes('Subpixel intervals use a minimum-width marker')).length);
  assert.equal(descendants(root,n=>n.tag==='polyline').length,0,'never connect snapshots or sum intervals');
});
test('shared timeline call-window controls reach later calls and reset independently with trials',()=>{
  const context=engineTimelineRenderer(),{trial,events}=engineTimelineFixture();
  trial.samples=Array.from({length:1001},(_,i)=>({...trial.samples[0],index:i,tier_window:{...trial.samples[0].tier_window,invocation:i+1}}));
  trial.engine_trace.data=Buffer.from(JSON.stringify({traceEvents:events})).toString('base64');
  const short={...trial,id:'short',samples:trial.samples.slice(0,2)},root=new Element('div'),before=JSON.stringify([trial,short]);
  context.engineTraceEvidence(root,[trial,short]);const selects=descendants(root,n=>n.tag==='select');
  assert.equal(selects[2].children.length,3);selects[2].select(1000);
  const markers=descendants(root,n=>n.attributes.role==='button');assert.ok(markers.some(m=>m.attributes['aria-label'].startsWith('Invocation 1001')));
  assert.ok(!markers.some(m=>m.attributes['aria-label'].startsWith('Invocation 1 ·')));
  selects[0].select(1);assert.equal(selects[2].value,'0');assert.equal(selects[2].children.length,1);
  assert.ok(descendants(root,n=>n.attributes.role==='button').some(m=>m.attributes['aria-label'].startsWith('Invocation 1 ·')));
  assert.equal(JSON.stringify([trial,short]),before);
});

test('tier diagnostics retain all windows, unavailable brackets, warmup and independent raw trials',()=>{
 const context=vm.createContext({node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},fmt:n=>`${n} ns`,document:{createElementNS:(_,tag)=>new Element(tag)}});
 vm.runInContext(html.slice(html.indexOf('function tierTrajectory('),html.indexOf('function timingSamples(')),context);
 const samples=Array.from({length:1001},(_,i)=>({index:i,warmup:i<2,elapsed_ns:10,verified:true,result:['7'],tier_window:{version:1,invocation:i+1,before:{state:'uncompiled',start_ns:i*20,end_ns:i*20+2},operation_start_ns:i*20+3,operation_end_ns:i*20+13,after:{state:'unavailable',start_ns:i*20+14,end_ns:i*20+15,reason:'queries disagreed'},collector:'V8/testing-code-tier-intrinsics',collector_version:'test',scope:'exported_entry_code_nonatomic_boundary_snapshots',quality:'engine_reported'}}));
 const traces=[{id:'long/id',runtime_configuration:'v8-tier-observed',workload:'w',block:0,profile:'profiling',scenario:'trajectory',status:'ok',samples},{id:'short',block:1,profile:'profiling',scenario:'trajectory',status:'error',reason:'stopped',samples:samples.slice(0,2)},{id:'admission',block:-1,profile:'profiling',scenario:'first-call',status:'ok',samples}];
 const before=JSON.stringify(traces),root=new Element('div');context.tierTrajectory(root,traces);
 const launch=byID(root,'tier-trial'),window=byID(root,'tier-window');assert.equal(launch.children.length,2);assert.equal(window.children.length,3);
 assert.equal(descendants(root,n=>n.tag==='tr').length,501);assert.ok(descendants(root,n=>n.textContent==='Yes').length===2);
 assert.equal(dots(root).length,1000,'each invocation retains independent before and after snapshots');assert.match(titles(root)[0],/Invocation 1 · before.*warmup/);assert.equal(dots(root)[1].attributes.fill,'#88949b','unavailable is not a known tier');
 assert.ok(descendants(root,n=>n.textContent==='unavailable [14, 15] ns · queries disagreed').length);assert.equal(descendants(root,n=>n.tag==='a')[0].href,'raw/trials/long%2Fid.json');
 window.select('1000');assert.equal(descendants(root,n=>n.tag==='tr').length,2);assert.ok(descendants(root,n=>n.textContent==='1001 / 1000').length);
 launch.select('1');assert.equal(window.value,'0');assert.equal(window.children.length,1);assert.equal(descendants(root,n=>n.tag==='a')[0].href,'raw/trials/short.json');assert.ok(descendants(root,n=>n.textContent.startsWith('Outcome: error · stopped')).length);
 assert.equal(JSON.stringify(traces),before,'rendering must not alter evidence');
 const failed=structuredClone(traces[1]);failed.samples[1].verified=false;delete failed.samples[1].result;failed.samples[1].tier_window.version=2;failed.samples[1].tier_window.invocation_outcome='guest_trap';failed.samples[1].tier_window.failure_reason='unreachable';
 context.tierTrajectory(root,[failed]);assert.ok(descendants(root,n=>n.textContent==='Not verified · guest_trap · unreachable · Returned bits: None').length);assert.equal(dots(root)[2].attributes.stroke,'#ed7878');assert.match(titles(root)[2],/Not verified · guest_trap · unreachable/);assert.ok(descendants(root,n=>n.textContent.includes('Failed trials and prefixes are diagnostic only')).length);
 context.tierTrajectory(root,[]);assert.match(root.children[0].textContent,/No exported-entry tier observations/);
});

function paretoRenderer(){
  const elements=new Map(['pareto-plot','pareto-table','pareto-detail','chart-workload','pareto-stage','pareto-coverage'].map(id=>[id,new Element('div')]));
  elements.get('chart-workload').value='w';elements.get('pareto-stage').value='compile';
  const data={memory_source:{id:'memory'},summaries:[],pareto:[]},manifest={lock:{runtime_configurations:[{id:'fast'},{id:'small'},{id:'missing'}]}};
  const runtimeInputs=new Map(manifest.lock.runtime_configurations.map(c=>[c.id,{input:{checked:true}}]));
  const context=vm.createContext({data,manifest,runtimeInputs,runtimeColors:['blue','green','gray'],el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},document:{createElementNS:(_,tag)=>new Element(tag)},fmt:n=>`${n} ns`,fmtBytes:n=>`${n} B`,openCellEvidence:()=>{}});
  vm.runInContext(html.slice(html.indexOf('function selectedParetoPoints('),html.indexOf('function phaseCPURows(')),context);
  return {context,elements,data,runtimeInputs};
}

test('Pareto filters recompute median frontier, keep zeros and ties, and never mutate evidence',()=>{
  const {context}=paretoRenderer();
  const point=(runtime,time_ns,rss_bytes)=>({runtime,time_ns,rss_bytes,status:'available'});
  const points=[point('fast',0,20),point('tie',0,20),point('small',20,0),point('dominated',20,20),{runtime:'missing',status:'unavailable',time_ns:0,rss_bytes:null},point('invalid',-1,0)];
  const before=JSON.stringify(points);
  let selected=Array.from(context.selectedParetoPoints(points,points.map(p=>p.runtime)));
  assert.deepEqual(selected.filter(p=>p.frontier).map(p=>p.runtime),['fast','tie','small']);
  assert.deepEqual(Array.from(selected[3].dominated_by),['fast','small','tie']);
  assert.equal(selected[4].frontier,false);assert.equal(selected[5].status,'unavailable');
  selected=Array.from(context.selectedParetoPoints(points,['dominated','missing']));assert.equal(selected[0].frontier,true,'frontier belongs to selected configurations');
  assert.equal(JSON.stringify(points),before);
});

test('Pareto renderer retains coverage, independent intervals, keyboard/raw evidence, and clears filtered details',()=>{
  const {context,elements,data,runtimeInputs}=paretoRenderer();
  data.pareto=[{runtime:'fast',workload:'w',scenario:'compile',status:'available',reason:'Separate runs',time_ns:0,rss_bytes:20,time_launches:3,memory_launches:3,time_ci95_low_ns:0,time_ci95_high_ns:1,rss_ci95_low_bytes:10,rss_ci95_high_bytes:30,memory_trials:['same/id']},{runtime:'small',workload:'w',scenario:'compile',status:'available',time_ns:20,rss_bytes:0,time_launches:1,memory_launches:1,memory_trials:[]},{runtime:'missing',workload:'w',scenario:'compile',status:'unavailable',reason:'Missing RSS',time_ns:5,rss_bytes:null,time_launches:1,memory_launches:0,memory_trials:[]}];
  context.renderPareto();
  const circles=descendants(elements.get('pareto-plot'),n=>n.tag==='circle');assert.equal(circles.length,2);
  assert.equal(descendants(elements.get('pareto-plot'),n=>n.tag==='svg')[0].attributes.role,'group','interactive child evidence buttons must not be hidden by an image role');
  assert.match(elements.get('pareto-coverage').textContent,/2 \/ 3 configurations/);
  assert.equal(descendants(elements.get('pareto-table'),n=>n.tag==='tr').length,4,'missing runtime remains in coverage table');
  assert.equal(circles[0].attributes.tabindex,'0');
  let prevented=false;circles[0].listeners.keydown({key:'Enter',preventDefault(){prevented=true;}});assert.equal(prevented,true);
  assert.equal(elements.get('pareto-detail').hidden,false);
  const links=descendants(elements.get('pareto-detail'),n=>n.tag==='a');assert.equal(links[0].href,'raw-memory/trials/same%2Fid.json');
  assert.ok(descendants(elements.get('pareto-detail'),n=>/Timing 95% interval: 0 ns – 1 ns/.test(n.textContent)).length);
  runtimeInputs.get('fast').input.checked=false;runtimeInputs.get('small').input.checked=false;context.renderPareto();
  assert.equal(elements.get('pareto-detail').hidden,true);assert.equal(elements.get('pareto-detail').children.length,0);assert.equal(descendants(elements.get('pareto-plot'),n=>n.tag==='circle').length,0);
  assert.match(elements.get('pareto-coverage').textContent,/0 \/ 1 configurations/);
});

test('all windows are reachable; changing launches resets the sample window', () => {
  const ui = renderer(), root = new Element('div');
  const a = trial('long', 1001), b = trial('short', 2, {block: 1});
  a.samples[0].warmup = true;
  const before = JSON.stringify([a, b]);
  ui.timingTrajectory(root, [a, b]);
  const launch = byID(root, 'timing-trial'), window = byID(root, 'timing-window');
  assert.equal(launch.children.length, 2);
  assert.deepEqual(window.children.map(n => n.textContent), ['Verified samples 1–500 of 1001', 'Verified samples 501–1000 of 1001', 'Verified samples 1001–1001 of 1001']);
  assert.equal(dots(root).length, 500);
  assert.equal(dots(root)[0].attributes.fill, '#edb85f');
  assert.match(titles(root)[0], /^Sample 0 .*warmup$/);
  window.select('500');
  assert.equal(dots(root).length, 500);
  assert.match(titles(root)[0], /^Sample 500 /);
  assert.match(titles(root).at(-1), /^Sample 999 /);
  window.select('1000');
  assert.equal(dots(root).length, 1);
  assert.match(titles(root)[0], /^Sample 1000 /);
  launch.select('1');
  assert.equal(window.children.length, 1);
  assert.equal(window.value, '0');
  assert.equal(dots(root).length, 2);
  assert.match(titles(root)[0], /^Sample 0 /);
  launch.select('0');
  assert.equal(window.value, '0');
  assert.equal(dots(root).length, 500);
  assert.equal(JSON.stringify([a, b]), before, 'raw samples must not change');
});

test('invalid samples remain gaps and failed or diagnostic launches never enter the selector', () => {
  const ui = renderer(), root = new Element('div');
  const valid = trial('valid', 0, {samples: [sample(0), sample(1, {verified: false}), sample(2), sample(10), sample(-1), sample(11, {elapsed_ns: -1}), sample(12, {operations: 0}), sample(13, {elapsed_ns: Number.MAX_SAFE_INTEGER + 1}), sample(14, {operations: 1.5})]});
  ui.timingTrajectory(root, [trial('failed', 2, {status: 'incorrect'}), trial('memory', 2, {profile: 'memory'}), trial('empty', 0), valid]);
  assert.equal(byID(root, 'timing-trial').children.length, 1);
  assert.match(byID(root, 'timing-trial').children[0].textContent, /valid$/);
  assert.equal(dots(root).length, 3);
  assert.deepEqual(dots(root).map(n => n.attributes.cx), ['15', '209', '985']);
  assert.equal(descendants(root, n => n.tag === 'line').length, 0, 'do not connect gaps or launches');
  assert.ok(descendants(root, n => n.tag === 'p').some(n => /6 invalid\/unverified/.test(n.textContent)));
});

test('no eligible launch produces a missing-evidence message, not a chart', () => {
  const root = new Element('div');
  renderer().timingTrajectory(root, [trial('failed', 3, {status: 'timeout'})]);
  assert.equal(descendants(root, n => n.tag === 'svg' || n.tag === 'select').length, 0);
  assert.match(root.children[0].textContent, /No verified timing samples/);
});

test('headline table distinguishes withheld, unavailable, and measured zero; filters clear stale details', () => {
  const ui = renderer(), elements = new Map();
  for (const id of ['detail', 'scenario', 'runtime', 'query', 'boundary', 'rows']) elements.set(id, new Element('div'));
  const el = id => elements.get(id);
  el('scenario').value = 'steady';
  el('runtime').value = '';
  el('query').value = '';
  const summary = (workload, overrides) => ({workload, runtime: 'r', scenario: 'steady', latency_status: 'timing_pass', latency_reason: 'test policy', independent_launches: 1, median_ns_per_operation: 0, ci95_low: null, ci95_high: null, successful_launches: 1, attempted_launches: 1, stability: 'not_established', outcomes: {ok: 1}, ...overrides});
  ui.el = el;
  ui.detail = () => {};
  ui.data = {scenarios: [{id: 'steady', boundary: 'test boundary'}], summaries: [
    summary('zero', {}),
    summary('diagnostic', {latency_status: 'not_timing_pass', median_ns_per_operation: 123}),
    summary('missing', {independent_launches: 0, median_ns_per_operation: null, successful_launches: 0, outcomes: {unsupported: 1}}),
  ]};
  vm.runInContext(html.slice(html.indexOf('function headlineEstimate('), start), ui);
  ui.draw();
  assert.deepEqual(el('rows').children.map(row => row.children[1].textContent), ['0 ns', 'Withheld', 'Unavailable']);
  assert.equal(descendants(el('rows'), n => n.className === 'bar').length, 1);
  assert.equal(el('rows').children[1].children[3].textContent, '1 / 1', 'diagnostic success remains visible');
  el('detail').append(new Element('p')); el('detail').style.display = 'block';
  el('query').value = 'zero';
  ui.draw();
  assert.equal(el('detail').style.display, 'none');
  assert.equal(el('detail').children.length, 0);
  assert.equal(el('rows').children.length, 1);
  el('runtime').value = 'other';
  ui.draw();
  assert.equal(el('rows').children.length, 0);
});

test('overview ranks only eligible timing estimates within the selected cell and retains missing configurations', () => {
  const context = vm.createContext({});
  vm.runInContext(html.slice(html.indexOf('function headlineEstimate('), html.indexOf('function draw(')), context);
  vm.runInContext(html.slice(html.indexOf('function overviewEntries('), html.indexOf('const runtimeColors=')), context);
  const summaries = [
    {workload:'a',scenario:'steady',runtime:'slow',latency_status:'timing_pass',independent_launches:3,median_ns_per_operation:20},
    {workload:'a',scenario:'steady',runtime:'fast',latency_status:'timing_pass',independent_launches:3,median_ns_per_operation:10},
    {workload:'a',scenario:'steady',runtime:'diagnostic',latency_status:'not_timing_pass',independent_launches:3,median_ns_per_operation:1},
    {workload:'b',scenario:'steady',runtime:'slow',latency_status:'timing_pass',independent_launches:3,median_ns_per_operation:2},
  ];
  const configurations = ['slow','fast','diagnostic','missing'].map(id=>({id}));
  const rows = context.overviewEntries(summaries,configurations,'a','steady');
  assert.deepEqual(Array.from(rows,r=>r.id),['fast','slow','diagnostic','missing']);
  assert.deepEqual(Array.from(rows,r=>r.available),[true,true,false,false]);
});

test('overview groups recorded scenarios into compile, instantiate, and execution without dropping extras', () => {
  const context = vm.createContext({});
  vm.runInContext(html.slice(html.indexOf('function overviewStages('), html.indexOf('const runtimeColors=')), context);
  const stages = Array.from(context.overviewStages(['steady', 'teardown', 'compile', 'first-call', 'instantiate']), s => [s.id, s.group, s.tone]);
  assert.deepEqual(stages, [
    ['compile', 'Compile', 'compile'],
    ['instantiate', 'Instantiate', 'instantiate'],
    ['first-call', 'Execution', 'first-call'],
    ['steady', 'Execution', 'execution'],
    ['teardown', 'Other stages', 'other'],
  ]);
});

test('checkpoint stages have explicit lifecycle labels and distinct stage tones', () => {
  const context=vm.createContext({});
  vm.runInContext(html.slice(html.indexOf('function overviewStages('),html.indexOf('const runtimeColors=')),context);
  const stages=Array.from(context.overviewStages(['checkpoint-execute','checkpoint-first-write','checkpoint-restore','checkpoint-create']),s=>[s.id,s.label,s.tone]);
  assert.deepEqual(stages,[['checkpoint-create','Checkpoint creation','compile'],['checkpoint-restore','Checkpoint restore','instantiate'],['checkpoint-first-write','First write','first-call'],['checkpoint-execute','Post-restore execution','execution']]);
});

test('native continuation stages distinguish guest resumption from API-return restoration', () => {
  assert.match(html,/stage.group==='Guest checkpoints'\|\|stage.group==='Native execution-stack continuations'/);
  const context=vm.createContext({});
  vm.runInContext(html.slice(html.indexOf('function overviewStages('),html.indexOf('const runtimeColors=')),context);
  const stages=Array.from(context.overviewStages(['continuation-execute','continuation-resume','continuation-create','continuation-first-write']),s=>[s.id,s.label,s.group,s.tone]);
  assert.deepEqual(stages,[
    ['continuation-create','Stack capture','Native execution-stack continuations','compile'],
    ['continuation-resume','Guest resumption','Native execution-stack continuations','instantiate'],
    ['continuation-first-write','First write','Native execution-stack continuations','first-call'],
    ['continuation-execute','Post-restore execution','Native execution-stack continuations','execution'],
  ]);
});

test('runtime comparison graph starts full-width with matching accessible state', () => {
  assert.match(html, /<main class="chart-expanded">/);
  assert.match(html, /id="chart-expand"[^>]*aria-expanded="true"[^>]*>Compact graph<\/button>/);
});

test('publication notice distinguishes recorded eligibility from independent operator trust',()=>{
  for(const publication of [null,{version:'future',audit:{status:'eligible'}},{version:'qualified-publication-archive-v1',audit:{status:'blocked'}},{version:'qualified-publication-archive-v1',audit:{status:'eligible'}}]) {
    const board=new Element('p'),notice=new Element('p'),reproduce=new Element('pre');
    board.textContent=notice.textContent='Exploratory';reproduce.textContent='Reproduce';
    const context=vm.createContext({data:{publication},document:{querySelector:s=>s==='.board-note'?board:notice},el:()=>reproduce,node:(tag,text)=>{const n=new Element(tag);n.textContent=text;return n;}});
    vm.runInContext(html.slice(html.indexOf('function renderPublicationNotice('),html.indexOf('function headlineEstimate(')),context);
    const eligible=publication?.version==='qualified-publication-archive-v1'&&publication.audit.status==='eligible';
    if(!eligible){assert.equal(board.textContent,'Exploratory');assert.equal(reproduce.textContent,'Reproduce');continue;}
    assert.match(notice.textContent,/signature authenticates assertions, not physical host truth/);
    assert.match(reproduce.textContent,/--qualification-key \/trusted\/operator-public.json/);
    assert.match(reproduce.textContent,/new qualification/);
    assert.deepEqual(board.children.filter(n=>n.tag==='a').map(n=>n.href),['publication-audit.json','qualification.json','raw-pilot/manifest.json']);
  }
});

test('expanded graph toggles layout and accessible state without changing data', () => {
  const button = new Element('button');
  button.getAttribute = key => button.attributes[key];
  button.setAttribute('aria-expanded', 'false');
  const classes = new Set(), data = {selectedStages: ['compile', 'instantiate', 'execution']};
  const before = JSON.stringify(data);
  const context = vm.createContext({data, el: () => button, document: {
    querySelector: selector => {assert.equal(selector, 'main'); return {classList: {
      toggle: (name, enabled) => enabled ? classes.add(name) : classes.delete(name),
    }};},
  }});
  vm.runInContext(html.slice(html.indexOf('function toggleHeadlineExpansion('), html.indexOf('function headlineTimeTooltip(')), context);
  button.listeners.click();
  assert.equal(button.attributes['aria-expanded'], 'true');
  assert.equal(button.textContent, 'Compact graph');
  assert.ok(classes.has('chart-expanded'));
  button.listeners.click();
  assert.equal(button.attributes['aria-expanded'], 'false');
  assert.equal(button.textContent, 'Expand graph');
  assert.equal(classes.size, 0);
  assert.equal(JSON.stringify(data), before);
});

test('large graph follows selected timing stages and labels the peak RSS scenario', () => {
  const context = vm.createContext({});
  vm.runInContext(html.slice(html.indexOf('function headlineGraphSpecs('), html.indexOf('function renderHeadlineGraph(')), context);
  const stage = (id, label, tone) => ({id, label, tone});
  const selected = [stage('compile', 'Compile', 'compile'), stage('instantiate', 'Instantiate', 'instantiate'), stage('steady', 'Steady', 'execution')];
  let specs = Array.from(context.headlineGraphSpecs(selected), s => [s.id, s.kind, s.label]);
  assert.deepEqual(specs, [
    ['lifecycle', 'time', 'Latency · selected stages'],
    ['compile', 'rss', 'Peak RSS · Compile'],
    ['instantiate', 'rss', 'Peak RSS · Instantiate'],
    ['steady', 'rss', 'Peak RSS · Steady'],
  ]);
  specs = Array.from(context.headlineGraphSpecs([selected[0]]), s => [s.id, s.kind]);
  assert.deepEqual(specs, [['lifecycle', 'time'], ['compile', 'rss']]);
  assert.equal(context.headlineGraphSpecs([]).length, 0);
});

test('large graph time hover labels matched stage RSS without inventing missing memory', () => {
  const context = vm.createContext({fmt: value => `${value} ns`, fmtBytes: value => `${value} B`});
  vm.runInContext(html.slice(html.indexOf('function headlineTimeTooltip('), html.indexOf('function headlineGraphSpecs(')), context);
  assert.match(context.headlineTimeTooltip('wago', 'Compile', 12, {median_bytes: 4096}), /Time: 12 ns \/ op\nPeak RSS: 4096 B/);
  assert.match(context.headlineTimeTooltip('wago', 'Instantiate', 3, null), /Peak RSS: unavailable/);
});

test('memory hover defaults to peak RSS and falls back only to recorded metrics',()=>{
  const select=new Element('select');
  const context=vm.createContext({el:()=>select,memoryMetricInfo:{'host.alloc.bytes':[],'process.peak_rss':[]},memoryStages:[{metric:'host.alloc.bytes'},{metric:'process.peak_rss'}]});
  const assignment=html.slice(html.indexOf("el('memory-metric').value="),html.indexOf("el('memory-metric').disabled="));
  vm.runInContext(assignment,context);assert.equal(select.value,'process.peak_rss');
  context.memoryStages=[{metric:'host.alloc.bytes'}];vm.runInContext(assignment,context);assert.equal(select.value,'host.alloc.bytes');
  context.memoryStages=[];vm.runInContext(assignment,context);assert.equal(select.value,'process.peak_rss');
});

test('large lifecycle graph shares timing and RSS scales and clicks the exact stage evidence', () => {
  const elements = new Map(['headline-chart-grid','headline-memory-detail','chart-workload','headline-chart-note'].map(id=>[id,new Element('div')]));
  elements.get('chart-workload').value='w';
  elements.get('headline-chart-grid').style.setProperty=()=>{};
  let clicked;
  const context=vm.createContext({
    el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},
    fmt:n=>`${n} ns`,fmtBytes:n=>`${n} B`,
    memoryCell:(runtime,workload,stage)=>runtime==='r'?{scenario:stage,median_bytes:stage==='compile'?2000:1000,independent_launches:3}:null,
    openCellEvidence:s=>{clicked=s;},
    data:{summaries:[
      {workload:'w',runtime:'r',scenario:'compile',latency_status:'timing_pass',independent_launches:3,median_ns_per_operation:60},
      {workload:'w',runtime:'r',scenario:'instantiate',latency_status:'timing_pass',independent_launches:3,median_ns_per_operation:40},
    ]},
  });
  vm.runInContext(html.slice(html.indexOf('function headlineEstimate('),html.indexOf('function draw(')),context);
  vm.runInContext(html.slice(html.indexOf('function compositeEntries('),html.indexOf('const runtimeColors=')),context);
  vm.runInContext(html.slice(html.indexOf('function headlineTimeTooltip('),html.indexOf('function headlineGraphSpecs(')),context);
  vm.runInContext(html.slice(html.indexOf('function headlineGraphSpecs('),html.indexOf("el('overview-query').addEventListener")),context);
  context.renderHeadlineGraph([{id:'r'},{id:'missing'}],[{id:'compile',label:'Compile',tone:'compile'},{id:'instantiate',label:'Instantiate',tone:'instantiate'}]);
  const root=elements.get('headline-chart-grid');
  const segments=descendants(root,n=>n.className?.startsWith('composite-segment'));
  assert.deepEqual(segments.map(n=>n.style.width),['60%','40%']);
  const scales=descendants(root,n=>n.className==='headline-scale');
  assert.deepEqual(scales.map(n=>n.children.map(child=>child.textContent)),[['0','100 ns'],['0','2000 B'],['0','2000 B']],'headers expose separate shared time and RSS scales');
  assert.ok(descendants(root,n=>n.textContent==='Incomplete').length);
  assert.equal(descendants(root,n=>n.textContent==='Not recorded').length,2);
  assert.deepEqual(descendants(root,n=>n.className==='headline-chart-fill').slice(0,2).map(n=>n.style.width),['100%','50%'],'RSS scales must be shared across stages, not independently normalized');
  const rssCells=descendants(root,n=>n.className?.startsWith('headline-chart-cell rss'));
  const tooltips=descendants(root,n=>n.className==='bar-tooltip');
  assert.equal(new Set(tooltips.map(n=>n.id)).size,tooltips.length,'tooltip IDs are unique across runtime, stage and metric');
  for(const target of [...segments,...rssCells.filter(n=>n.tag==='button')]){
    const tooltip=descendants(target,n=>n.className==='bar-tooltip')[0];
    assert.equal(target.attributes['aria-describedby'],tooltip.id);
    assert.equal(tooltip.attributes.role,'tooltip','hover details are also described to keyboard and screen-reader users');
  }
  assert.deepEqual(rssCells.slice(0,2).map(n=>n.className),['headline-chart-cell rss compile','headline-chart-cell rss instantiate']);
  assert.equal(rssCells[1].title,'r · Instantiate\nTime: 40 ns / op\nPeak RSS: 1000 B');
  assert.deepEqual(descendants(rssCells[1],n=>n.className==='bar-tooltip')[0].children.map(n=>n.textContent),['r · Instantiate','Time: 40 ns / op','Peak RSS: 1000 B'],'RSS hover uses the same compact details as timing');
  assert.deepEqual(descendants(root,n=>n.className==='headline-stage-label').map(n=>n.textContent),['Compile','Instantiate']);
  const stageValues=descendants(root,n=>n.className==='stage-key headline-stage-value');
  assert.equal(stageValues.length,4,'every selected stage stays readable, including missing data');
  assert.equal(stageValues[0].title,'r · Compile\nTime: 60 ns / op\nPeak RSS: 2000 B');
  assert.equal(stageValues[2].tag,'span','missing timing cannot be clicked as measured evidence');
  stageValues[0].listeners.click();assert.equal(clicked.scenario,'compile');
  segments[1].listeners.click();assert.equal(clicked.scenario,'instantiate');
  context.data.summaries[0].median_ns_per_operation=999;
  context.data.summaries[1].median_ns_per_operation=1;
  context.renderHeadlineGraph([{id:'r'}],[{id:'compile',label:'Compile',tone:'compile'},{id:'instantiate',label:'Instantiate',tone:'instantiate'}]);
  const tiny=descendants(root,n=>n.className==='stage-key headline-stage-value');
  assert.equal(tiny.length,2,'all stages must have separate readable click targets');
  assert.match(tiny[1].attributes['aria-label'],/Instantiate evidence/);
  assert.deepEqual(descendants(root,n=>n.className?.startsWith('composite-segment')).map(n=>n.style.width),['99.9%','0.1%'],'do not inflate quantitative geometry');
  tiny[1].listeners.click();assert.equal(clicked.scenario,'instantiate');
  context.data.summaries.push({workload:'w',runtime:'r',scenario:'first-call',latency_status:'timing_pass',independent_launches:3,median_ns_per_operation:20});
  const threeStages=[{id:'compile',label:'Compile',tone:'compile'},{id:'instantiate',label:'Instantiate',tone:'instantiate'},{id:'first-call',label:'Execution',tone:'first-call'}];
  context.renderHeadlineGraph([{id:'r'}],threeStages);
  const allSegments=descendants(root,n=>n.className?.startsWith('composite-segment'));
  assert.deepEqual(allSegments.map(n=>n.className),['composite-segment compile','composite-segment instantiate','composite-segment first-call']);
  assert.ok(Math.abs(allSegments.reduce((sum,n)=>sum+parseFloat(n.style.width),0)-100)<1e-9,'all three stages share one quantitative bar');
  const executionTooltip=descendants(allSegments[2],n=>n.className==='bar-tooltip')[0];
  assert.deepEqual(executionTooltip.children.map(n=>n.textContent),['r · Execution','Time: 20 ns / op','Peak RSS: 1000 B']);
  allSegments[2].listeners.click();assert.equal(clicked.scenario,'first-call');
  context.renderHeadlineGraph([{id:'r'}],threeStages.slice(1));
  assert.deepEqual(descendants(root,n=>n.className?.startsWith('composite-segment')).map(n=>n.className),['composite-segment instantiate','composite-segment first-call'],'stage selection removes exactly the deselected stage');
  assert.equal(descendants(root,n=>n.className?.startsWith('headline-chart-cell rss')).length,2,'RSS follows the same selected stages');
  context.renderHeadlineGraph([{id:'missing'}],[{id:'compile',label:'Compile',tone:'compile'}]);
  assert.deepEqual(descendants(root,n=>n.className==='headline-scale').map(n=>n.children.map(child=>child.textContent)),[['0','No measurements'],['0','No measurements']],'missing data must not produce a zero-valued scale');
  assert.equal(descendants(root,n=>n.className==='headline-chart-fill').length,0,'unavailable RSS must not render a default full-width colored fill');
});

test('peak RSS click evidence keeps every launch and withholds short intervals', () => {
  const root = new Element('div');
  const context = vm.createContext({
    el: id => id === 'headline-memory-detail' ? root : undefined,
    node: (tag, value) => { const element = new Element(tag); element.textContent = value ?? ''; return element; },
    fmtBytes: value => `${value} B`,
    memoryMetricInfo: {'process.peak_rss': ['Peak process RSS', 'Whole-process maximum.']},
  });
  vm.runInContext(html.slice(html.indexOf('function openMemoryEvidence('), html.indexOf('function renderHeadlineGraph(')), context);
  context.openMemoryEvidence({scenario:'steady',median_bytes:20,independent_launches:2,ci95_low_bytes:null,ci95_high_bytes:null,launch_values:[{trial_id:'one',bytes:10},{trial_id:'two',bytes:30}]},'r','w');
  assert.equal(root.hidden, false);
  assert.equal(descendants(root, n => n.tag === 'a').length, 2);
  assert.deepEqual(descendants(root, n => n.tag === 'a').map(n => n.href), ['raw-memory/trials/one.json', 'raw-memory/trials/two.json']);
  assert.ok(descendants(root, n => n.tag === 'p').some(n => n.textContent.includes('fewer than three')));
  descendants(root, n => n.tag === 'button')[0].listeners.click();
  assert.equal(root.hidden, true);
  assert.equal(root.children.length, 0);
});

test('paired memory scaling links exact raw memory launches, including failures', () => {
  const root=new Element('div');
  const context=vm.createContext({
    el:()=>root,node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},
    data:{memory_source:{id:'m'},memory_scaling_trials:[
      {id:'a',runtime:'r',workload:'w',scenario:'guest-density',status:'ok'},
      {id:'b',runtime:'r',workload:'w',scenario:'guest-density',status:'unsupported'},
      {id:'other',runtime:'different',workload:'w',scenario:'guest-density',status:'ok'},
    ],bundle:{trials:[]},summaries:[]},
    detail:()=>{throw new Error('memory click must not substitute timing evidence');},
  });
  vm.runInContext(html.slice(html.indexOf('function openScalingEvidence('),html.indexOf('function drawMarginal(')),context);
  context.openScalingEvidence({profile:'memory',runtime:'r',scenario:'guest-density',measurement:{metric:'host.alloc.bytes',phase:'provision',scope:'go_heap',collector:'go'}},{workload:'w'});
  assert.deepEqual(descendants(root,n=>n.tag==='a').map(n=>n.href),['raw-memory/trials/a.json','raw-memory/trials/b.json']);
  assert.ok(descendants(root,n=>n.textContent==='b · unsupported').length);
});

test('scaling filters retain global curve indices and clear stale empty selections', () => {
  const elements=new Map(['scaling-choice','scaling-profile','scaling-runtime','scaling-metric','scaling-coverage','scaling-evidence'].map(id=>[id,new Element('select')]));
  let draws=0;
  const curves=[
    {profile:'timing',runtime:'r',measurement:{metric:'time.wall'},points:[{median:0}]},
    {profile:'memory',runtime:'r',measurement:{metric:'host.heap.end'},points:[{median:null}]},
    {profile:'memory',runtime:'other',measurement:{metric:'host.heap.end'},points:[{median:50}]},
  ];
  const context=vm.createContext({curves,el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},drawScaling:()=>draws++,drawMarginal:()=>draws++});
  vm.runInContext(html.slice(html.indexOf('function matchingScalingCurves('),html.indexOf('if(curves.length)')),context);
  elements.get('scaling-profile').value='memory';elements.get('scaling-runtime').value='r';elements.get('scaling-metric').value='host.heap.end';
  context.selectScalingCurves();
  assert.deepEqual(elements.get('scaling-choice').children.map(n=>n.value),['1']);
  assert.match(elements.get('scaling-coverage').textContent,/1 \/ 3 curves · 0 with numeric/);
  elements.get('scaling-evidence').append(new Element('a'));
  elements.get('scaling-runtime').value='missing';context.selectScalingCurves();
  assert.equal(elements.get('scaling-choice').disabled,true);assert.equal(elements.get('scaling-choice').value,'');
  assert.equal(elements.get('scaling-evidence').children.length,0);
  elements.get('scaling-profile').value='timing';elements.get('scaling-runtime').value='r';elements.get('scaling-metric').value='time.wall';context.selectScalingCurves();
  assert.match(elements.get('scaling-coverage').textContent,/1 with numeric/,'measured zero remains measured');
  assert.equal(draws,6);
});

test('empty scaling selections clear both real chart renderers instead of drawing curve zero', () => {
  const elements=new Map(['scaling-choice','scaling-chart','scaling-marginal'].map(id=>[id,new Element('div')]));
  elements.get('scaling-chart').append(new Element('svg'));elements.get('scaling-marginal').append(new Element('svg'));
  const context=vm.createContext({curves:[{}],el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;}});
  vm.runInContext(html.slice(html.indexOf('function drawScaling('),html.indexOf('function openScalingEvidence(')),context);
  vm.runInContext(html.slice(html.indexOf('function drawMarginal('),html.indexOf('const memoryTimelines=')),context);
  context.drawScaling();context.drawMarginal();
  assert.match(elements.get('scaling-chart').children[0].textContent,/No measurement curves/);
  assert.equal(elements.get('scaling-marginal').children.length,0);
  assert.equal(descendants(elements.get('scaling-chart'),n=>n.tag==='svg').length,0);
});

test('paired memory timelines use source-qualified IDs and correct raw links', () => {
  const elements=new Map(['memory-exports','memory-trial','memory-measurement','memory-window'].map(id=>[id,new Element('select')]));
  const measurement={metric:'host.heap.end',phase:'end',scope:'heap',collector:'go',quality:'engine_reported'};
  const timeline={trial:'same/id',measurement,points:[{value:0}]};
  const context=vm.createContext({data:{memory_timelines:[timeline],paired_memory_timelines:[timeline],memory_evidence_version:'v1'},el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;}});
  vm.runInContext(html.slice(html.indexOf('const memoryTimelines='),html.indexOf('const cpuStacks=')),context);
  assert.notEqual(context.memoryTimelineTrialKey({...timeline,evidence_source:'raw'}),context.memoryTimelineTrialKey({...timeline,evidence_source:'raw-memory'}));
  assert.equal(context.memoryTimelineRawLink({...timeline,evidence_source:'raw-memory'}),'raw-memory/trials/same%2Fid.json');
  assert.equal(context.memoryTimelineRawLink({...timeline,evidence_source:'raw'}),'raw/trials/same%2Fid.json');
  let redraw=0;context.selectMemoryMeasurement=()=>redraw++;
  vm.runInContext(html.slice(html.indexOf('function selectMemoryTrial('),html.indexOf('function selectMemoryMeasurement(')),context);
  elements.get('memory-trial').value=context.memoryTimelineTrialKey({...timeline,evidence_source:'raw-memory'});
  context.selectMemoryTrial();assert.deepEqual(elements.get('memory-measurement').children.map(n=>n.value),['1']);
  assert.equal(redraw,1,'never combine colliding primary and paired trials');
});

test('phase CPU rows group metric domains without turning zero into missing', () => {
  const context = vm.createContext({});
  vm.runInContext(html.slice(html.indexOf('function phaseCPURows('), html.indexOf('const phaseCPU=phaseCPURows(')), context);
  const rows = context.phaseCPURows([
    {workload:'w',runtime:'r',scenario:'compile',metric:'time.cpu.user',median_ns:0},
    {workload:'w',runtime:'r',scenario:'compile',metric:'time.cpu.total',median_ns:20},
    {workload:'w',runtime:'r',scenario:'compile',metric:'time.cpu.system',median_ns:null},
    {workload:'w',runtime:'other',scenario:'compile',metric:'time.cpu.total',median_ns:10},
  ]);
  assert.equal(rows.length, 2);
  assert.equal(rows[1].metrics['time.cpu.user'].median_ns, 0);
  assert.equal(rows[1].metrics['time.cpu.system'].median_ns, null);
  assert.equal(rows[1].metrics['time.cpu.total'].median_ns, 20);
  assert.equal(context.hasPhaseCPUData(rows), true);
  assert.equal(context.hasPhaseCPUData(context.phaseCPURows([{workload:'w',runtime:'r',scenario:'compile',metric:'time.cpu.total',median_ns:null}])), false);
});

test('sustained renderer uses elapsed clocks, retains gaps and reaches later sample windows', () => {
  const elements=new Map(['sustained-session','sustained-chart','sustained-metric','sustained-window'].map(id=>[id,new Element('div')]));
  elements.get('sustained-session').value='0';elements.get('sustained-window').value='0';elements.get('sustained-metric').value='api_ns_per_operation';
  const point=(sample,start,value)=>({sample,warmup:sample===0,window:{start_ns:start,operation_start_ns:start+100,end_ns:start+200},api_ns_per_operation:value,go_allocated_bytes_per_diagnostic_second:sample===1?null:0});
  const session={trial:'t',profile:'memory',status:'ok',measured_api_ns:100,target_measured_api_ns:50,points:[point(0,0,5),point(1,1e9,6),point(2,1e10,7)]};
  const context=vm.createContext({el:id=>elements.get(id),node:(tag,text)=>{const n=new Element(tag);n.textContent=text??'';return n;},document:{createElementNS:(_,tag)=>new Element(tag)},sustainedSessions:[session],data:{memory_source:{}},fmt:n=>`${n} ns`});
  vm.runInContext(html.slice(html.indexOf('function drawSustained('),html.indexOf('function drawMemoryTimeline(')),context);
  context.drawSustained();let root=elements.get('sustained-chart'),circles=dots(root);
  assert.equal(circles.length,3);assert.ok(Math.abs(Number(circles[1].attributes.cx)-120)<.001,'X must use elapsed clocks, not ordinal spacing');assert.equal(circles[0].attributes.fill,'#edb85f');
  assert.equal(descendants(root,n=>n.tag==='a')[0].href,'raw-memory/trials/t.json');
  assert.ok(descendants(root,n=>n.tag==='p').some(n=>n.textContent.includes('No forced Go GC requested')));
  session.logical_release={start_ns:0,end_ns:1,closed:true,post_collection:{start_ns:2,end_ns:3,policy:'one_forced_go_gc',observations:[{metric:'host.heap.end',status:'available',value:42},{metric:'host.gc.forced_cycles',status:'available',value:1}]}};
  context.drawSustained();assert.ok(descendants(root,n=>n.tag==='p').some(n=>n.textContent.includes('Go heap after collection: 42 bytes')&&n.textContent.includes('Not reclaimed RSS')));
  assert.equal(descendants(root,n=>n.tag==='p').some(n=>n.textContent.includes('No forced Go GC requested')),false,'forced collection cannot be labeled natural release');
  session.logical_release={start_ns:0,end_ns:1,closed:false,policy:'js_references_dropped'};
  session.points.forEach(p=>{p.v8_heap_end_bytes=0;});elements.get('sustained-metric').value='v8_heap_end_bytes';context.drawSustained();assert.equal(dots(root).length,3);assert.ok(descendants(root,n=>n.tag==='p').some(n=>n.textContent.includes('runtime closed: false')&&n.textContent.includes('JS references dropped only')));
  elements.get('sustained-metric').value='go_allocated_bytes_per_diagnostic_second';context.drawSustained();
  assert.equal(dots(root).length,2,'measured zero must stay visible');assert.equal(descendants(root,n=>n.tag==='line').length,0,'missing rate must break the line');
  session.points=Array.from({length:1001},(_,i)=>point(i,i*1e6,i));elements.get('sustained-window').value='1000';context.drawSustained();assert.equal(dots(root).length,1);assert.match(titles(root)[0],/Sample 1000/);
  session.points=[];session.status='unsupported';context.drawSustained();assert.equal(dots(root).length,0);assert.ok(descendants(root,n=>n.tag==='p').some(n=>/Unsupported and failed/.test(n.textContent)));
});

test('composite bars rank only configurations with every selected stage eligible', () => {
  const context = vm.createContext({});
  vm.runInContext(html.slice(html.indexOf('function headlineEstimate('), html.indexOf('function draw(')), context);
  vm.runInContext(html.slice(html.indexOf('function overviewEntries('), html.indexOf('const runtimeColors=')), context);
  const summary = (runtime,scenario,median,overrides={}) => ({runtime,scenario,workload:'w',latency_status:'timing_pass',independent_launches:3,median_ns_per_operation:median,...overrides});
  const stages = [{id:'compile'},{id:'instantiate'},{id:'first-call'}];
  const rows = context.compositeEntries([
    summary('a','compile',10),summary('a','instantiate',20),summary('a','first-call',30),
    summary('b','compile',2),summary('b','instantiate',3),summary('b','first-call',4),
    summary('c','compile',1),summary('c','instantiate',1),summary('c','first-call',1,{latency_status:'not_timing_pass'}),
  ],['a','b','c','missing'].map(id=>({id})),'w',stages);
  assert.deepEqual(Array.from(rows,row=>[row.id,row.complete,row.total]),[['b',true,9],['a',true,60],['c',false,null],['missing',false,null]]);
});
