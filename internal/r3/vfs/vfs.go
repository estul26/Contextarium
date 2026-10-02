//go:build r3 && cgo

// Package vfs exists only in the explicitly tagged R3 test executable.
package vfs

/*
#include <stdlib.h>
int r3_init(const char*,const char*,const char*,int);
void r3_phase(const char*);
void r3_report(void);
int r3_probe(const char*);
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func Init(root, trace, mode string, sector int) error {
	a, b, c := C.CString(root), C.CString(trace), C.CString(mode)
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(b))
	defer C.free(unsafe.Pointer(c))
	if rc := C.r3_init(a, b, c, C.int(sector)); rc != 0 {
		return fmt.Errorf("VFS initialization code %d", rc)
	}
	return nil
}
func Phase(s string) { p := C.CString(s); defer C.free(unsafe.Pointer(p)); C.r3_phase(p) }
func Probe(path string) int {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	return int(C.r3_probe(p))
}

// Report emits bounded counters, never database bytes. The shim never delegates xFetch.
func Report() { C.r3_report() }
