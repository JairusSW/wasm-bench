package main

// Explicit sample flags are authoritative. Defaults apply only to fresh plans;
// existing locks retain their recorded launch, sample and operation budgets.
func defaultScenarioSamples(profile string, scenarios []string, counts map[string]int, samplesExplicit, countsExplicit bool) map[string]int {
	if profile != "timing" || samplesExplicit || countsExplicit {
		return counts
	}
	result := map[string]int{"*": 1}
	for _, scenario := range scenarios {
		switch scenario {
		case "compile", "instantiate", "steady":
			result[scenario] = 3
		}
	}
	return result
}
