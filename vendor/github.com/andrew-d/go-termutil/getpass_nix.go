// +build linux darwin freebsd

package termutil

import (
	"io"
	"syscall"
	"unsafe"
)

func GetPass(prompt string, prompt_fd, input_fd uintptr) ([]byte, error) {
	// Firstly, print the prompt.
	written := 0
	buf := []byte(prompt)
	for written < len(prompt) {
		n, err := syscall.Write(int(prompt_fd), buf[written:])
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
	defer syscall.Write(int(prompt_fd), []byte("\n"))

	// Get the current state of the terminal
	var oldState syscall.Termios
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
		uintptr(input_fd),
		ioctlReadTermios,
		uintptr(unsafe.Pointer(&oldState)),
		0, 0, 0); err != 0 {
		return nil, err
	}

	// Turn off echo and write the new state.
	newState := oldState
	newState.Lflag &^= syscall.ECHO
	newState.Lflag |= syscall.ICANON | syscall.ISIG
	newState.Iflag |= syscall.ICRNL
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
		uintptr(input_fd),
		ioctlWriteTermios,
		uintptr(unsafe.Pointer(&newState)),
		0, 0, 0); err != 0 {
		return nil, err
	}

	// Regardless of how we exit, we need to restore the old state.
	defer func() {
		syscall.Syscall6(syscall.SYS_IOCTL,
			uintptr(input_fd),
			ioctlWriteTermios,
			uintptr(unsafe.Pointer(&oldState)),
			0, 0, 0)
	}()

	// Read in increments of 16 bytes.
	var readBuf [16]byte
	var ret []byte
	for {
		n, err := syscall.Read(int(input_fd), readBuf[:])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			if len(ret) == 0 {
				return nil, io.EOF
			}
			break
		}

		// Trim the trailing newline.
		if readBuf[n-1] == '\n' {
			n--
		}

		ret = append(ret, readBuf[:n]...)
		if n < len(readBuf) {
			break
		}
	}

	return ret, nil
}
