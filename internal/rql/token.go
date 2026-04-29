package rql

// Token types for the Rawth Query Language.
// We invented this language. There are no standards to follow.
// There are no rules. Only SHOVE and YOINK.

// TokenType represents the type of a lexical token.
type TokenType int

const (
	// Commands — the verbs of RQL
	TOKEN_SHOVE TokenType = iota // SET equivalent
	TOKEN_YOINK                  // GET equivalent
	TOKEN_YEET                   // DELETE equivalent
	TOKEN_PEEK                   // EXISTS check
	TOKEN_KEYS                   // List all keys
	TOKEN_NUKE                   // Drop everything
	TOKEN_STATS                  // Database statistics
	TOKEN_HELP                   // Show help

	// Modifiers
	TOKEN_TTL // TTL keyword for expiration

	// Literals
	TOKEN_STRING // Quoted string: "hello world"
	TOKEN_IDENT  // Unquoted identifier: my_key
	TOKEN_NUMBER // Numeric literal: 60

	// Special
	TOKEN_EOF     // End of input
	TOKEN_ILLEGAL // Something we don't understand
)

// Token is a single lexical unit in an RQL command.
type Token struct {
	Type    TokenType
	Literal string
}

// String returns a human-readable name for a token type.
func (t TokenType) String() string {
	switch t {
	case TOKEN_SHOVE:
		return "SHOVE"
	case TOKEN_YOINK:
		return "YOINK"
	case TOKEN_YEET:
		return "YEET"
	case TOKEN_PEEK:
		return "PEEK"
	case TOKEN_KEYS:
		return "KEYS"
	case TOKEN_NUKE:
		return "NUKE"
	case TOKEN_STATS:
		return "STATS"
	case TOKEN_HELP:
		return "HELP"
	case TOKEN_TTL:
		return "TTL"
	case TOKEN_STRING:
		return "STRING"
	case TOKEN_IDENT:
		return "IDENT"
	case TOKEN_NUMBER:
		return "NUMBER"
	case TOKEN_EOF:
		return "EOF"
	case TOKEN_ILLEGAL:
		return "ILLEGAL"
	default:
		return "UNKNOWN"
	}
}

// keywords maps keyword strings to their token types.
var keywords = map[string]TokenType{
	"SHOVE": TOKEN_SHOVE,
	"shove": TOKEN_SHOVE,
	"YOINK": TOKEN_YOINK,
	"yoink": TOKEN_YOINK,
	"YEET":  TOKEN_YEET,
	"yeet":  TOKEN_YEET,
	"PEEK":  TOKEN_PEEK,
	"peek":  TOKEN_PEEK,
	"KEYS":  TOKEN_KEYS,
	"keys":  TOKEN_KEYS,
	"NUKE":  TOKEN_NUKE,
	"nuke":  TOKEN_NUKE,
	"STATS": TOKEN_STATS,
	"stats": TOKEN_STATS,
	"HELP":  TOKEN_HELP,
	"help":  TOKEN_HELP,
	"TTL":   TOKEN_TTL,
	"ttl":   TOKEN_TTL,
}

// LookupIdent checks if an identifier is actually a keyword.
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOKEN_IDENT
}
