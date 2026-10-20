// abibridgefixture prepares one native facade for all packages in a CI job.
// Its temporary artifacts intentionally survive this process until job cleanup.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cpunion/dylib-go/compiler/abibridge"
)

func main() {
	library, err := abibridge.Build(context.Background(), abibridge.Options{
		Source: os.Getenv("DYLIB_ABIBRIDGE_SOURCE"), LibffiSource: os.Getenv("DYLIB_ABIBRIDGE_LIBFFI_SOURCE"),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("DYLIB_ABIBRIDGE_FACADE=" + library.Path)
}
