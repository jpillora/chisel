// +build !windows,!linux,!darwin,!freebsd

package termutil

func GetPass(prompt string, prompt_fd, input_fd uintptr) ([]byte, error) {
	panic("not implemented")
}
