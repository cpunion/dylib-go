#include "exports.h"
#include <stdarg.h>
Plain echo_plain(Plain v) { v.x+=1.5f; v.y+=1; return v; }
Packed echo_packed(Packed v) { v.x+=1.5f; v.y+=1; return v; }
Aligned echo_aligned(Aligned v) { v.x+=1.5f; v.y+=1; return v; }
PlainAlias echo_alias(PlainAlias v) { return echo_plain(v); }
Envelope echo_envelope(Envelope v) {
    for(int i=0;i<2;i++) { v.a[i]=echo_plain(v.a[i]); v.b[i]=echo_packed(v.b[i]); v.c[i]=echo_aligned(v.c[i]); }
    return v;
}
PackedFn packed_factory(void) { return echo_packed; }
Packed apply_packed(PackedFn f,Packed v) { return f?f(v):v; }
Plain *plain_pointer(Plain v) { static Plain storage; storage=v; return &storage; }
Packed *packed_pointer(Packed v) { static Packed storage; storage=v; return &storage; }
Plain read_plain(Plain *v) { return echo_plain(*v); }
Packed read_packed(Packed *v) { return echo_packed(*v); }
Packed packed_variable(Packed v,...) {
    va_list args; va_start(args,v); v.y+=va_arg(args,int); v.x+=(float)va_arg(args,double); va_end(args); return v;
}
#if defined(__APPLE__) && defined(__aarch64__)
Wide echo_wide(Wide v) { v.x+=1.5f; v.y+=1; return v; }
Small echo_small(Small v) { v.x+=1.5f; v.y+=1; return v; }
#endif
