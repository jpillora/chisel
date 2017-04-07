// +build windows

package termutil

import (
	"syscall"
	"io"
	"errors"
)

var (
	f_getwch uintptr // wint_t _getwch(void)
)

func init() {
	f_getwch = syscall.MustLoadDLL("msvcrt.dll").MustFindProc("_getwch").Addr()
}

func GetPass(prompt string, prompt_fd, input_fd uintptr) ([]byte, error) {
	// Firstly, print the prompt.
	written := 0
	buf := []byte(prompt)
	for written < len(prompt) {
		n, err := syscall.Write(syscall.Handle(prompt_fd), buf[written:])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.EOF
		}

		written += n
	}

	// Write a newline after we're done, since it won't be echoed when the
	// user presses 'Enter'.
	defer syscall.Write(syscall.Handle(prompt_fd), []byte("\r\n"))

	// Read the characters
	var chars []uint16
	for {
		ret, _, _ := syscall.Syscall(f_getwch, 0, 0, 0, 0)
		if ret == 0x000A || ret == 0x000D {
			break
		} else if ret == 0x0003 {
			return nil, errors.New("Input has been interrupted by user.")
		} else if ret == 0x0008 {
			chars = chars[0:len(chars)-2]
		} else {
			chars = append(chars, uint16(ret))
		}
	}

	// Convert to string...
	s := syscall.UTF16ToString(chars)

	// ... and back to UTF-8 bytes.
	return []byte(s), nil
}
