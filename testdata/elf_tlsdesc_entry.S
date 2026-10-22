// Independent assembler reference for the Go-generated x86 TLSDESC adapters.
.text
reference_entry:
#if defined(__x86_64__)
endbr64
pushq %rbp
movq %rsp, %rbp
pushq %rbx
pushq %rcx
pushq %rdx
pushq %rsi
pushq %rdi
pushq %r8
pushq %r9
pushq %r10
pushq %r11
pushq %rax
pushq $0
pushq $0
andq $-64, %rsp
#else
endbr32
pushl %ebp
movl %esp, %ebp
pushl %ebx
pushl %ecx
pushl %edx
pushl %esi
pushl %edi
pushl %eax
pushl $0
pushl $0
andl $-64, %esp
#endif
movl $1, %eax
cpuid
// OSXSAVE guarantees XGETBV and the enabled user-state save area.
testl $0x08000000, %ecx
jz .Lfxsize
xorl %ecx, %ecx
xgetbv
#if defined(__x86_64__)
movl %eax, -88(%rbp)
movl %edx, -96(%rbp)
#else
movl %eax, -28(%ebp)
movl %edx, -32(%ebp)
#endif
movl $13, %eax
xorl %ecx, %ecx
cpuid
movl %ebx, %eax
jmp .Lallocate
.Lfxsize:
movl $512, %eax
.Lallocate:
addl $63, %eax
andl $-64, %eax
#if defined(__x86_64__)
subq %rax, %rsp
cmpl $0, -88(%rbp)
je .Lfxsave
// Clear the complete XSAVE header, including reserved words.
leaq 512(%rsp), %rdi
xorl %eax, %eax
movl $8, %ecx
rep stosq
movl -88(%rbp), %eax
movl -96(%rbp), %edx
xsave64 (%rsp)
jmp .Lcall
.Lfxsave:
fxsave64 (%rsp)
.Lcall:
fninit
movq -80(%rbp), %rdi
movq 8(%rdi), %rdi
movq (%rdi), %rdi
movabsq $0x12345678, %rax
callq *%rax
movq -80(%rbp), %rdx
movq 8(%rdx), %rdx
addq 8(%rdx), %rax
subq %fs:0, %rax
movq %rax, -80(%rbp)
cmpl $0, -88(%rbp)
je .Lfxrestore
movl -88(%rbp), %eax
movl -96(%rbp), %edx
xrstor64 (%rsp)
jmp .Lreturn
.Lfxrestore:
fxrstor64 (%rsp)
.Lreturn:
movq -80(%rbp), %rax
leaq -72(%rbp), %rsp
popq %r11
popq %r10
popq %r9
popq %r8
popq %rdi
popq %rsi
popq %rdx
popq %rcx
popq %rbx
leave
retq
#else
// Keep a separate aligned outgoing cdecl argument area below the state.
addl $64, %eax
subl %eax, %esp
leal 64(%esp), %esi
cmpl $0, -28(%ebp)
je .Lfxsave
leal 512(%esi), %edi
xorl %eax, %eax
movl $16, %ecx
rep stosl
movl -28(%ebp), %eax
movl -32(%ebp), %edx
xsave (%esi)
jmp .Lcall
.Lfxsave:
fxsave (%esi)
.Lcall:
fninit
movl -24(%ebp), %eax
movl 4(%eax), %eax
movl (%eax), %eax
movl %eax, (%esp)
movl $0x12345678, %eax
calll *%eax
movl -24(%ebp), %edx
movl 4(%edx), %edx
addl 4(%edx), %eax
subl %gs:0, %eax
movl %eax, -24(%ebp)
cmpl $0, -28(%ebp)
je .Lfxrestore
movl -28(%ebp), %eax
movl -32(%ebp), %edx
xrstor (%esi)
jmp .Lreturn
.Lfxrestore:
fxrstor (%esi)
.Lreturn:
movl -24(%ebp), %eax
leal -20(%ebp), %esp
popl %edi
popl %esi
popl %edx
popl %ecx
popl %ebx
leave
retl
#endif
.section .note.GNU-stack,"",@progbits
