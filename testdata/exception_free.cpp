namespace Example { int add(int a, int b) { return a + b; } }
extern "C" int cpp_add(int a, int b) { return Example::add(a, b); }
