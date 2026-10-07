# Fixed-signature call example

This package demonstrates the exact C signature `int32_t(int32_t,int32_t)`.
The CLI and compiler probes reuse it; applications should supply adapters for
their own signatures. It is an example package, separate from the loader API.

Ordinary Go builds use a small cgo bridge. llgo builds use a caller-defined
`//llgo:type C` function pointer. Neither path enables the optional dynamic
libffi backend.

Use the adapter within a resolved symbol's lifetime guard:

```go
import examplecall "github.com/cpunion/dylib-go/examples/call"

symbol, err := session.Resolve("add")
if err != nil { panic(err) }
var result int32
err = symbol.WithAddress(func(address uintptr) error {
    var err error
    result, err = examplecall.Int32(address, 20, 22)
    return err
})
if err != nil { panic(err) }
// result == 42
```

The symbol must actually match this signature. For other types, see the
[mixed-signature cgo example](../cgo/main.go), the [llgo example](../llgo/main.go),
or the loader's `Bind(name, abi.Signature)` API for dynamic scalar calls.
