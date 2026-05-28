package zon

import "fmt"

// parse lexes and parses a complete ZON document into an AST.
func parse(src string) (node, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	n, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	if p.cur().kind != tokenEOF {
		return nil, p.errorf(p.cur(), "unexpected trailing data after top-level value")
	}
	return n, nil
}

type parser struct {
	toks  []token
	pos   int
	depth int
}

// cur returns the current token.
func (p *parser) cur() token { return p.toks[p.pos] }

// at returns the token at the given offset from the current position, or EOF if out of bounds.
func (p *parser) at(off int) token {
	i := p.pos + off
	if i >= len(p.toks) {
		return p.toks[len(p.toks)-1] // tokenEOF
	}
	return p.toks[i]
}

// advance returns the current token and moves the parser position forward.
func (p *parser) advance() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

// errorf returns a SyntaxError at the specified token's location with a formatted message.
func (p *parser) errorf(t token, format string, args ...any) error {
	return &SyntaxError{msg: fmt.Sprintf(format, args...), Line: t.line, Col: t.col}
}

const maxParseDepth = 1000

// parseValue parses any valid ZON value (aggregates, strings, numbers, booleans, enums, null).
func (p *parser) parseValue() (node, error) {
	p.depth++
	if p.depth > maxParseDepth {
		return nil, p.errorf(p.cur(), "exceeded maximum parsing depth of %d", maxParseDepth)
	}
	defer func() { p.depth-- }()

	t := p.cur()
	switch t.kind {
	case tokenDotBrace:
		return p.parseAggregate()
	case tokenString:
		p.advance()
		return stringNode{value: t.value}, nil
	case tokenInt:
		p.advance()
		return numberNode{raw: t.value}, nil
	case tokenFloat:
		p.advance()
		return numberNode{raw: t.value, isFloat: true}, nil
	case tokenMinus:
		return p.parseNegative()
	case tokenDot:
		// Enum literal: . identifier
		p.advance()
		id := p.cur()
		if id.kind != tokenIdent {
			return nil, p.errorf(id, "expected identifier after .")
		}
		p.advance()
		return enumNode{name: id.value}, nil
	case tokenIdent:
		p.advance()
		switch t.value {
		case "true":
			return boolNode{value: true}, nil
		case "false":
			return boolNode{value: false}, nil
		case "null":
			return nullNode{}, nil
		case "inf":
			return numberNode{raw: "inf", isFloat: true}, nil
		case "nan":
			return numberNode{raw: "nan", isFloat: true}, nil
		default:
			return nil, p.errorf(t, "unexpected identifier %q", t.value)
		}
	default:
		return nil, p.errorf(t, "unexpected token")
	}
}

// parseNegative parses a negative number or a negative inf float value.
func (p *parser) parseNegative() (node, error) {
	p.advance() // -
	t := p.cur()
	switch t.kind {
	case tokenInt:
		p.advance()
		return numberNode{raw: "-" + t.value}, nil
	case tokenFloat:
		p.advance()
		return numberNode{raw: "-" + t.value, isFloat: true}, nil
	case tokenIdent:
		if t.value == "inf" {
			p.advance()
			return numberNode{raw: "-inf", isFloat: true}, nil
		}
		return nil, p.errorf(t, "unexpected %q after -", t.value)
	default:
		return nil, p.errorf(t, "expected number after -")
	}
}

// parseAggregate parses .{ ... }. It is a struct when the contents begin with a
// field assignment (.name = ...); otherwise it is a tuple. An empty .{} is an
// empty tuple.
func (p *parser) parseAggregate() (node, error) {
	p.advance() // .{
	if p.cur().kind == tokenRBrace {
		p.advance()
		return tupleNode{}, nil
	}
	if p.cur().kind == tokenDot && p.at(1).kind == tokenIdent && p.at(2).kind == tokenEquals {
		return p.parseStructBody()
	}
	return p.parseTupleBody()
}

// parseStructBody parses the field sequence of a ZON struct inside braces.
func (p *parser) parseStructBody() (node, error) {
	var fields []field
	for {
		if p.cur().kind != tokenDot {
			return nil, p.errorf(p.cur(), "expected field name")
		}
		p.advance()
		name := p.cur()
		if name.kind != tokenIdent {
			return nil, p.errorf(name, "expected identifier for field name")
		}
		p.advance()
		if p.cur().kind != tokenEquals {
			return nil, p.errorf(p.cur(), "expected = after field name")
		}
		p.advance()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		fields = append(fields, field{name: name.value, value: val})

		switch p.cur().kind {
		case tokenComma:
			p.advance()
			if p.cur().kind == tokenRBrace {
				p.advance()
				return structNode{fields: fields}, nil
			}
		case tokenRBrace:
			p.advance()
			return structNode{fields: fields}, nil
		default:
			return nil, p.errorf(p.cur(), "expected , or }")
		}
	}
}

// parseTupleBody parses the item sequence of a ZON tuple/array inside braces.
func (p *parser) parseTupleBody() (node, error) {
	var items []node
	for {
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		items = append(items, val)

		switch p.cur().kind {
		case tokenComma:
			p.advance()
			if p.cur().kind == tokenRBrace {
				p.advance()
				return tupleNode{items: items}, nil
			}
		case tokenRBrace:
			p.advance()
			return tupleNode{items: items}, nil
		default:
			return nil, p.errorf(p.cur(), "expected , or }")
		}
	}
}
