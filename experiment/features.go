package experiment

import (
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
)

func incompatibleValidatorFeatures(policy *protocol.ValidatorFeaturePolicy, required []string) string {
	if policy == nil || policy.Namespace != "wasmparser/0.251.0" || policy.Evidence == "" {
		return ""
	}
	for _, feature := range required {
		if supported, declared := policy.Supported[feature]; declared && !supported {
			return fmt.Sprintf("artifact requires validator flag %s under its locked policy; adapter explicitly disables it (%s; %s)", feature, policy.Namespace, policy.Evidence)
		}
	}
	return ""
}
