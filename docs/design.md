# DDL 的 Go 移植与实现边界

## 来源核实

恢复并阅读了 [Marenz/ddl](https://github.com/Marenz/ddl/tree/3bf531e9701469ccecd5c3c698036ef4ef72362b)，提交 `3bf531e9701469ccecd5c3c698036ef4ef72362b`。它包含 `ddl/`、`xf/linker/`、`meta/`，主页指向原 dsource DDL。不能凭这一份镜像证明完整 SVN 历史已恢复，但核心源码已可直接取得。

在这个快照中，[DefaultRegistry.d](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/DefaultRegistry.d) 注册 OMF、ELF 和 InSituMap，而归档、COFF 的默认注册被注释掉。[ELFBinary.d](https://github.com/Marenz/ddl/blob/3bf531e9701469ccecd5c3c698036ef4ef72362b/ddl/elf/ELFBinary.d) 明确拒绝 ELFCLASS64。因此不能把最早的规划和这个版本中所有目录的存在等同于完整格式支持。

## 对应关系

| DDL 源码职责 | Go 实现 | 改动 |
| --- | --- | --- |
| `DynamicLibraryLoader` / `LoaderRegistry` 的内容识别 | `parse`、各格式解析器、`Session.Load` | 依文件头识别；不根据后缀猜测 |
| `DynamicLibrary` / `DynamicModule` | 内部 `file/object/section/symbol/relocation` 模型 | 用结构体和标准库解析，去除 D 类/模块元数据 |
| `Linker.link` 依赖解析 | `selectObjects`、`definitions`、`image.symbol` | 确定性强/弱符号规则、common 合并、重复定义报错 |
| `ar/ArchiveReader` / `ArchiveLibrary` | `archive.go` | GNU/SysV、BSD 扩展名；从对象重建索引，按需提取 |
| 各模块 `resolveFixups` | `relocate.go` | 现代 64 位 ELF/Mach-O/COFF 子集、边界检查、GOT、远跳板 |
| `host` / `insitu` | `Define`、显式动态库、可选宿主符号 | 不依赖 D MAP/ModuleInfo；仅传原生地址 |
| `Memory` 和对象寿命 | `internal/native`、`Session.Close` | W^X、失败回滚、显式所属会话与调用锁 |
| 模板绑定和 D 反射 | `BindInt32`、`abi.Signature`、`Function` | C ABI 签名显式提供；不实现 D ABI |
| D ModuleInfo 构造/析构 | 不移植 | 系统动态库可承担语言运行时初始化；直接对象拒绝已识别需求 |

这是按功能重构的移植，不是逐行语法转换。旧 OMF 记录解析、D 名称修饰、类导出、Enki/meta 工具没有复制。底层现代重定位根据格式规范独立实现；见下方参考。

## 加载流程

1. `Load` 解析对象，暂存归档；动态库立即交给 OS，所以 `Load(shared)` 可能运行库构造函数。
2. 首次 `Link(roots...)` 或 `Lookup` 选择全部显式对象及必需归档成员。每加入一个成员就重建定义索引，直到依赖闭包稳定。
3. 全局强定义优先于 weak/common；多个强定义失败；common 取最大大小与对齐。局部符号保持对象内作用域。
4. 单个连续映射容纳全部段、common、GOT 和分支跳板。每个段独占页，以分离写/执行权限；总映像上限 64 MiB，单文件上限 256 MiB。超过页大小的段对齐明确拒绝。
5. Go 完成重定位和范围验证。分支到远处宿主地址时可用相邻跳板；不能把任意数据引用改成跳板。GOT 为间接引用保存原生地址。
6. 刷新指令缓存并设置段权限。所有步骤成功后发布映像；失败释放映射，允许补依赖后重试。动态库打开的副作用不属于这次映像回滚。
7. `Close` 先释放映像，再逆序释放系统动态库引用。`KeepLibraries` 则有意保留库句柄到进程退出。

查找不推断签名。`Lookup` 的原始地址只在会话存活期间有效；绑定句柄保留所属会话，调用时检查是否已经关闭。没有 finalizer 自动卸载代码，避免 GC 时机决定可执行代码的生命周期。

## Go 优先与 llgo 的位置

解析、归档、符号表、链接布局、错误处理、重定位、签名描述全部用 Go。默认无外部 Go 模块依赖。`CGO_ENABLED=0` 保留检查功能。

POSIX 映射与保护使用 Go `syscall`；Windows 使用 `VirtualAlloc` / `VirtualProtect` / `VirtualFree`。POSIX `dlopen/dlsym/dlclose` 和指令缓存刷新使用很小的 C 桥。普通 gc 通过 cgo 正确切换到 C ABI；不能将 Go `func` 的内部表示当成 C 指针。

`internal/native/call_llgo.go` 使用 `//llgo:type C` 定义函数类型，读取原生函数指针并直接调用。编译期固定签名是 llgo 优先路径。动态未知的标量签名可启用 `abi` 的 libffi 后端；这只承担寄存器/栈参数编排，不解析或链接二进制。

## 规范参考

- [Go debug/elf](https://pkg.go.dev/debug/elf)、[debug/macho](https://pkg.go.dev/debug/macho)、[debug/pe](https://pkg.go.dev/debug/pe)：文件结构解析。
- [Arm ELF64 ABI](https://github.com/ARM-software/abi-aa/blob/main/aaelf64/aaelf64.rst)：AArch64 ELF 重定位与对齐。
- [Microsoft PE/COFF](https://learn.microsoft.com/en-us/windows/win32/debug/pe-format)：COFF/PE、AMD64 重定位。
- [LLVM Mach-O ARM64 定义](https://github.com/llvm/llvm-project/blob/llvmorg-22.1.8/llvm/include/llvm/BinaryFormat/MachO.h)：格式常量与结构。
- [LLVM JITLink](https://llvm.org/docs/JITLink.html)：长期后端范围的对照；本项目没有使用其实现作为运行时后端。
