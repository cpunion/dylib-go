static int tally;
int weak_value = 100;
void optional_event(int a, int b) { tally += a + b; }
int total(int a, int b) { (void)a; (void)b; return tally; }
