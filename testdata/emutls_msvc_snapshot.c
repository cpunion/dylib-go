extern void __dyn_tls_on_demand_init(void);
static int counter;
static void initialize(void) { ++counter; }
__attribute__((used, section(".CRT$XDU")))
static void (*volatile table)(void) = initialize;
int tls_snapshot_value(int a, int b) { (void)a; (void)b; return counter; }
int tls_snapshot_step(int a, int b) {
    table = (void (*)(void))1;
    __dyn_tls_on_demand_init();
    __dyn_tls_on_demand_init();
    return counter + a + b;
}
