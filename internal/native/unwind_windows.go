//go:build windows && (amd64 || arm64)

package native

import "fmt"

var addFunctionTable = kernel32.NewProc("RtlAddFunctionTable")
var deleteFunctionTable = kernel32.NewProc("RtlDeleteFunctionTable")

func AddFunctionTable(table uintptr, count uint32, base uintptr) error {
	r, _, _ := addFunctionTable.Call(table, uintptr(count), base)
	if byte(r) == 0 {
		return fmt.Errorf("RtlAddFunctionTable failed")
	}
	return nil
}

func DeleteFunctionTable(table uintptr) error {
	r, _, _ := deleteFunctionTable.Call(table)
	if byte(r) == 0 {
		return fmt.Errorf("RtlDeleteFunctionTable failed; native code/table storage retained")
	}
	return nil
}
