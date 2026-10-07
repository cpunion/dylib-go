# DDL、ABIBridge、llcppg 如何互补

## 对照的具体版本

- [DDL `3bf531e`](https://github.com/Marenz/ddl/tree/3bf531e9701469ccecd5c3c698036ef4ef72362b)：读了模型、注册表、Linker、ELF/OMF/COFF/归档部分。
- [ABIBridge `4dfbda2`](https://github.com/lynnswap/ABIBridge/tree/4dfbda22afb9a1492e1c5c99933a9defbc5437a4)：读了 README、RuntimeArchitecture、CFunctionInvocation、CXXObjectInvocation、Architectures 及源文件职责。
- [llcppg `6098773`](https://github.com/goplus/llcppg/tree/6098773c3116609e61c26968b83eec83e32a976c)：读了配置、Clang/mangling 入口、函数/类生成、实例输出。
- llgo 本地参考提交 `8ac217053d0337b7261cebc923bb6b17955bf49a`；实际编译器为本机 devel 版本，见验证记录。

## 职责比较

| 问题 | DDL | llgo-dylib | ABIBridge | llcppg |
| --- | --- | --- | --- | --- |
| `.o/.a` 入内存并修正地址 | 核心职责 | Go 实现核心职责 | 重点是已加载/系统加载的 Apple 映像，不替代通用对象链接器 | 不负责 |
| 从声明名找到导出符号 | D 修饰名/模板/反射特例 | 使用真实链接名；无通用 demangler | Swift/C++ 源码层名称解析、候选筛选 | 从头文件/Clang 信息生成实际 mangled symbol 绑定 |
| C ABI 调用 | 当时的 D 模板/函数指针 | gc 小型 cgo 桥、llgo 直接调用、可选动态标量 libffi | C ABI 后端使用 libffi，并有 Apple 认证指针处理 | 编译期生成 llgo 声明、类型和 C 调用约定 |
| C++ 对象与方法 | DDL 本身不解决一般 C++ ABI | 建议显式 C façade；复杂方法未实现 | 支持范围内的方法、receiver、vtable、适配器及所有权 | Clang 驱动的类、方法、布局/绑定生成；具体特性取决于生成器覆盖 |
| Swift/ObjC/SwiftUI | 不负责 | 原生支持未实现；系统库的 C façade 可用 | 专门的元数据、运行时和调用/值管理层 | 主要面向 C/C++，不提供原生 Swift ABI 适配 |
| 库和句柄寿命 | library/module | 会话、绑定句柄、关闭检查、可选永久库引用 | image lease、physical call plan、值和 callback owner | 主要是静态生成时/链接时语义 |

ABIBridge 的价值超出通用 libffi：例如 Swift 类型元数据、泛型 witness、间接返回、async/throws、值生命周期和 authenticated pointers。这些都需要具体语言/平台知识，不能由 `Lookup` 后做一次指针转换代替。其 [运行时分层说明](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Docs/RuntimeArchitecture.md) 把图像、调用计划、值和回调所有权分开；本项目采用了相同的职责边界思想，但没有复制 Swift/C++ 实现。

ABIBridge 的 [架构说明](https://github.com/lynnswap/ABIBridge/blob/4dfbda22afb9a1492e1c5c99933a9defbc5437a4/Sources/ABIBridge/ABIBridge.docc/Architectures.md) 也把元数据可识别、编译成功、设备执行分别标注。本项目同样不把某个外来 CPU 对象的解析/重定位测试写成该 CPU 的执行支持。ABIBridge 是 Apple 平台方案，不会自动增加本项目的 Windows/Linux 加载能力。

llcppg 的现有输出通常是 `//go:linkname F C.<mangled-name>`，配合 `LLGoPackage` 链接参数；这意味着符号在宿主构建/链接时绑定。它的 [函数生成器](https://github.com/goplus/llcppg/blob/6098773c3116609e61c26968b83eec83e32a976c/cl/func.go) 区分全局/静态函数和带 `this` 的方法。**现有输出不能直接宣称已支持运行时重新绑定**，但它已有的头文件语义和类型信息非常适合作为动态绑定生成的输入。

## 建议的组合接口

```text
头文件 / Clang / llcppg                 Apple runtime / ABIBridge adapter
  提供类型、原型、mangled name                 提供原生 Swift/ObjC 适配
                 \                         /
                  显式声明与所属库 / 对象契约
                             |
          llgo-dylib Session + Bind + Lookup
              /                           \
      Go 对象链接器                     OS 动态库加载器
              \                           /
              typed llgo call / 可选 C libffi call
```

可实现的互补路线，**以下尚未接入外部项目**：

1. 为 llcppg 增加可选动态输出模式：保留它生成的类型、名称和布局信息，把可调用函数改成带 `Session` 的绑定字段；固定签名生成 `//llgo:type C` 函数指针。无需让本项目重写头文件解析器。
2. 定义独立的声明清单，包含目标 triple、C/C++ ABI、原生链接名、返回/参数、record 大小/对齐、this/隐藏返回参数、所有权和异常约束。现有 `abi.Signature` 只覆盖第一步的标量 C ABI；不能把复杂类型强行压成若干整数。
3. C++ 复杂类首先生成小型 `extern "C"` façade：由匹配编译器处理构造、析构、继承、返回值和异常转错误码。再将 façade 对象或动态库交给本项目。可减少手写跨编译器 ABI 的范围。
4. Apple 专用扩展可以在 Swift/ObjC++ 侧使用 ABIBridge，再暴露少量 C ABI 操作。Go 侧保存 opaque handle，明确 retain/release 和主线程要求。需要新增并测试适配器；本项目目前没有这种运行时依赖。
5. 将来支持 callback 时，原生入口、Go 闭包、所属库与回调注册都必须有显式寿命协议。不能仅返回一个临时函数地址；也不能跨 Go/C++/Swift 边界传播各自的异常。

短期最适合的扩展领域是带头文件的 C/C++ SDK、插件引擎和数值计算库。SwiftUI、Swift 泛型/async、Objective-C 动态派发是 ABIBridge 路线的专门扩展，不能列入本库当前已实现的功能。
