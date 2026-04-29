package rql

// the lexer is basically a glorified string splitter but way more annoying 
// to write. it goes through the input byte by byte. 
// regex would be easier but everyone says regex is slow 
// and i want this db to be fast even if it only stores memes.
type Lexer struct {
	input   string
	pos     int  // where we are
	readPos int  // where we are going
	ch      byte // what we are looking at
}

// start the engine
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar() 
	return l
}

// move to the next byte. if we hit the end we set ch to 0
func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0 
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
}

// just a quick peek. no touching.
func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

// the main loop. it decides what kind of token we just found. 
// strings, numbers, words... it's like teaching a toddler to read.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	switch {
	case l.ch == 0:
		return Token{Type: TOKEN_EOF, Literal: ""}

	case l.ch == '"':
		return l.readString()

	case l.ch == '\'':
		return l.readSingleQuotedString()

	case isDigit(l.ch):
		return l.readNumber()

	case isLetter(l.ch) || l.ch == '_':
		return l.readIdentOrKeyword()

	default:
		tok := Token{Type: TOKEN_ILLEGAL, Literal: string(l.ch)}
		l.readChar()
		return tok
	}
}

// give me everything you got
func (l *Lexer) Tokenize() []Token {
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TOKEN_EOF {
			break
		}
	}
	return tokens
}

// reading strings is fun because of escape characters. 
// i only support basic escapes because i am lazy.
func (l *Lexer) readString() Token {
	l.readChar() 

	start := l.pos
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar() 
		}
		l.readChar()
	}

	literal := l.input[start:l.pos]

	if l.ch == '"' {
		l.readChar() 
	}

	return Token{Type: TOKEN_STRING, Literal: literal}
}

// same as double quotes but with single quotes. redundancy is the spice of life.
func (l *Lexer) readSingleQuotedString() Token {
	l.readChar() 

	start := l.pos
	for l.ch != '\'' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar() 
		}
		l.readChar()
	}

	literal := l.input[start:l.pos]

	if l.ch == '\'' {
		l.readChar() 
	}

	return Token{Type: TOKEN_STRING, Literal: literal}
}

func (l *Lexer) readNumber() Token {
	start := l.pos
	for isDigit(l.ch) {
		l.readChar()
	}
	return Token{Type: TOKEN_NUMBER, Literal: l.input[start:l.pos]}
}

// keywords or just names. we check against a map later to see if it's special.
func (l *Lexer) readIdentOrKeyword() Token {
	start := l.pos
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' || l.ch == '-' || l.ch == '.' || l.ch == ':' {
		l.readChar()
	}

	literal := l.input[start:l.pos]
	tokenType := LookupIdent(literal)

	return Token{Type: tokenType, Literal: literal}
}

// ignore the empty space. like my brain on a monday morning.
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' || l.ch == '\n' {
		l.readChar()
	}
}

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}
