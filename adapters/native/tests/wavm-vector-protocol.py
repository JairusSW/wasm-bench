#!/usr/bin/env python3
"""Exercise WAVM ordered-vector lifecycle and output verification."""
import hashlib
import json
import subprocess
import sys
import tempfile
from pathlib import Path

if len(sys.argv) != 2:
    raise SystemExit("usage: wavm-vector-protocol.py /path/to/adapter-native")
with tempfile.TemporaryDirectory(prefix="wasmbench-wavm-vector-") as directory:
    artifact = Path(directory) / "vector.wasm"
    # (memory (export "memory") 1), run(i32,i32,i32)->i32 returns 0;
    # a data segment initializes output byte 0 to 1. Input is written at 16.
    artifact.write_bytes(bytes.fromhex(
        "0061736d0100000001080160037f7f7f017f030201000503010001"
        "0710020372756e0000066d656d6f72790200"
        "0a0601040041000b0b07010041000b0101"
    ))
    workload = {"abi": "core", "reset": "fresh_instance_per_sample", "export": "run", "args": [],
                "oracle": {"kind": "exact_vectors", "expected": []}, "vector_byte_budget": 5,
                "vectors": {"input_offset": 16, "output_offset": 0, "output_len": 1,
                            "mod": 2, "cases": [{"len": 4, "out": "01"}]}}
    preparation = {"profile": "timing", "artifact": str(artifact),
                   "artifact_sha256": hashlib.sha256(artifact.read_bytes()).hexdigest(),
                   "workload": workload}
    requests = [{"method": "describe"}, {"method": "prepare", "prepare": preparation}]
    for scenario in ["compile", "instantiate", "first-call", "steady"]:
        requests.append({"method": "run", "run": {"scenario": scenario, "samples": 3,
                        "operations": 1, "warmup": 1}})
    bad = json.loads(json.dumps(preparation))
    bad["workload"]["vectors"]["cases"][0]["out"] = "02"
    requests += [{"method": "prepare", "prepare": bad},
                 {"method": "run", "run": {"scenario": "steady", "samples": 1,
                                             "operations": 1, "warmup": 0}},
                 {"method": "close"}]
    envelopes = [{"version": 1, "id": i + 1, **r} for i, r in enumerate(requests)]
    result = subprocess.run([sys.argv[1]], input="".join(json.dumps(r)+"\n" for r in envelopes),
                            capture_output=True, text=True, check=True, timeout=60)
    responses = [json.loads(line) for line in result.stdout.splitlines()]
    assert len(responses) == len(requests), result.stdout
    assert responses[0]["description"]["capabilities"]["can_run_vectors"] is True
    assert responses[1]["status"] == "ok", responses[1]
    for response in responses[2:6]:
        assert response["status"] == "ok" and len([s for s in response["samples"] if not s["warmup"]]) == 3, response
    assert responses[7]["status"] == "error", responses[7]
    print("WAVM vector protocol passed: four phases, three samples, incorrect output rejected")
