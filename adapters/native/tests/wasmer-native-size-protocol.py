#!/usr/bin/env python3
"""Verify deterministic native-size evidence from the selected Wasmer SDK."""
import hashlib,json,subprocess,sys,tempfile
from pathlib import Path
if len(sys.argv)!=2:raise SystemExit('usage: wasmer-native-size-protocol.py ADAPTER')
with tempfile.TemporaryDirectory(prefix='wasmbench-wasmer-size-') as directory:
    # One and two defined i32-returning functions; the second need not be exported.
    modules=[bytes.fromhex('0061736d010000000105016000017f030201000707010372756e00000a06010400412a0b'),
             bytes.fromhex('0061736d010000000105016000017f03030200000707010372756e00000a0b020400412a0b0400410d0b')]
    requests=[{'method':'describe'}]
    for count,wasm in enumerate(modules,1):
        artifact=Path(directory)/f'{count}.wasm';artifact.write_bytes(wasm)
        preparation={'profile':'code','artifact':str(artifact),'artifact_sha256':hashlib.sha256(wasm).hexdigest(),
                     'workload':{'abi':'core','reset':'stateless','export':'run','args':[],
                                 'oracle':{'kind':'exact_u64','expected':['42']}}}
        requests += [{'method':'prepare','prepare':preparation},{'method':'inspect'},{'method':'inspect'}]
    requests.append({'method':'close'})
    result=subprocess.run([sys.argv[1]],input=''.join(json.dumps({'version':1,'id':i+1,**r})+'\n'
                          for i,r in enumerate(requests)),capture_output=True,text=True,check=True,timeout=60)
    responses=[json.loads(line) for line in result.stdout.splitlines()]
    assert len(responses)==len(requests),result.stdout
    assert responses[0]['description']['capabilities']['can_measure_native_code_size'] is True
    sizes=[]
    for i in (1,4):
        assert responses[i]['status']=='ok',responses[i]
        for response in responses[i+1:i+3]:
            assert response['status']=='ok',response
            measurement=response['diagnostics'][0]
            assert measurement['metric']=='native.code_size' and measurement['status']=='available',measurement
            assert isinstance(measurement['value'],int) and measurement['value']>0,measurement
            assert 'defined-function' in measurement['reason'],measurement
        assert responses[i+1]['diagnostics'][0]['value']==responses[i+2]['diagnostics'][0]['value']
        sizes.append(responses[i+1]['diagnostics'][0]['value'])
    assert sizes[1]>sizes[0],sizes
    print(f'Wasmer native-size protocol passed: deterministic, complete defined functions ({sizes})')
