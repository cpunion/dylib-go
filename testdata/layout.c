#include <stdint.h>
#include <stddef.h>

#ifdef _WIN32
#define API __declspec(dllexport)
#else
#define API
#endif

struct layout_padded { int8_t tag; double value; int16_t tail; };
struct layout_nested {
    int8_t tag;
    struct layout_padded inner;
    int32_t values[3];
    void *pointer;
    _Bool ready;
};
typedef struct layout_padded record_array[2];
typedef int16_t matrix[2][3];
typedef struct layout_padded *pointer_array[3];

#define TYPES(M) \
    M(0,int8_t) M(1,uint8_t) M(2,int16_t) M(3,uint16_t) \
    M(4,int32_t) M(5,uint32_t) M(6,int64_t) M(7,uint64_t) \
    M(8,float) M(9,double) M(10,_Bool) M(11,void *) \
    M(12,struct layout_padded) M(13,struct layout_nested) \
    M(14,record_array) M(15,matrix) M(16,pointer_array) \
    M(17,struct layout_padded *)

API uint64_t native_layout_size(int32_t type) {
    switch(type) {
#define SIZE_CASE(ID,T) case ID: return sizeof(T);
        TYPES(SIZE_CASE)
#undef SIZE_CASE
    }
    return UINT64_MAX;
}
API uint64_t native_layout_alignment(int32_t type) {
    switch(type) {
#define ALIGN_CASE(ID,T) case ID: return _Alignof(T);
        TYPES(ALIGN_CASE)
#undef ALIGN_CASE
    }
    return UINT64_MAX;
}
API uint64_t native_layout_offset(int32_t type, int32_t member) {
    switch(type) {
    case 12: {
        static const size_t offsets[] = {
            offsetof(struct layout_padded,tag), offsetof(struct layout_padded,value),
            offsetof(struct layout_padded,tail)
        };
        if (member>=0 && member<3) return offsets[member];
        break;
    }
    case 13: {
        static const size_t offsets[] = {
            offsetof(struct layout_nested,tag), offsetof(struct layout_nested,inner),
            offsetof(struct layout_nested,values), offsetof(struct layout_nested,pointer),
            offsetof(struct layout_nested,ready)
        };
        if (member>=0 && member<5) return offsets[member];
        break;
    }
    case 14: if (member>=0 && member<2) return sizeof(((record_array *)0)[0][0])*member; break;
    case 15: if (member>=0 && member<2) return sizeof(((matrix *)0)[0][0])*member; break;
    case 16: if (member>=0 && member<3) return sizeof(((pointer_array *)0)[0][0])*member; break;
    }
    return UINT64_MAX;
}
