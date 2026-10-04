package main

import "github.com/wasmbench/wasmbench/protocol"

func validatorFeaturePolicy() *protocol.ValidatorFeaturePolicy {
	return &protocol.ValidatorFeaturePolicy{
		Namespace: "wasmparser/0.251.0",
		Evidence:  "wazero v1.12.0 api/features.go and experimental/features.go; engine explicitly configured WithCoreFeatures(api.CoreFeaturesV2)",
		// These experimental bits are excluded from CoreFeaturesV2. This is
		// deliberately not a blanket declaration about every validator flag.
		Supported: map[string]bool{
			"THREADS": false, "TAIL_CALL": false, "EXTENDED_CONST": false,
			"EXCEPTIONS": false, "FUNCTION_REFERENCES": false,
            "RELAXED_SIMD": false, "GC": false, "MEMORY64": false,
            "MULTI_MEMORY": false, "STACK_SWITCHING": false, "LEGACY_EXCEPTIONS": false,
		},
	}
}
