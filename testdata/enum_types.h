#ifndef DYLIB_TEST_ENUM_TYPES_H
#define DYLIB_TEST_ENUM_TYPES_H

#if defined(DYLIB_ENUM_TYPES)
/* Fixed C enum types are a Clang extension in the C17 producer fixtures. */
typedef enum : signed char { EnumI8Min=-128, EnumI8Max=127 } TestEnumI8;
typedef enum : unsigned char { EnumU8Max=255 } TestEnumU8;
typedef enum : short { EnumI16Min=-32768, EnumI16Max=32767 } TestEnumI16;
typedef enum : unsigned short { EnumU16Max=65535 } TestEnumU16;
typedef enum : int { EnumI32Min=-2147483647-1, EnumI32Max=2147483647 } TestEnumI32;
typedef enum : unsigned int { EnumU32Max=4294967295U } TestEnumU32;
typedef enum : long long { EnumI64Min=-9223372036854775807LL-1, EnumI64Max=9223372036854775807LL } TestEnumI64;
typedef enum : unsigned long long { EnumU64Max=18446744073709551615ULL } TestEnumU64;
typedef enum : _Bool { EnumFalse=0, EnumTrue=1 } TestEnumBool;
#else
typedef signed char TestEnumI8;
typedef unsigned char TestEnumU8;
typedef short TestEnumI16;
typedef unsigned short TestEnumU16;
typedef int TestEnumI32;
typedef unsigned int TestEnumU32;
typedef long long TestEnumI64;
typedef unsigned long long TestEnumU64;
typedef _Bool TestEnumBool;
#endif

#endif
