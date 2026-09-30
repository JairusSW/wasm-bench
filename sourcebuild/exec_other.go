//go:build !linux && !darwin

package sourcebuild

import (
	"fmt"
	"os/exec"
)

func supported() error {
	return fmt.Errorf("source builds currently require Linux or Darwin process groups")
}
func configure(*exec.Cmd) {}
