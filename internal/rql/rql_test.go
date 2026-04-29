package rql

import (
	"testing"
)

func TestLexerBasic(t *testing.T) {
	tests := []struct {
		input    string
		expected []TokenType
	}{
		{`SHOVE key "value"`, []TokenType{TOKEN_SHOVE, TOKEN_IDENT, TOKEN_STRING, TOKEN_EOF}},
		{`YOINK mykey`, []TokenType{TOKEN_YOINK, TOKEN_IDENT, TOKEN_EOF}},
		{`YEET mykey`, []TokenType{TOKEN_YEET, TOKEN_IDENT, TOKEN_EOF}},
		{`KEYS`, []TokenType{TOKEN_KEYS, TOKEN_EOF}},
		{`NUKE`, []TokenType{TOKEN_NUKE, TOKEN_EOF}},
		{`STATS`, []TokenType{TOKEN_STATS, TOKEN_EOF}},
		{`HELP`, []TokenType{TOKEN_HELP, TOKEN_EOF}},
		{`SHOVE k "v" TTL 60`, []TokenType{TOKEN_SHOVE, TOKEN_IDENT, TOKEN_STRING, TOKEN_TTL, TOKEN_NUMBER, TOKEN_EOF}},
		{`shove k "v"`, []TokenType{TOKEN_SHOVE, TOKEN_IDENT, TOKEN_STRING, TOKEN_EOF}}, // lowercase
	}

	for _, tt := range tests {
		lex := NewLexer(tt.input)
		tokens := lex.Tokenize()
		if len(tokens) != len(tt.expected) {
			t.Errorf("input %q: expected %d tokens, got %d", tt.input, len(tt.expected), len(tokens))
			continue
		}
		for i, tok := range tokens {
			if tok.Type != tt.expected[i] {
				t.Errorf("input %q token %d: expected %s, got %s (%q)", tt.input, i, tt.expected[i], tok.Type, tok.Literal)
			}
		}
	}
}

func TestParserShove(t *testing.T) {
	cmd, err := Parse(`SHOVE greeting "hello world"`)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Type != CMD_SHOVE {
		t.Fatalf("expected CMD_SHOVE, got %d", cmd.Type)
	}
	if cmd.Key != "greeting" {
		t.Fatalf("expected key 'greeting', got %q", cmd.Key)
	}
	if cmd.Value != "hello world" {
		t.Fatalf("expected value 'hello world', got %q", cmd.Value)
	}
}

func TestParserShoveTTL(t *testing.T) {
	cmd, err := Parse(`SHOVE token "abc123" TTL 3600`)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.TTL != 3600 {
		t.Fatalf("expected TTL 3600, got %d", cmd.TTL)
	}
}

func TestParserYoink(t *testing.T) {
	cmd, err := Parse(`YOINK greeting`)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Type != CMD_YOINK || cmd.Key != "greeting" {
		t.Fatalf("unexpected: %+v", cmd)
	}
}

func TestParserErrors(t *testing.T) {
	badInputs := []string{
		"",
		"SHOVE",            // missing key and value
		"SHOVE key",        // missing value
		"NOTACOMMAND foo",  // unknown command
	}

	for _, input := range badInputs {
		_, err := Parse(input)
		if err == nil {
			t.Errorf("expected error for %q, got nil", input)
		}
	}
}
