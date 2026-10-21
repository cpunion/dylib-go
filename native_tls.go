package dylib

import "fmt"

func nativeTLSSection(o *object, s symbol) *section {
	if s.section > 0 && s.section < len(o.sections) {
		sec := o.sections[s.section]
		if sec != nil && sec.nativeTLS != 0 {
			return sec
		}
	}
	return nil
}

func (im *image) publicSymbol(d definition) (uintptr, error) {
	target, err := im.resolveDefinition(d.o, d.index)
	if err != nil {
		return 0, err
	}
	if target.sym().tls || nativeTLSSection(target.o, target.sym()) != nil {
		return 0, fmt.Errorf("%s is native TLS; use a native getter on its owning OS thread", d.sym().name)
	}
	return im.symbol(d.o, d.index, false)
}
