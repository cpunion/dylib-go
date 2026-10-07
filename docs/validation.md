# 验证记录

日期：2026-10-07。记录本次实际运行结果；后续不同工具链/主机需要重新执行。

## GitHub 原生 runner 验证

独立 Go / llgo 工作流和精确覆盖范围见 [CI 矩阵](ci.md)。在 [PR #1](https://github.com/cpunion/llgo-dylib/pull/1) 的 `001f665` 提交上，llgo 六个原生 runner 作业全部通过：Linux/macOS 双架构、Windows amd64 的对象/归档/动态库执行，以及 Windows arm64 的 DLL、标量 ABI、Go/llgo 共享库调用。Windows arm64 原始 COFF 重定位仍拒绝。

同一提交的 Go 五个原生作业的功能、语言接口和 race 测试全部通过，八个无 cgo 检查作业全部通过（包含实际运行的 Linux/Windows 386 测试进程）；Windows 的 `go vet` 另发现 `VirtualAlloc` 地址转换告警，后续提交已明确使用 OS 地址位重解释。最终结果以 PR 的最新提交检查为准。下面保留首次本地验证的环境和证据。

## 测试环境与结果

| 环境 | 编译器/运行方式 | 结果 |
| --- | --- | --- |
| macOS arm64 | Go 1.27.0；clang 22.1.8 | 默认测试、无 cgo 检查测试、`go vet` 通过 |
| macOS arm64 | Go `-race -tags libffi`，libffi 3.8.0 | 生命周期/并发、混合标量 ABI 测试通过 |
| macOS arm64 | 本机 llgo devel，LLVM 22.1.8 | 默认及 `-tags libffi` 测试通过，直接 C 函数指针路径实际调用成功 |
| macOS amd64 | `GOARCH=amd64 CGO_ENABLED=1 CC='clang -arch x86_64'`，Rosetta | 核心执行/归档/动态库测试通过；不代表物理 Intel CPU 测试 |
| Linux arm64 | Docker arm64，Go 1.26.5，clang 22.1.8 | `go test -tags libffi -v ./...` 通过 |
| Linux amd64 | Docker amd64，Go 1.27.0，clang 22；宿主 arm64 架构转换 | `go test -tags libffi -v ./...` 通过 |
| Windows amd64 | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c` | 交叉构建通过；没有 Windows 原生加载/调用测试 |
| Linux arm64 无 cgo | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c` | 交叉构建通过；不计作另一次执行验证 |

本地完整日志在 Git 忽略的 `build/verify-*.log`。Docker 验证使用既有环境，不是下载未知测试镜像：

- `llgo-pr2404-linux22-arm64:local`，image `a75eaf43ac8f567f0d5727b8a5c9a1827cdf7e524f36908cb2535a9ddfdc0cf7`。
- `llgo-dev-llvm22-amd64:e2e`，image `e09da790c4bfb31a6b1a7354d4c28529da074fd4679f7064e3794758c8497eca`。

这些是本机镜像标识，不要求其他使用者拥有它们。只需包含 Go、clang、ar、C 开发工具的对应平台环境；libffi 测试另需其开发包。没有在这些 Linux 容器中验证 llgo 编译宿主。

通过 `otool -L` 核对默认 CLI：普通 Go 版本只链接系统 `libresolv/libSystem`，没有 LLVM/libffi；llgo 版本链接工具链自身的 libffi、BDW GC、libc++ 和系统库，没有 LLVM 库。llgo 固定函数签名调用代码本身不经过可选 `abi/libffi.go`。

## 行为证据

测试会在临时目录用真实编译器生成输入，加载并断言结果，不使用伪造的“执行成功”：

- 独立 `.o` 中 `add(20,22)==42`。
- 数据、BSS、绝对指针、common 符号和跨对象引用。
- `.a` 只提取被引用成员；无用成员故意含 unresolved dependency，不能影响选中函数。
- 对象调用显式加载动态库中的函数、可选 libc 宿主符号、手工 `Define`。
- 缺依赖失败后补充对象重试；重复强符号拒绝；未知符号/根符号拒绝。
- 段外重定位和未知 relocation 失败时不发布映像，修正后可重试。
- `Close` 幂等、关闭后绑定句柄拒绝调用、调用/关闭并发串行化。
- 真实 TLS、构造函数、Swift 注册元数据对象拒绝，避免默默跳过初始化需求。
- 交叉生成 6 种 ELF/Mach-O/COFF 目标并检查；不兼容目标不能执行。同 ISA 的 iOS 对象不能当作 macOS 对象装入。
- 对 5 种格式/架构组合完成绝对指针重定位并检查目标地址。外来目标**没有被当成宿主代码执行**。
- x86-64 Mach-O `SIGNED_1/2/4` 使用真实 byte/word/dword store 指令回归，验证隐含 addend 不被重复减去。
- 可选 libffi：整数/浮点混合寄存器参数、负数返回、64 位高位保真、签名参数不匹配和关闭后调用。

## 语言生产者

在 macOS arm64 执行 `DYLIB_TEST_LANGUAGES=1 DYLIB_TEST_LLGO=1 go test -v ./...`：

| 生产者 | 版本 | 输入 / 结果 |
| --- | --- | --- |
| Rust | rustc 1.98.0 | `no_std` + `extern C` + `panic=abort` 的 `.o`，返回 42 |
| Zig | 0.16.0 | `export fn` 的 `.o`，返回 42 |
| Fortran | GNU Fortran 16.2.0 | `bind(C)` 的 `.o`，返回 42 |
| C++ | clang 22.1.8 | 无异常/RTTI的函数 + C wrapper `.o`，返回 42 |
| Swift | Apple Swift 6.3.3 | 原始 `.o` 因注册段拒绝；C 导出的 `.dylib` 返回 42 |
| Go gc | 1.27.0 | `c-shared` / `//export` 返回 42，保留运行时库引用 |
| llgo | 本机 devel | 相同 Go 插件源码 `c-shared` 返回 42，保留运行时库引用 |

这只证明样例中的 C ABI 子集，不证明语言全部特性、任意优化选项或任意运行时组合可用。现在显式选择的语言缺少工具会失败；未选择的语言不计入覆盖。GitHub 上的持续验证范围见 [CI 矩阵](ci.md)，实际运行结果以对应提交的 Actions 状态为准。

## 可复现命令

```sh
scripts/verify.sh
DYLIB_TEST_FFI=1 DYLIB_TEST_LLGO=1 scripts/verify.sh
DYLIB_TEST_LANGUAGES=1 DYLIB_TEST_LLGO=1 go test -v ./...
GOARCH=amd64 CGO_ENABLED=1 CC='clang -arch x86_64' go test -v ./...
```

最后一条针对带 Rosetta 的 macOS 环境；在原生对应平台直接运行标准测试即可。测试用 `CLANG` 指定 clang 可执行文件，默认 `clang`。
