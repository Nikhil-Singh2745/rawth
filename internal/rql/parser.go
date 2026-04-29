package rql

import (
	"fmt"
	"strconv"
)

// the command types. i used iota because it makes me feel like a pro 
// even though i just found out about it last week.
type CommandType int

const (
	CMD_SHOVE CommandType = iota
	CMD_YOINK
	CMD_YEET
	CMD_PEEK
	CMD_KEYS
	CMD_NUKE
	CMD_STATS
	CMD_HELP
)

// this is the "AST". quotes because it's just a flat struct. 
// our language doesn't have nested loops or functions or any of that fancy stuff 
// so why bother with a tree? a struct is fine. future me, if you are adding 
// subqueries... just dont.
type Command struct {
	Type  CommandType
	Key   string
	Value string
	TTL   int64 // 0 means it lives forever. or until i reboot the server.
}

// the parser takes the tokens and tries to make sense of them. 
// it's "recursive descent" in name only because there's nowhere to descend to. 
// it's more like a "switch-case descent".
type Parser struct {
	tokens []Token
	pos    int
}

// the main entry point for turning strings into intentions
func Parse(input string) (*Command, error) {
	lexer := NewLexer(input)
	tokens := lexer.Tokenize()

	p := &Parser{tokens: tokens, pos: 0}
	return p.parseCommand()
}

// where we are right now
func (p *Parser) current() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

// move forward. slowly.
func (p *Parser) advance() Token {
	tok := p.current()
	p.pos++
	return tok
}

// if we dont see what we expect we complain. loud.
func (p *Parser) expect(t TokenType) (Token, error) {
	tok := p.current()
	if tok.Type != t {
		return tok, fmt.Errorf("expected %s, got %s (%q)", t, tok.Type, tok.Literal)
	}
	p.advance()
	return tok, nil
}

// what are we doing today?
func (p *Parser) parseCommand() (*Command, error) {
	tok := p.current()

	switch tok.Type {
	case TOKEN_SHOVE:
		return p.parseShove()
	case TOKEN_YOINK:
		return p.parseYoink()
	case TOKEN_YEET:
		return p.parseYeet()
	case TOKEN_PEEK:
		return p.parsePeek()
	case TOKEN_KEYS:
		return p.parseKeys()
	case TOKEN_NUKE:
		return p.parseNuke()
	case TOKEN_STATS:
		return p.parseStats()
	case TOKEN_HELP:
		return p.parseHelp()
	case TOKEN_EOF:
		return nil, fmt.Errorf("empty command")
	default:
		return nil, fmt.Errorf("unknown command: %q (try HELP)", tok.Literal)
	}
}

// SHOVE key "value" [TTL seconds]
func (p *Parser) parseShove() (*Command, error) {
	p.advance() 

	keyTok := p.advance()
	if keyTok.Type != TOKEN_IDENT && keyTok.Type != TOKEN_STRING {
		return nil, fmt.Errorf("SHOVE: expected key, got %s (%q)", keyTok.Type, keyTok.Literal)
	}

	valTok := p.advance()
	if valTok.Type != TOKEN_STRING && valTok.Type != TOKEN_IDENT && valTok.Type != TOKEN_NUMBER {
		return nil, fmt.Errorf("SHOVE: expected value, got %s (%q)", valTok.Type, valTok.Literal)
	}

	cmd := &Command{
		Type:  CMD_SHOVE,
		Key:   keyTok.Literal,
		Value: valTok.Literal,
	}

	// maybe they want it to die eventually
	if p.current().Type == TOKEN_TTL {
		p.advance() 
		ttlTok, err := p.expect(TOKEN_NUMBER)
		if err != nil {
			return nil, fmt.Errorf("SHOVE: TTL requires a number: %w", err)
		}
		ttl, err := strconv.ParseInt(ttlTok.Literal, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("SHOVE: invalid TTL value: %w", err)
		}
		cmd.TTL = ttl
	}

	return cmd, nil
}

// YOINK key
func (p *Parser) parseYoink() (*Command, error) {
	p.advance() 

	keyTok := p.advance()
	if keyTok.Type != TOKEN_IDENT && keyTok.Type != TOKEN_STRING {
		return nil, fmt.Errorf("YOINK: expected key, got %s (%q)", keyTok.Type, keyTok.Literal)
	}

	return &Command{
		Type: CMD_YOINK,
		Key:  keyTok.Literal,
	}, nil
}

// YEET key
func (p *Parser) parseYeet() (*Command, error) {
	p.advance() 

	keyTok := p.advance()
	if keyTok.Type != TOKEN_IDENT && keyTok.Type != TOKEN_STRING {
		return nil, fmt.Errorf("YEET: expected key, got %s (%q)", keyTok.Type, keyTok.Literal)
	}

	return &Command{
		Type: CMD_YEET,
		Key:  keyTok.Literal,
	}, nil
}

// PEEK key
func (p *Parser) parsePeek() (*Command, error) {
	p.advance() 

	keyTok := p.advance()
	if keyTok.Type != TOKEN_IDENT && keyTok.Type != TOKEN_STRING {
		return nil, fmt.Errorf("PEEK: expected key, got %s (%q)", keyTok.Type, keyTok.Literal)
	}

	return &Command{
		Type: CMD_PEEK,
		Key:  keyTok.Literal,
	}, nil
}

func (p *Parser) parseKeys() (*Command, error) {
	p.advance() 
	return &Command{Type: CMD_KEYS}, nil
}

func (p *Parser) parseNuke() (*Command, error) {
	p.advance() 
	return &Command{Type: CMD_NUKE}, nil
}

func (p *Parser) parseStats() (*Command, error) {
	p.advance() 
	return &Command{Type: CMD_STATS}, nil
}

func (p *Parser) parseHelp() (*Command, error) {
	p.advance() 
	return &Command{Type: CMD_HELP}, nil
}
