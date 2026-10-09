extern int got_abs_eval(int, int);
extern int got_based_eval(int, int);
extern int got_lea_eval(int, int);
extern int got_call_eval(int, int);
extern int got_jump_eval(int, int);
extern int weak_abs_eval(int, int);
int main(void) {
    return got_abs_eval(20, 22) != 84 || got_based_eval(20, 22) != 84 ||
           got_lea_eval(20, 22) != 84 || got_call_eval(20, 22) != 84 ||
           got_jump_eval(20, 22) != 84 || weak_abs_eval(20, 22) != 42;
}
