//go:build !darwin

package experiment

func cloneExclusive(src, dst string) (bool, error) { return false, nil }
