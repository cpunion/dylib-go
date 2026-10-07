# 平台、编译目标和语言输出

下表区分**文件检查**、**重定位代码实现**、**实际执行证据**。测试样例通过不意味着某平台的全部 relocation、TLS、异常或语言 ABI 均已实现。

## 当前平台

| 宿主 / 目标 | 直接对象 | 系统动态库 | 当前证据 |
| --- | --- | --- | --- |
| macOS arm64 | Mach-O 64 `.o` / `.a` | `.dylib` / bundle | Go、llgo 执行测试；可选 libffi |
| macOS amd64 | Mach-O 64 `.o` / `.a` | `.dylib` | Go、llgo 在 GitHub Intel runner 的执行测试通过；另有本地 Rosetta 验证 |
| Linux arm64 | ELF64 LE RELA `.o` / `.a` | `.so` | Go、llgo 在 GitHub ARM64 runner 的执行测试通过 |
| Linux amd64 | ELF64 LE RELA `.o` / `.a` | `.so` | Go、llgo 在 GitHub AMD64 runner 的执行测试通过 |
| Windows amd64 | AMD64 COFF `.obj` / 普通 `.lib` 归档 | PE `.dll` | Go、llgo 在 GitHub Windows runner 的对象、归档、DLL、标量 ABI 执行测试通过 |
| Windows arm64 | COFF 可以检查；拒绝原始对象执行 | PE `.dll` | llgo 在原生 ARM64 runner 的 DLL、libffi、Go/llgo c-shared 测试通过；ARM64 COFF 重定位未实现 |
| Linux / Windows 386 | 可检查；拒绝 32 位对象执行 | 未提供本项目执行支持 | CI 实际运行 32 位 Go 解析与拒绝测试；llgo setup 暂不接受 386 |
| Linux RISC-V、LoongArch、PPC、s390x、ARM32 | 标准库能检查部分 ELF | 未提供本项目执行支持 | 没有对应重定位后端或执行测试 |
| FreeBSD 等 BSD | 可构建纯 Go 检查路径 | 未提供 native 后端 | 需要系统接口、OS ABI 与执行测试 |
| iOS / tvOS / watchOS / visionOS | 部分 Mach-O 元数据可检查 | 未提供宿主集成 | 识别并拒绝移动平台对象作为 macOS 插件；还涉及平台代码执行政策 |
| Wasm、GPU、eBPF、裸机 MCU | 不作为本库原生输入 | 不适用 | 需要独立执行器/驱动/内核加载器，见扩展领域 |

纯 Go 检查不要求文件的 CPU 与当前 CPU 相同。实际执行要求架构、位宽、格式一致，并校验可取得的 OS 信息。ELF `OSABI_NONE` 常见且不标明 libc/OS，不能靠文件头证明其 OS ABI 兼容。CPU 扩展指令能力也仍由调用方保证。Mach-O 保存原始 CPU subtype，拒绝 arm64e 等尚未实现的认证 ABI。

## 文件与重定位

| 输入 | 当前行为 |
| --- | --- |
| ELF/Mach-O/COFF 64 位可重定位对象 | Go 直接链接已实现的重定位；无需先调用系统链接器生成共享库 |
| GNU/BSD 普通 ar（`.a`、普通 COFF `.lib`） | 解析全部成员的元数据；只把有用成员装入映像，未选中对象的未定义依赖不影响链接 |
| `.so`、`.dylib`、PE DLL | 使用当前 OS 加载器处理完整映像、依赖、TLS 与初始化 |
| PE EXE、ELF EXEC/PIE、Mach-O EXEC | 识别为可执行文件后拒绝作为库装入 |
| thin archive、fat Mach-O、COFF import library/bigobj | 当前拒绝或不支持；先转换成普通目标切片/对象，DLL 可直接加载 |
| OMF、D `.ddl`、Go gc `.a`、LLVM bitcode、原始 LLVM IR | 不直接加载；IR/bitcode 可先由匹配工具链输出本机 `.o` |

已实现的主要重定位：

- ELF x86-64：NONE、64、PC32、PLT32、GOTPCREL/GOTPCRELX/REX_GOTPCRELX、32/32S、PC64。
- ELF AArch64：NONE、ABS64/ABS32、PREL64/PREL32、ADRP page、ADD/LDST8/16/32/64/128 low12、CALL26/JUMP26、GOT page/load64。
- Mach-O x86-64：UNSIGNED、SIGNED、BRANCH、GOT_LOAD/GOT、外部 SUBTRACTOR+UNSIGNED、SIGNED_1/2/4。
- Mach-O arm64：UNSIGNED、外部 SUBTRACTOR+UNSIGNED、BRANCH26、PAGE21/PAGEOFF12、GOT page/offset、POINTER_TO_GOT、配对 ADDEND。局部 section-ordinal 的指令重定位仍拒绝。
- COFF AMD64：ABSOLUTE、ADDR64、ADDR32、ADDR32NB、REL32 至 REL32_5；`__imp_` 符号通过 GOT 槽适配显式 DLL 符号。SECTION/SECREL、弱外部 auxiliary alias、COMDAT 未实现。

未知 relocation、溢出、段外写入、未知目标和重复强符号都会报错，不发布部分可执行映像。对异常展开段只跳过元数据，不注册 unwinder；因此**直接对象路径不支持抛异常/栈展开**。包含 TLS、自动构造析构、COMDAT 或已识别 Swift/ObjC 注册段的对象会明确拒绝。

## 语言输出

对象格式本身不等于语言支持。可调用性 = 文件/重定位支持 + ISA/OS ABI 一致 + 原型正确 + 语言运行时就绪。

| 生产者 | 推荐边界 | 实测 / 限制 |
| --- | --- | --- |
| C（clang；GCC 原生输出理论可用） | 普通 C ABI，`-fPIC` | clang 多平台对象、数据、BSS、common、跨对象、动态库已测；GCC 编译器专有扩展未全面验证 |
| 汇编（llvm-mc/clang/as 等） | C ABI 入口 | 格式及指令/重定位子集需匹配；不是任意汇编包自动兼容 |
| C++ | `extern "C"` 包装，或已知 mangled name 的简单函数 | 无异常/RTTI/复杂运行时的样例对象已测；完整 STL、类生命周期、继承布局用完整动态库及生成适配器 |
| Rust | `extern "C"` + 导出稳定符号，`panic=abort` | Linux/macOS 双架构 `no_std` leaf 对象实测；Rust ABI、trait object、panic unwinding 不支持 |
| Zig | `export fn`、C-compatible 参数 | Linux/macOS 双架构对象实测；Zig 内部 ABI、复杂布局不自动适配 |
| Fortran | `bind(C)` + `iso_c_binding`，值参数显式 `value` | Linux/macOS 双架构 gfortran 简单对象实测；I/O/数组描述符等需 libgfortran、运行时和包装 |
| Go gc | `go build -buildmode=c-shared` 的 `//export` | Linux/macOS/Windows 双架构动态库实测；Go 自有 `.a`、普通 Go ABI、GC 数据结构不作为 C ABI 调用 |
| llgo | 用其编译宿主；插件使用 C ABI 导出/`c-shared` | 宿主与插件都实测；不能因是 LLVM 输出就忽略 llgo GC、初始化和函数元数据 |
| Swift | 完整动态库 + C 导出 façade | macOS 双架构 C 导出 dylib 实测；原始对象因注册元数据拒绝；原生 Swift 泛型/async/throws 未实现 |
| Objective-C / ObjC++ | 系统加载 framework/dylib，C façade 或显式 ObjC 适配层 | 原始 ObjC 注册段拒绝；本库没有 objc_msgSend 签名/ARC 管理 |
| D | 若输出独立 C ABI 边界可按普通原生输入尝试 | 不保留 DDL 的 D 特例；没有 D 编译器/运行时验证 |
| Odin、Nim、Pascal、其他本机编译器 | C ABI 导出 + 完整运行时库或纯计算对象 | 架构上可扩展；尚无执行证据，不能计作已支持 |

固定 C 签名路径可由 llgo 编译成直接调用。可选 libffi 后端支持 `void/i32/u32/i64/u64/f32/f64/pointer`，最多 32 个固定参数，使用当前平台默认 C ABI；它不补齐 C++、Swift 或任意跨系统 ABI。

## 更多领域

Python/Ruby/Lua 扩展、JNI、本机数据库/图像/音视频计算插件，可在**宿主语言运行时已启动且入口协议正确**时复用本库。发现导出符号并不会自动初始化解释器或正确管理其对象。

Wasm 可另加 Go 的 Wasm 执行器接口（例如后续评估 wazero），不应走 native mmap 调用。CUDA/HIP/Metal/OpenCL 需要设备驱动、模块与 kernel launch API；eBPF 需要内核 verifier/loader。它们适合成为同一插件系统的独立后端，当前均未接入。

跨 CPU 可考虑独立执行进程 + RPC，跨 OS 还需相应系统与运行时。链接器重定位无法替代模拟器、Wine、ABI thunk 或系统服务。
