# 来源与许可

本项目根据 DDL 的通用对象/库/链接器设计进行 Go 移植重构，格式解析和现代重定位实现另行编写，没有捆绑 D 运行时或 DDL 源码树。

DDL 源码来源：[Marenz/ddl](https://github.com/Marenz/ddl)，参考快照 `3bf531e9701469ccecd5c3c698036ef4ef72362b`。原文件注释常写 `BSD Derivative`，实际许可条文在源文件头中；这里保留相关作者与条文副本于 [third_party/DDL-LICENSE.txt](third_party/DDL-LICENSE.txt)。

ABIBridge 和 llcppg 仅作架构/能力对照，不包含其源代码副本或运行时依赖。libffi 是通过构建标签启用的可选系统依赖，使用其系统安装的许可；Go 标准库及 llgo 编译器也分别遵循各自许可。

本地 `.research/` 只保存只读参考 checkout，已被 Git 忽略；构建不依赖该目录。
