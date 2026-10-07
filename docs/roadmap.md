# 已完成的范围与后续扩展

当前版本完成了可独立构建、可真实调用的 Go 对象加载器：多格式识别、对象模型、归档按需链接、局部/全局/weak/common 符号、明确重定位集合、相邻 GOT/远调用跳板、W^X 内存、失败回滚、生命周期绑定、原生动态库、gc/llgo 双调用路径、可选动态标量 ABI。

它是 DDL 通用加载机制的现代实现，尚不是系统链接器的全功能替代。以下是明确未实现的扩展，不是隐藏的自动降级：

1. **Windows 原生执行验证**：AMD64 COFF 重定位/指针槽和 Windows 系统接口已有代码，先补真实 Windows/Mingw/MSVC 测试；然后实现 import library、COMDAT、SECREL/SECTION 和 unwind 注册。
2. **对象初始化与异常**：按平台实现初始化数组的时序、析构逆序、循环依赖和失败处理，再加入 `.eh_frame`/compact unwind/Windows runtime function tables。当前要求调用方使用完整动态库承担这些工作。
3. **TLS 与更多 relocation**：明确每个平台的 TLS 模型，增加线程注册/销毁；补 ARM64 COFF、RISC-V/LoongArch 等。只有 parser 能读文件不构成执行后端。
4. **归档与对象容器**：universal slice 选择、thin archive 路径约束、LLVM bitcode/IR 的可选编译阶段。OMF 和 D `.ddl` 不在兼容目标内。
5. **llcppg 动态生成模式**：与现有静态 `go:linkname` 输出并存，逐项验证头文件 ABI，而不尝试用 demangle 字符串推断所有类型。
6. **ABIBridge 专用适配器**：Apple 的 Swift/ObjC 值与调用层，通过 C ABI opaque handles 接入；平台限定、版本/硬件认证范围需单独测试。
7. **高级签名与回调**：可选 libffi record、变参、closure，以及 llgo 编译期 thunk。当前标量动态签名不缓存 prepared CIF，尚无吞吐基准；默认固定签名路径适合小型热点函数。
8. **独立执行器**：远程/隔离进程、Wasm、GPU 都使用单独的执行接口，避免把不同执行模型混入原生地址 API。

优先维持 Go 主体与明确边界；需要更低层的能力才扩展 llgo/小型 native 适配器。若将来提供可选 LLVM 后端，也应保留当前 Go 实现为独立后端，而不把 LLVM 变成默认硬依赖。
