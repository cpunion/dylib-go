extern unsigned char narrow_a8, narrow_guard;
extern unsigned short narrow_a16;
extern signed char narrow_p8, narrow_m8;
extern short narrow_p16, narrow_m16;
int narrow_root(int a, int b) {
    return narrow_a8 == 42 && narrow_a16 == 4242 &&
           narrow_p8 == 7 && narrow_m8 == -9 &&
           narrow_p16 == 11 && narrow_m16 == -15 && narrow_guard == 0x5a
           ? a+b : -1;
}
