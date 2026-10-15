/* Independently written split startup fixtures. PART=0/1/2 selects each input. */
#if defined(__x86_64__)
#define SECTION(N) .section N,"ax",@progbits
#define PROLOGUE subq $8,%rsp
#define EPILOGUE addq $8,%rsp; ret
#define CALL(N) call N
#elif defined(__i386__)
#define SECTION(N) .section N,"ax",@progbits
#define PROLOGUE pushl %ebx; subl $8,%esp
#define EPILOGUE addl $8,%esp; popl %ebx; ret
#define CALL(N) call N
#elif defined(__aarch64__)
#define SECTION(N) .section N,"ax",%progbits
#define PROLOGUE stp x29,x30,[sp,#-16]!; mov x29,sp
#define EPILOGUE ldp x29,x30,[sp],#16; ret
#define CALL(N) bl N
#endif
SECTION(.init)
#if PART == 0
.p2align 2
.globl _init
.hidden _init
_init:
PROLOGUE
#elif PART == 1
.p2align 5
CALL(startup_init_marker)
.globl startup_anchor
startup_anchor:
#elif PART == 2
.p2align 4
EPILOGUE
#endif
SECTION(.fini)
#if PART == 0
.p2align 2
.globl _fini
.hidden _fini
_fini:
PROLOGUE
#elif PART == 1
.p2align 5
CALL(startup_fini_marker)
#elif PART == 2
.p2align 4
EPILOGUE
#endif
#if defined(__aarch64__)
.section .note.GNU-stack,"",%progbits
#else
.section .note.GNU-stack,"",@progbits
#endif
