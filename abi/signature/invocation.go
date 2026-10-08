package signature

import (
	"fmt"
	"go/scanner"
	"go/token"
	"strings"
)

type lexeme struct {
	kind       token.Token
	text       string
	start, end int
}

func splitInvocation(text string) (name string, types, values []string, result string, err error) {
	fset := token.NewFileSet()
	file := fset.AddFile("invocation", -1, len(text))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(text), func(p token.Position, message string) {
		if err == nil {
			err = fmt.Errorf("%s: %s", p, message)
		}
	}, 0)
	var tokens []lexeme
	for {
		pos, kind, lit := lexer.Scan()
		if kind == token.EOF {
			break
		}
		if kind == token.SEMICOLON && lit == "\n" {
			continue
		}
		if lit == "" {
			lit = kind.String()
		}
		start := file.Offset(pos)
		tokens = append(tokens, lexeme{kind, lit, start, start + len(lit)})
	}
	if err != nil {
		return
	}
	i := 0
	fail := func() { err = fmt.Errorf("expected name(value:type, ...)result") }
	if len(tokens) < 3 || tokens[0].kind != token.IDENT || tokens[1].kind != token.LPAREN {
		fail()
		return
	}
	name = tokens[0].text
	i = 2
	for i < len(tokens) && tokens[i].kind != token.RPAREN {
		valueStart := tokens[i].start
		depth := 0
		for i < len(tokens) {
			k := tokens[i].kind
			if k == token.COLON && depth == 0 {
				break
			}
			if (k == token.COMMA || k == token.RPAREN) && depth == 0 {
				fail()
				return
			}
			switch k {
			case token.LPAREN, token.LBRACE, token.LBRACK:
				depth++
			case token.RPAREN, token.RBRACE, token.RBRACK:
				depth--
			}
			if depth < 0 {
				fail()
				return
			}
			i++
		}
		if i >= len(tokens) || tokens[i].start == valueStart {
			fail()
			return
		}
		values = append(values, strings.TrimSpace(text[valueStart:tokens[i].start]))
		i++
		if i >= len(tokens) {
			fail()
			return
		}
		typeStart := tokens[i].start
		end, ok := consumeType(tokens, i)
		if !ok {
			fail()
			return
		}
		types = append(types, text[typeStart:tokens[end-1].end])
		i = end
		if i < len(tokens) && tokens[i].kind == token.COMMA {
			i++
			if i >= len(tokens) || tokens[i].kind == token.RPAREN {
				fail()
				return
			}
		}
	}
	if i >= len(tokens) || tokens[i].kind != token.RPAREN {
		fail()
		return
	}
	result = strings.TrimSpace(text[tokens[i].end:])
	return
}

func consumeType(tokens []lexeme, i int) (int, bool) {
	if i >= len(tokens) {
		return i, false
	}
	if tokens[i].kind == token.MUL {
		return consumeType(tokens, i+1)
	}
	if tokens[i].kind == token.LBRACK {
		if i+2 >= len(tokens) || tokens[i+1].kind != token.INT || tokens[i+2].kind != token.RBRACK {
			return i, false
		}
		return consumeType(tokens, i+3)
	}
	if tokens[i].kind == token.IDENT {
		i++
		for i < len(tokens) && tokens[i].kind == token.PERIOD {
			i++
			if i >= len(tokens) || tokens[i].kind != token.IDENT {
				return i, false
			}
			i++
		}
		return i, true
	}
	if tokens[i].kind == token.LPAREN {
		end, ok := consumeType(tokens, i+1)
		if !ok || end >= len(tokens) || tokens[end].kind != token.RPAREN {
			return end, false
		}
		return end + 1, true
	}
	if tokens[i].kind == token.STRUCT && i+1 < len(tokens) && tokens[i+1].kind == token.LBRACE {
		depth := 0
		for j := i + 1; j < len(tokens); j++ {
			switch tokens[j].kind {
			case token.LBRACE:
				depth++
			case token.RBRACE:
				depth--
			}
			if depth == 0 {
				return j + 1, true
			}
		}
	}
	return i, false
}
