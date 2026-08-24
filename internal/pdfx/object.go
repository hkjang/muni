// Package pdfx extracts structured text, headings, lists and images from PDF
// files so they can be imported as muni documents.
package pdfx

import (
	"errors"
	"strconv"
	"strings"
)

type Name string

type Ref struct {
	Number     int
	Generation int
}

type Dict map[Name]Object

type Array []Object

type String []byte

type Stream struct {
	Dict Dict
	Raw  []byte
	ref  Ref
}

// Object is one of: nil, bool, int64, float64, Name, String, Array, Dict, *Stream, Ref.
type Object any

func (d Dict) get(keys ...Name) Object {
	for _, key := range keys {
		if value, ok := d[key]; ok {
			return value
		}
	}
	return nil
}

const (
	tokenEOF = iota
	tokenNumber
	tokenName
	tokenString
	tokenKeyword
	tokenArrayOpen
	tokenArrayClose
	tokenDictOpen
	tokenDictClose
	tokenBraceOpen
	tokenBraceClose
)

type token struct {
	kind    int
	text    string
	number  float64
	integer int64
	isInt   bool
	bytes   []byte
}

type lexer struct {
	data []byte
	pos  int
}

func isWhitespace(c byte) bool {
	return c == 0x00 || c == 0x09 || c == 0x0a || c == 0x0c || c == 0x0d || c == 0x20
}

func isDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isWhitespace(c) {
			l.pos++
			continue
		}
		if c == '%' {
			for l.pos < len(l.data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
				l.pos++
			}
			continue
		}
		return
	}
}

func (l *lexer) next() token {
	// Stray delimiters are skipped in place rather than by recursing, so a run
	// of malformed bytes cannot exhaust the stack.
	for {
		l.skipSpace()
		if l.pos >= len(l.data) {
			return token{kind: tokenEOF}
		}
		c := l.data[l.pos]
		switch {
		case c == '[':
			l.pos++
			return token{kind: tokenArrayOpen}
		case c == ']':
			l.pos++
			return token{kind: tokenArrayClose}
		case c == '{':
			l.pos++
			return token{kind: tokenBraceOpen}
		case c == '}':
			l.pos++
			return token{kind: tokenBraceClose}
		case c == '<':
			if l.pos+1 < len(l.data) && l.data[l.pos+1] == '<' {
				l.pos += 2
				return token{kind: tokenDictOpen}
			}
			return l.hexString()
		case c == '>':
			if l.pos+1 < len(l.data) && l.data[l.pos+1] == '>' {
				l.pos += 2
				return token{kind: tokenDictClose}
			}
			l.pos++
		case c == '(':
			return l.literalString()
		case c == '/':
			return l.name()
		case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
			return l.number()
		case c == ')':
			l.pos++
		default:
			return l.keyword()
		}
	}
}

func (l *lexer) name() token {
	l.pos++
	var out strings.Builder
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isWhitespace(c) || isDelimiter(c) {
			break
		}
		if c == '#' && l.pos+2 < len(l.data) {
			if value, err := strconv.ParseUint(string(l.data[l.pos+1:l.pos+3]), 16, 8); err == nil {
				out.WriteByte(byte(value))
				l.pos += 3
				continue
			}
		}
		out.WriteByte(c)
		l.pos++
	}
	return token{kind: tokenName, text: out.String()}
}

func (l *lexer) number() token {
	start := l.pos
	if l.data[l.pos] == '+' || l.data[l.pos] == '-' {
		l.pos++
	}
	isInt := true
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if c >= '0' && c <= '9' {
			l.pos++
			continue
		}
		if c == '.' || c == '-' || c == '+' || c == 'e' || c == 'E' {
			isInt = false
			l.pos++
			continue
		}
		break
	}
	text := string(l.data[start:l.pos])
	if isInt {
		value, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return token{kind: tokenNumber, text: text, number: float64(value), integer: value, isInt: true}
		}
	}
	value, _ := strconv.ParseFloat(strings.TrimSuffix(text, "."), 64)
	return token{kind: tokenNumber, text: text, number: value, integer: int64(value)}
}

func (l *lexer) keyword() token {
	start := l.pos
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		if isWhitespace(c) || isDelimiter(c) {
			break
		}
		l.pos++
	}
	if l.pos == start {
		l.pos++
	}
	return token{kind: tokenKeyword, text: string(l.data[start:l.pos])}
}

func (l *lexer) hexString() token {
	l.pos++ // consume '<'
	out := make([]byte, 0, 32)
	var high byte
	haveHigh := false
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		l.pos++
		if c == '>' {
			break
		}
		value, ok := hexValue(c)
		if !ok {
			continue
		}
		if haveHigh {
			out = append(out, high<<4|value)
			haveHigh = false
		} else {
			high = value
			haveHigh = true
		}
	}
	if haveHigh {
		out = append(out, high<<4)
	}
	return token{kind: tokenString, bytes: out}
}

func hexValue(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

func (l *lexer) literalString() token {
	l.pos++ // consume '('
	out := make([]byte, 0, 32)
	depth := 1
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		l.pos++
		switch c {
		case '\\':
			if l.pos >= len(l.data) {
				break
			}
			escape := l.data[l.pos]
			l.pos++
			switch escape {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '(', ')', '\\':
				out = append(out, escape)
			case '\r':
				if l.pos < len(l.data) && l.data[l.pos] == '\n' {
					l.pos++
				}
			case '\n':
				// Line continuation inside a literal string.
			default:
				if escape >= '0' && escape <= '7' {
					value := int(escape - '0')
					for count := 0; count < 2 && l.pos < len(l.data); count++ {
						digit := l.data[l.pos]
						if digit < '0' || digit > '7' {
							break
						}
						value = value*8 + int(digit-'0')
						l.pos++
					}
					out = append(out, byte(value))
				} else {
					out = append(out, escape)
				}
			}
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return token{kind: tokenString, bytes: out}
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return token{kind: tokenString, bytes: out}
}

type parser struct {
	lexer
	peeked []token
}

func newParser(data []byte, offset int) *parser {
	return &parser{lexer: lexer{data: data, pos: offset}}
}

func (p *parser) read() token {
	if len(p.peeked) > 0 {
		value := p.peeked[0]
		p.peeked = p.peeked[1:]
		return value
	}
	return p.next()
}

func (p *parser) unread(values ...token) {
	p.peeked = append(append([]token{}, values...), p.peeked...)
}

var errParse = errors.New("PDF 구조를 해석하지 못했습니다")

// object parses one object, resolving the "N G R" indirect-reference form that
// only becomes recognisable after two numbers and a keyword.
func (p *parser) object(depth int) (Object, error) {
	if depth > 64 {
		return nil, errParse
	}
	current := p.read()
	switch current.kind {
	case tokenEOF:
		return nil, errParse
	case tokenNumber:
		if current.isInt {
			second := p.read()
			if second.kind == tokenNumber && second.isInt {
				third := p.read()
				if third.kind == tokenKeyword && third.text == "R" {
					return Ref{Number: int(current.integer), Generation: int(second.integer)}, nil
				}
				p.unread(second, third)
			} else {
				p.unread(second)
			}
			return current.integer, nil
		}
		return current.number, nil
	case tokenName:
		return Name(current.text), nil
	case tokenString:
		return String(current.bytes), nil
	case tokenArrayOpen:
		array := Array{}
		for {
			item := p.read()
			if item.kind == tokenArrayClose || item.kind == tokenEOF {
				return array, nil
			}
			p.unread(item)
			value, err := p.object(depth + 1)
			if err != nil {
				return array, nil
			}
			array = append(array, value)
		}
	case tokenDictOpen:
		dict := Dict{}
		for {
			key := p.read()
			if key.kind == tokenDictClose || key.kind == tokenEOF {
				return dict, nil
			}
			if key.kind != tokenName {
				continue
			}
			value, err := p.object(depth + 1)
			if err != nil {
				return dict, nil
			}
			dict[Name(key.text)] = value
		}
	case tokenKeyword:
		switch current.text {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null":
			return nil, nil
		}
		return Name(current.text), nil
	}
	return nil, errParse
}

func toFloat(value Object) (float64, bool) {
	switch typed := value.(type) {
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	}
	return 0, false
}

func toInt(value Object) (int, bool) {
	number, ok := toFloat(value)
	return int(number), ok
}
