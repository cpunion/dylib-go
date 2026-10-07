# Sources and licenses

This project reimplements DDL's general object, library, and linker design in Go. File parsing and modern relocation handling were written separately. The D runtime and the DDL source tree are not bundled.

The DDL reference is [Marenz/ddl](https://github.com/Marenz/ddl), snapshot `3bf531e9701469ccecd5c3c698036ef4ef72362b`. Original comments often call the license `BSD Derivative`; the actual terms are in the source file headers. Relevant authorship and terms are preserved in [third_party/DDL-LICENSE.txt](third_party/DDL-LICENSE.txt).

ABIBridge and llcppg are architectural references, with no copied source or runtime dependency included. libffi is an optional system dependency enabled by a build tag and is distributed under its own installed license. The Go standard library and llgo compiler retain their respective licenses.

Local `.research/` checkouts are ignored by Git and are not required to build this project.
