# go-termutil

This package exposes some very basic, useful functions:

    Isatty(file *os.File) bool

This function will return whether or not the given file is a TTY, attempting to use native
operations when possible.  It wil fall back to using the `isatty()` function from `unistd.h`
through cgo if on an unknown platform.

		GetPass(prompt string, prompt_fd, input_fd uintptr) ([]byte, error)

This function will print the `prompt` string to the file identified by `prompt_fd`, prompt the user
for a password without echoing the password to the terminal, print a newline, and then return the
given password to the user.  NOTE: not yet tested on anything except Linux & OS X.
