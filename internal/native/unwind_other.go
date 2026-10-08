//go:build !windows || (!amd64 && !arm64)

package native

import "fmt"

func AddFunctionTable(uintptr, uint32, uintptr) error {
	return fmt.Errorf("Windows runtime function tables require a Windows amd64/arm64 process")
}
func DeleteFunctionTable(uintptr) error {
	return fmt.Errorf("Windows runtime function tables require a Windows amd64/arm64 process")
}
