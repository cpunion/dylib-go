.globl _alias_eval
_alias_eval = _eval
.globl _alias_chain
_alias_chain = _alias_eval
.private_extern _hidden_eval
_hidden_eval = _eval
.data
.globl _alias_address
_alias_address:
.quad _alias_eval
.subsections_via_symbols
