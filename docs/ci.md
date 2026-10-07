# GitHub CI

普通 Go 和 llgo 使用独立工作流、独立测试进程：

- [Go 工作流](../.github/workflows/go.yml)：`actions/setup-go@v7`，Go 1.27.x。
- [llgo 工作流](../.github/workflows/llgo.yml)：`xgo-dev/setup-llgo@v0.2.0`，llgo v1.0.6、Go 1.27.x、LLVM 22；Windows 使用 MinGW profile。

工作流在 push、PR、手工触发时运行，不将未支持的目标伪装为原生执行成功。纯 Go 作业设置 `CGO_ENABLED=0`，在目标架构的进程中运行解析、错误处理与执行拒绝测试。

| 目标 | Go 原生执行 | Go 纯检查 | llgo |
| --- | --- | --- | --- |
| Linux amd64 / arm64 | 各自原生 runner | 各自原生 runner | 原生执行 |
| macOS amd64 / arm64 | Intel / Apple Silicon runner | 同左 | 原生执行 |
| Windows amd64 | 原生 runner，Clang/MinGW | 原生 runner | 原生执行，MinGW |
| Windows arm64 | 独立 gc 测试进程验证 DLL / 标量 ABI；复用 llgo 作业的工具链安装 | ARM64 runner | 系统 DLL 调用、标量 ABI、Go/llgo 共享库；ARM64 COFF 拒绝测试 |
| Linux 386 | 暂不提供 32 位对象执行 | amd64 Linux 上运行 32 位 Go 测试进程 | setup-llgo 当前不能安装 386 |
| Windows 386 | 暂不提供 32 位对象执行 | amd64 Windows 上运行 32 位 Go 测试进程 | setup-llgo 当前不能安装 386 |
| macOS 386 | 不适用 | Go 已无 darwin/386 port；CI 验证此边界 | 不适用 |

原生对象作业设置 `DYLIB_TEST_REQUIRE_NATIVE=1` 和 `DYLIB_TEST_REQUIRE_TOOLS=1`。缺少执行后端或必需工具时直接失败。Windows ARM64 作业命名为 `shared`，用 `DYLIB_TEST_REQUIRE_SHARED=1` 强制验证系统 DLL 调用，不宣称支持原始 ARM64 COFF 重定位。386 与 ARM64 COFF 测试由 clang 实际生成对象，再断言解析结果与加载拒绝。

## 语言与 ABI

| 接口 / 生产者 | Go 宿主 | llgo 宿主 |
| --- | --- | --- |
| C、无异常/RTTI 的 C++ 对象、归档、动态库 | 五个原生执行目标 | 五个原生执行目标 |
| 固定 `int32(int32,int32)` | cgo 桥 | llgo 直接 C ABI 函数指针 |
| libffi 混合整数/浮点标量接口 | 六个目标；其中五个原始对象目标另有 race 检查 | 六个目标；Windows 用系统 DLL 处理常量池 COMDAT |
| Rust `extern C`、Zig `export`、Fortran `bind(C)` | Linux/macOS 两架构 | Linux/macOS 两架构 |
| Swift C 导出动态库、原始元数据对象拒绝 | macOS 两架构 | macOS 两架构 |
| Go `c-shared` | 六个目标，含 Windows arm64 DLL | 六个目标，含 Windows arm64 DLL |
| llgo `c-shared` | 本地 macOS 与 CI Windows arm64；其余由 llgo 宿主验证 | 六个目标，含 Windows arm64 DLL |

这些是工作流的测试要求，成功与否应查看 [Go](https://github.com/cpunion/llgo-dylib/actions/workflows/go.yml) / [llgo](https://github.com/cpunion/llgo-dylib/actions/workflows/llgo.yml) 对应提交的结果。接口覆盖只代表样例中的 C ABI 子集；Windows Rust/Zig/Fortran、Linux Swift、ObjC 原生 ABI 等没有在本矩阵中验证。

语言测试通过 `DYLIB_TEST_LANGUAGES=rust,zig,fortran,swift,go,llgo` 按需选择。选中的编译器缺失或平台不适用会失败。可用 `DYLIB_RUSTC`、`DYLIB_ZIG`、`DYLIB_FC`、`DYLIB_SWIFTC`、`DYLIB_GO`、`DYLIB_LLGO` 指定可执行文件路径。Go/llgo 插件在子进程中加载，并保留库引用到进程退出，避免卸载运行时及 Windows DLL 文件锁影响清理。

## 上游边界

[setup-llgo v0.2.0](https://github.com/xgo-dev/setup-llgo/tree/v0.2.0) 的 [平台校验](https://github.com/xgo-dev/setup-llgo/blob/v0.2.0/src/platform.ts) 只接受 amd64/arm64，虽然 llgo 编译器本身已有部分 386 能力。本项目尚无 32 位重定位后端，不能把安装或交叉构建成功等同于动态对象执行支持。

llgo v1.0.6 在 Linux 上会把只有换行的 `pkg-config --cflags libffi` 输出误解析为 `-`，导致 Clang 读取额外标准输入、产生两份 AST JSON 并使 cgo 构建失败（[上游 issue #2749](https://github.com/xgo-dev/llgo/issues/2749)）。llgo 作业设置 `PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1` 保留系统 include 参数作为临时兼容措施；它不关闭任何测试。

Runner 标签依据 [GitHub 官方列表](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)，固定使用 ubuntu-24.04、ubuntu-24.04-arm、macos-15-intel、macos-15、windows-2022、windows-11-arm。
