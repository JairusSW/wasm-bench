#!/usr/bin/env python3
"""The AssemblyScript abort import must trap when called, not return success."""
import hashlib
import json
import subprocess
import sys
import tempfile
from pathlib import Path

if len(sys.argv) != 2:
    raise SystemExit("usage: wavm-abort-protocol.py /path/to/adapter-native")
with tempfile.TemporaryDirectory(prefix="wasmbench-wavm-abort-") as directory:
    prefix = "0061736d01000000010c0260047f7f7f7f006000017f020d0103656e760561626f72740000030201010707010372756e0001"
    requests = []
    for name, code in [("unused", "0a06010400412a0b"),
                       ("called", "0a10010e0041004100410041001000412a0b")]:
        artifact = Path(directory) / (name + ".wasm")
        artifact.write_bytes(bytes.fromhex(prefix + code))
        requests.append({"method": "prepare", "prepare": {
            "profile": "timing", "artifact": str(artifact),
            "artifact_sha256": hashlib.sha256(artifact.read_bytes()).hexdigest(),
            "workload": {"abi": "core", "reset": "stateless", "host_profile": "assemblyscript-abort-v1",
                         "export": "run", "args": [], "oracle": {"kind": "exact_u64", "expected": ["42"]}}
        }})
        requests.append({"method": "run", "run": {"scenario": "steady", "samples": 1,
                                                   "operations": 1, "warmup": 0}})
    requests.append({"method": "close"})
    result = subprocess.run([sys.argv[1]], input="".join(json.dumps({"version":1,"id":i+1,**r})+"\n"
                            for i,r in enumerate(requests)), capture_output=True, text=True, check=True, timeout=60)
    responses = [json.loads(line) for line in result.stdout.splitlines()]
    assert len(responses) == len(requests), result.stdout
    assert responses[0]["status"] == "ok" and responses[1]["status"] == "ok", responses[:2]
    assert responses[2]["status"] == "ok", responses[2]
    assert responses[3]["status"] == "error" and "trap" in responses[3]["reason"], responses[3]
    print("WAVM env.abort protocol passed: unused import runs, called import traps")
