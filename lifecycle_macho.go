package dylib

import (
	"fmt"
	"math"
)

// Offset tables retain the original image's uint32 offsets, without relocations.
// Raw input sections get independent pages, so a uniform Mach-O slide cannot
// translate them. Normalize each selected entry to this private image's base.
// Parsed bytes remain intact for retries; unused archive members are untouched.
func (im *image) rebaseMachOInitializerOffsets() error {
	for _, o := range im.objects {
		for _, table := range o.sections {
			if table == nil || !table.initOffsets {
				continue
			}
			for pos := uint64(0); pos < table.size; pos += 4 {
				b := im.mem[table.offset+pos : table.offset+pos+4]
				value := uint64(le.Uint32(b))
				if value > math.MaxUint64-o.originalBase {
					return fmt.Errorf("%s: %s+%#x: initializer offset overflows original image", o.info.Name, table.name, pos)
				}
				address := o.originalBase + value
				var target *section
				for _, s := range o.sections {
					if s != nil && address >= s.original && address-s.original < s.size {
						if target != nil {
							return fmt.Errorf("%s: %s+%#x: ambiguous original initializer offset", o.info.Name, table.name, pos)
						}
						target = s
					}
				}
				if target == nil || !target.exec {
					return fmt.Errorf("%s: %s+%#x: initializer offset is outside executable object sections", o.info.Name, table.name, pos)
				}
				if err := unsigned32(b, int64(target.offset+(address-target.original))); err != nil {
					return fmt.Errorf("%s: %s+%#x: %w", o.info.Name, table.name, pos, err)
				}
			}
		}
	}
	return nil
}
