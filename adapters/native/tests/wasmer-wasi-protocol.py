#!/usr/bin/env python3
"""Check WASI phases, output oracles, bounded capture, and code inspection."""
import hashlib,json,subprocess,sys,tempfile
from pathlib import Path

def leb(n):
    result=[]
    while True:
        byte=n&127;n>>=7;result.append(byte|(128 if n else 0))
        if not n:return bytes(result)
def name(s):
    data=s.encode();return leb(len(data))+data
def section(i,data):return bytes([i])+leb(len(data))+data
def module(write=False, modern_tag=False):
    types=b'\x02'+(b'\x60\x04\x7f\x7f\x7f\x7f\x01\x7f' if write else b'\x60\x01\x7f\x00')+b'\x60\x00\x00'
    imported=b'\x01'+name('wasi_snapshot_preview1')+name('fd_write' if write else 'proc_exit')+b'\x00\x00'
    exports=b'\x02'+name('memory')+b'\x02\x00'+name('_start')+b'\x00\x01'
    body=b'\x00'+(b'\x41\x01\x41\x00\x41\x01\x41\x10\x10\x00\x1a' if write else b'\x41\x07\x10\x00')+b'\x0b'
    result=b'\0asm\x01\0\0\0'+section(1,types)+section(2,imported)+section(3,b'\x01\x01')+section(5,b'\x01\x00\x01')+(section(13,b"\x01\x00\x00") if modern_tag else b"")+section(7,exports)+section(10,b'\x01'+leb(len(body))+body)
    if write:
        data=b'\x08\0\0\0\x02\0\0\0ok'
        result+=section(11,b'\x01\x00\x41\x00\x0b'+leb(len(data))+data)
    return result

if len(sys.argv)!=2:raise SystemExit('usage: wasmer-wasi-protocol.py ADAPTER')
with tempfile.TemporaryDirectory(prefix='wasmbench-wasmer-wasi-') as directory:
    process=subprocess.Popen([sys.argv[1]],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    request_id=0
    def request(method,**payload):
        global request_id
        request_id+=1
        process.stdin.write(json.dumps({'version':1,'id':request_id,'method':method,**payload})+'\n');process.stdin.flush()
        while True:
            line=process.stdout.readline()
            assert line,process.stderr.read()
            response=json.loads(line)
            assert response['id']==request_id,response
            if response['status']=='phase':
                process.stdin.write(json.dumps({'version':1,'id':request_id,'method':'continue'})+'\n');process.stdin.flush()
            else:return response
    description=request('describe')['description']
    assert description['capabilities']['can_run_commands'] is True
    for write in [False,True]:
        artifact=Path(directory)/('write.wasm' if write else 'exit.wasm');artifact.write_bytes(module(write))
        command={'argv':['test'],'exit_code':0 if write else 7,'stdout_sha256':hashlib.sha256(b'ok' if write else b'').hexdigest(),'output_limit_bytes':1024}
        workload={'abi':'wasi-command','host_profile':'wasi-preview1-readonly-v1','reset':'fresh_instance_per_sample','export':'_start','args':[],'oracle':{'kind':'exact_command','expected':[]},'command':command}
        preparation={'profile':'timing','artifact':str(artifact),'artifact_sha256':hashlib.sha256(artifact.read_bytes()).hexdigest(),'workload':workload}
        assert request('prepare',prepare=preparation)['status']=='ok'
        for scenario in ['compile','instantiate','first-call','steady']:
            response=request('run',run={'scenario':scenario,'samples':1 if scenario=='first-call' else 3,'operations':1,'warmup':2})
            assert response['status']=='ok',response
            measured=[s for s in response['samples'] if not s['warmup']]
            assert len(measured)==(1 if scenario=='first-call' else 3),response
            assert all(s['verified'] and s['command_result']['stdout_sha256']==command['stdout_sha256'] for s in measured),response
        preparation['profile']='memory'
        assert request('prepare',prepare=preparation)['status']=='ok'
        for scenario in ['compile','instantiate','first-call']:
            response=request('run',run={'scenario':scenario,'samples':1,'operations':1,'warmup':0,'phase_barriers':True})
            assert response['status']=='ok',response
        preparation['profile']='code'
        assert request('prepare',prepare=preparation)['status']=='ok'
        response=request('inspect');assert response['status']=='ok' and response['diagnostics'][0]['value']>0,response
        preparation['profile']='timing'
        command['stdout_sha256']='0'*64
        assert request('prepare',prepare=preparation)['status']=='ok'
        response=request('run',run={'scenario':'steady','samples':1,'operations':1,'warmup':0})
        assert response['status']=='error' and 'oracle mismatch' in response['reason'],response
        if write:
            command['output_limit_bytes']=1
            assert request('prepare',prepare=preparation)['status']=='ok'
            response=request('run',run={'scenario':'steady','samples':1,'operations':1,'warmup':0})
            assert response['status']=='error' and 'output budget' in response['reason'],response
    # Independent fixture-root regression: open a staged file using the broad
    # read-only rights requested by Go, then copy its exact bytes to stdout.
    types = b'\x03' + b'\x60\x09' + b'\x7f'*5 + b'\x7e'*2 + b'\x7f'*2 + b'\x01\x7f' + b'\x60\x04' + b'\x7f'*4 + b'\x01\x7f' + b'\x60\x00\x00'
    imported = b'\x03'
    for field, type_index in [('path_open',0),('fd_read',1),('fd_write',1)]:
        imported += name('wasi_snapshot_preview1') + name(field) + b'\x00' + leb(type_index)
    exports = b'\x02' + name('memory') + b'\x02\x00' + name('_start') + b'\x00\x03'
    # Trap on any syscall error. Memory: path at 0, iovec at 16, opened fd at 32.
    body = b'\x00' + bytes.fromhex('41034100410041094100') + b'\x42' + leb(0xff7febe) + b'\x42' + leb(0xfffffff) + bytes.fromhex('4100412010000440000b')
    body += bytes.fromhex('412028020041104101412810010440000b')
    body += bytes.fromhex('410141104101412c10020440000b0b')
    payload = b'input.txt' + b'\0'*7 + bytes.fromhex('4000000002000000')
    binary = b'\0asm\x01\0\0\0' + section(1,types) + section(2,imported) + section(3,b'\x01\x02') + section(5,b'\x01\x00\x01') + section(7,exports) + section(10,b'\x01'+leb(len(body))+body) + section(11,b'\x01\x00\x41\x00\x0b'+leb(len(payload))+payload)
    artifact = Path(directory)/'fixture-root.wasm';artifact.write_bytes(binary)
    workload = {'abi':'wasi-command','host_profile':'wasi-preview1-readonly-v1','reset':'fresh_instance_per_sample','export':'_start','args':[],'oracle':{'kind':'exact_command','expected':[]},'command':{'argv':['fixture-root'],'files':{'input.txt':{'data':'b2s=','sha256':hashlib.sha256(b'ok').hexdigest(),'size':2}},'exit_code':0,'stdout_sha256':hashlib.sha256(b'ok').hexdigest(),'output_limit_bytes':1024}}
    preparation = {'profile':'timing','artifact':str(artifact),'artifact_sha256':hashlib.sha256(binary).hexdigest(),'workload':workload}
    response=request('prepare',prepare=preparation);assert response['status']=='ok',response
    for scenario in ['compile','instantiate','first-call','steady']:
        response = request('run',run={'scenario':scenario,'samples':1,'operations':1,'warmup':0})
        assert response['status']=='ok',response
        assert response['samples'][0]['command_result']['stdout_sha256']==hashlib.sha256(b'ok').hexdigest(),response
    request('close');process.wait(timeout=5);assert process.returncode==0
print('Wasmer WASI protocol passed: timing, memory barriers, native size, exit/output oracles, bounded output')
