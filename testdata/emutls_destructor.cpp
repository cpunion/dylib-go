struct TLSObject {
    int value = 7;
    ~TLSObject();
};
thread_local TLSObject tls_object;
int tls_destructor_root() { return tls_object.value; }
