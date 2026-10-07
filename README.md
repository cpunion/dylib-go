# llgo-dylib

[![Go](https://github.com/cpunion/llgo-dylib/actions/workflows/go.yml/badge.svg)](https://github.com/cpunion/llgo-dylib/actions/workflows/go.yml)
[![llgo](https://github.com/cpunion/llgo-dylib/actions/workflows/llgo.yml/badge.svg)](https://github.com/cpunion/llgo-dylib/actions/workflows/llgo.yml)

用 Go 实现的运行时对象加载器：直接加载 `.o/.obj`、按需链接 `.a/.lib`，解析符号和重定位后在当前进程执行；`.so/.dylib/.dll` 由所在系统加载。

这是对早期 [DDL](https://github.com/Marenz/ddl) 通用加载机制的 Go 移植与重构，**不兼容 D ABI、ModuleInfo 或 D 运行时**。主体使用 Go 标准库 `debug/elf`、`debug/macho`、`debug/pe`；不依赖 LLVM C++、ORC 或 JITLink。普通 Go 用小型 cgo 桥调用 C 函数，llgo 用直接 C ABI 函数指针调用。

当前是有真实执行测试的实验性实现，支持范围见 [平台与语言矩阵](docs/support.md)。不是完整的系统链接器，也不会模拟不同操作系统或 CPU。

## 快速使用

需要 Go 1.23+ 和 C 编译器。使用加载器本身不需要 clang；下面用 clang 生成示例输入。

```sh
mkdir -p build
clang -fPIC -c testdata/add.c -o build/add.o
go run ./cmd/ddlgo inspect build/add.o
go run ./cmd/ddlgo call add 20 22 build/add.o
# 42

ar rcs build/add.a build/add.o
go run ./cmd/ddlgo call add 20 22 build/add.a
# 42

# 同一套 Go 源码由 llgo 编译，调用走直接 C 函数指针路径
llgo build -o build/ddlgo-llgo ./cmd/ddlgo
build/ddlgo-llgo call add 20 22 build/add.o
```

只检查元数据可以关闭 cgo：`CGO_ENABLED=0 go run ./cmd/ddlgo inspect build/add.o`。

```go
import dylib "github.com/cpunion/llgo-dylib"

s := dylib.New(dylib.Options{})
defer s.Close()
if err := s.Load("build/add.a"); err != nil { panic(err) }
add, err := s.BindInt32("add") // 已知签名：int32_t(int32_t,int32_t)
if err != nil { panic(err) }
result, err := add.Call(20, 22) // 42
```

`Load` 可调用多次，直到首次 `Link`、`Lookup` 或绑定。先添加所有依赖，再链接。链接失败可补充依赖后重试。链接成功后会话封闭；需要另一组插件时创建新会话。

## 调用接口

- `Inspect(path)`：不执行代码，返回格式、架构、符号和归档成员等元数据。
- `Load(path)` / `Define(name, address)`：添加输入或宿主提供的原生符号。
- `Link(roots...)`：显式指定归档根符号，完成按需提取和重定位。
- `Lookup(name)`：获取原生地址。Mach-O 名称移除一个链接器前缀 `_`；C++ 名称仍需其实际 mangled name。
- `BindInt32` / `CallInt32`：固定 C 签名，普通 Go 和 llgo 默认可用。
- `Bind(name, abi.Signature)`：可选的动态标量签名，使用 `-tags libffi`。
- `Close()`：释放对象代码和动态库引用；绑定句柄随后返回 `ErrClosed`。

不同浮点、整数、指针组合可选用 `abi`：

```go
import "github.com/cpunion/llgo-dylib/abi"

f, err := s.Bind("mixed", abi.Signature{
    Result: abi.F64,
    Args: []abi.Type{abi.I32, abi.F64, abi.F32, abi.U64},
})
// 检查 err 后：
v, err := f.Call(abi.Int32(10), abi.Float64(20.5), abi.Float32(1.5), abi.Uint64(10))
// math.Float64frombits(v.Bits) == 42
```

此路径需系统 libffi 开发文件和 pkg-config，使用 `go test -tags libffi ./...` 或 `llgo test -tags libffi ./...`。普通 Go 默认构建不需要 libffi；llgo 编译器自身运行时仍有其 GC/libffi 等依赖，独立于本包是否开启动态签名后端。当前不支持动态结构体、变参、回调、C++ `this` 调整或 Swift 调用约定。函数原型必须由调用方提供；符号名不构成原型验证。

Linux 使用 llgo v1.0.6 时，先设置 `export PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1`，避免该版本把空的 pkg-config C 编译参数误解析为 `-`；CI 已设置此兼容选项，普通 Go 不需要。

## 生命周期与边界

输入必须是可信原生代码。解析器和执行器均不提供安全沙箱。目标 ISA、对象格式、OS ABI、CPU 指令集和依赖需要与宿主匹配；不能在 macOS 直接运行 Windows DLL，也不能把 arm64 机器码当作 x86-64 运行。

对象链接采用 RW 分配、Go 重定位、指令缓存刷新、按段改为 RX/R/RW；没有 RWX 页面。当前直接对象路径拒绝已识别的 TLS、自动构造/析构、COMDAT 和语言运行时注册需求；不注册异常展开信息。需要这些功能时先构建完整的系统动态库。动态库的初始化、TLS 和依赖由系统加载器处理，但异常仍不得穿过调用边界。

`Function` 和 `Int32Func` 的调用与 `Close` 串行化。原生函数不得重入同一会话的加锁方法；原始地址、异步线程及回调的生命周期由调用者负责。Go 函数值不能强转成 C 函数指针；`Define` 的地址必须来自原生函数或原生内存。

Go/llgo 的 `c-shared` 以及其他启动后台线程的运行时库应使用 `Options{KeepLibraries:true}`，CLI 对应 `-keep-libraries`，直到进程退出保留系统库引用。`ProcessSymbols:true`（CLI `-process`）才会搜索 POSIX 宿主导出符号，Windows 应显式加载提供符号的 DLL。

## 目录与验证

```text
format_*.go, archive.go   Go 格式解析与统一对象模型
linker.go, relocate.go   Go 符号选择、归档提取、内存布局、重定位
bind.go, function.go     有所属会话的函数句柄
abi/                    Go 签名描述 + 可选 libffi 标量调用
internal/native/        OS 内存/动态库；gc 与 llgo 的底层调用
cmd/ddlgo/              检查与固定签名调用工具
testdata/               各语言真实编译输入
docs/                   移植设计、兼容矩阵、项目对比、验证记录
```

```sh
go test ./...
CGO_ENABLED=0 go test ./...
go test -race -tags libffi ./...
llgo test -tags libffi ./...
# 显式选择需要验证的编译器；缺少所选工具会失败
DYLIB_TEST_LANGUAGES=rust,zig,fortran,go go test -v ./...
# macOS 上验证全部语言，包括 Swift 和 llgo 插件
DYLIB_TEST_LANGUAGES=1 DYLIB_TEST_LLGO=1 go test -v ./...
```

参阅 [设计与移植对应关系](docs/design.md)、[ABIBridge / llcppg 对比](docs/comparison.md)、[CI 矩阵](docs/ci.md)、[测试证据](docs/validation.md) 和 [已知限制与后续路线](docs/roadmap.md)。
