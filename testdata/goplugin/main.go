package main

import "C"

//export go_add
func go_add(a, b C.int) C.int { return a + b }

func main() {}
