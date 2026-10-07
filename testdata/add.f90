function fortran_add(a, b) result(c) bind(C, name="fortran_add")
  use iso_c_binding
  integer(c_int), value :: a, b
  integer(c_int) :: c
  c = a + b
end function
