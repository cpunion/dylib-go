//go:build cgo

package plainpadding

import dylib "github.com/ramblelabs/dylib-go"

func Call(path string) (float32, error) {
	session := dylib.New(dylib.Options{ProcessSymbols: true})
	defer session.Close()
	if err := session.Load(path); err != nil {
		return 0, err
	}
	b, err := NewBindings(session)
	if err != nil {
		return 0, err
	}
	v, err := b.Echo_wide(BindingsRecord0{Wide_real: 40.5, Wide_tag: 20})
	return v.Wide_real, err
}
