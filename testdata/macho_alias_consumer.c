extern int alias_chain(int a, int b);
extern int hidden_eval(int a, int b);
extern int (*alias_address)(int a, int b);

int consume_alias(int a, int b) {
    return alias_chain(a, b) + hidden_eval(a, b) + alias_address(a, b);
}
