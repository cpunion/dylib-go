// Appended to the generated private C preamble by the compiler test. Callee
// addresses come from a separately compiled Clang unit, preventing GCC LTO or
// direct-call type inference from replacing the bridge's function prototype.
#define CALL(n) dylib_go_42696e64696e6773_call##n
#define VALUE(n) dylib_go_42696e64696e6773_record##n##_transport
#define CHECK(condition) do { if (!(condition)) return __LINE__; } while (0)

static int wide_is(VALUE(0) v,float real,int tag) { return v.f0==real && v.f1==tag; }

int main(void) {
    VALUE(0) wide={20.5f,20};
    CHECK(wide_is(CALL(0)(entry_echo_wide(),&wide),22,21));
    VALUE(1) lead={20.5f};
    CHECK(CALL(1)(entry_echo_lead(),&lead).f0==22);
    VALUE(2) tail={20.5f};
    CHECK(CALL(2)(entry_echo_tail(),&tail).f0==22);
    VALUE(3) pair={20.5f,19.5f};
    pair=CALL(3)(entry_echo_pair(),&pair);
    CHECK(pair.f0==22 && pair.f1==22);
    VALUE(4) mixed={-128,20.5};
    mixed=CALL(4)(entry_echo_mixed(),&mixed);
    CHECK(mixed.f0==-127 && mixed.f1==22);
    void *address=CALL(13)(entry_plain_pointer());
    CHECK(address!=0);
    VALUE(5) pointer={address,20.5f};
    pointer=CALL(5)(entry_echo_pointer(),&pointer);
    CHECK(pointer.f0==address && pointer.f1==22);
    VALUE(6) multiple={20,20.5f};
    multiple=CALL(6)(entry_echo_multiple(),&multiple);
    CHECK(multiple.f0==21 && multiple.f1==22);
    VALUE(7) array={.f0={{20.5f,20.5f},{20.5f,20.5f}}};
    array=CALL(7)(entry_echo_array(),&array);
    for(int i=0;i<2;i++)for(int j=0;j<2;j++)CHECK(array.f0[i][j]==22);
    VALUE(8) envelope={0};
    envelope.f0=20; envelope.f3=20.5;
    for(int i=0;i<2;i++) { envelope.f2[i]=lead; for(int j=0;j<2;j++)envelope.f1[i][j]=wide; }
    envelope=CALL(8)(entry_echo_envelope(),&envelope);
    CHECK(envelope.f0==21 && envelope.f3==22);
    for(int i=0;i<2;i++) { CHECK(envelope.f2[i].f0==22); for(int j=0;j<2;j++)CHECK(wide_is(envelope.f1[i][j],22,21)); }
    CHECK(wide_is(CALL(9)(entry_plain_variable(),&wide,-8,1.5f),22,12));
    CHECK(wide_is(CALL(16)(entry_plain_record_variable(),42,&wide,-8,1.5f),22,12));
    CHECK(wide_is(CALL(10)(entry_plain_overflow(),1,2,3,4,5,6,7,8,1,2,3,4,5,6,7,8,&wide),22,21));
    void *entry=CALL(11)(entry_plain_factory());
    CHECK(entry!=0);
    CHECK(wide_is(CALL(12)(entry_plain_forward(),entry,&wide),22,21));
    CHECK(wide_is(CALL(12)(entry_plain_forward(),0,&wide),20.5f,20));
    CALL(15)(entry_plain_write(),address,&wide);
    CHECK(wide_is(CALL(14)(entry_plain_read(),address),20.5f,20));
    return 0;
}
