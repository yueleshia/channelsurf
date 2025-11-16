//go:build !windows

package term

import "syscall"

func Sys_read(fd int, p []byte) (int, error) {
	return syscall.Read(fd, p)
}
func Sys_set_nonblock(fd int, nonblock bool) error {
	return syscall.SetNonblock(fd, nonblock)
}
