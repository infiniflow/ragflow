package cli

import (
	"strings"
	"unicode"
)

// Lexer performs lexical analysis of the input
type Lexer struct {
	input   string
	pos     int
	readPos int
	ch      byte
}

// NewLexer creates a new lexer for the given input
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
}

func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

// NextToken returns the next token from the input
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	switch l.ch {
	case ';':
		tok = newToken(TokenSemicolon, l.ch)
		l.readChar()
	case ',':
		tok = newToken(TokenComma, l.ch)
		l.readChar()
	case '\'':
		tok.Type = TokenQuotedString
		tok.Value = l.readQuotedString('\'')
	case '"':
		tok.Type = TokenQuotedString
		tok.Value = l.readQuotedString('"')
	case '\\':
		// Meta command: backslash followed by command name
		tok.Type = TokenIdentifier
		tok.Value = l.readMetaCommand()
	case 0:
		tok.Type = TokenEOF
		tok.Value = ""
	default:
		if isLetter(l.ch) {
			ident := l.readIdentifier()
			return l.lookupIdent(ident)
		} else if isDigit(l.ch) {
			tok.Type = TokenNumber
			tok.Value = l.readNumber()
			return tok
		} else {
			tok = newToken(TokenIllegal, l.ch)
			l.readChar()
		}
	}

	return tok
}

func (l *Lexer) readMetaCommand() string {
	start := l.pos
	l.readChar() // consume backslash
	for isLetter(l.ch) || l.ch == '?' {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func newToken(tokenType int, ch byte) Token {
	return Token{Type: tokenType, Value: string(ch)}
}

func (l *Lexer) readIdentifier() string {
	start := l.pos
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' || l.ch == '-' || l.ch == '.' {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func (l *Lexer) readNumber() string {
	start := l.pos
	for isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func (l *Lexer) readQuotedString(quote byte) string {
	l.readChar() // skip opening quote
	start := l.pos
	for l.ch != quote && l.ch != 0 {
		l.readChar()
	}
	str := l.input[start:l.pos]
	if l.ch == quote {
		l.readChar() // skip closing quote
	}
	return str
}

func (l *Lexer) lookupIdent(ident string) Token {
	upper := strings.ToUpper(ident)
	switch upper {
	case "LOGIN":
		return Token{Type: TokenLogin, Value: ident}
	case "REGISTER":
		return Token{Type: TokenRegister, Value: ident}
	case "LIST":
		return Token{Type: TokenList, Value: ident}
	case "SERVICES":
		return Token{Type: TokenServices, Value: ident}
	case "SHOW":
		return Token{Type: TokenShow, Value: ident}
	case "CREATE":
		return Token{Type: TokenCreate, Value: ident}
	case "SERVICE":
		return Token{Type: TokenService, Value: ident}
	case "SHUTDOWN":
		return Token{Type: TokenShutdown, Value: ident}
	case "STARTUP":
		return Token{Type: TokenStartup, Value: ident}
	case "RESTART":
		return Token{Type: TokenRestart, Value: ident}
	case "USERS":
		return Token{Type: TokenUsers, Value: ident}
	case "DROP":
		return Token{Type: TokenDrop, Value: ident}
	case "USER":
		return Token{Type: TokenUser, Value: ident}
	case "ALTER":
		return Token{Type: TokenAlter, Value: ident}
	case "ACTIVE":
		return Token{Type: TokenActive, Value: ident}
	case "ADMIN":
		return Token{Type: TokenAdmin, Value: ident}
	case "PASSWORD":
		return Token{Type: TokenPassword, Value: ident}
	case "DATASET":
		return Token{Type: TokenDataset, Value: ident}
	case "DATASETS":
		return Token{Type: TokenDatasets, Value: ident}
	case "OF":
		return Token{Type: TokenOf, Value: ident}
	case "AGENTS":
		return Token{Type: TokenAgents, Value: ident}
	case "ROLE":
		return Token{Type: TokenRole, Value: ident}
	case "ROLES":
		return Token{Type: TokenRoles, Value: ident}
	case "DESCRIPTION":
		return Token{Type: TokenDescription, Value: ident}
	case "GRANT":
		return Token{Type: TokenGrant, Value: ident}
	case "REVOKE":
		return Token{Type: TokenRevoke, Value: ident}
	case "ALL":
		return Token{Type: TokenAll, Value: ident}
	case "PERMISSION":
		return Token{Type: TokenPermission, Value: ident}
	case "TO":
		return Token{Type: TokenTo, Value: ident}
	case "FROM":
		return Token{Type: TokenFrom, Value: ident}
	case "FOR":
		return Token{Type: TokenFor, Value: ident}
	case "RESOURCES":
		return Token{Type: TokenResources, Value: ident}
	case "ON":
		return Token{Type: TokenOn, Value: ident}
	case "SET":
		return Token{Type: TokenSet, Value: ident}
	case "RESET":
		return Token{Type: TokenReset, Value: ident}
	case "VERSION":
		return Token{Type: TokenVersion, Value: ident}
	case "VAR":
		return Token{Type: TokenVar, Value: ident}
	case "VARS":
		return Token{Type: TokenVars, Value: ident}
	case "CONFIGS":
		return Token{Type: TokenConfigs, Value: ident}
	case "ENVS":
		return Token{Type: TokenEnvs, Value: ident}
	case "KEY":
		return Token{Type: TokenKey, Value: ident}
	case "KEYS":
		return Token{Type: TokenKeys, Value: ident}
	case "GENERATE":
		return Token{Type: TokenGenerate, Value: ident}
	case "MODEL":
		return Token{Type: TokenModel, Value: ident}
	case "MODELS":
		return Token{Type: TokenModels, Value: ident}
	case "PROVIDER":
		return Token{Type: TokenProvider, Value: ident}
	case "PROVIDERS":
		return Token{Type: TokenProviders, Value: ident}
	case "DEFAULT":
		return Token{Type: TokenDefault, Value: ident}
	case "CHATS":
		return Token{Type: TokenChats, Value: ident}
	case "CHAT":
		return Token{Type: TokenChat, Value: ident}
	case "FILES":
		return Token{Type: TokenFiles, Value: ident}
	case "AS":
		return Token{Type: TokenAs, Value: ident}
	case "PARSE":
		return Token{Type: TokenParse, Value: ident}
	case "IMPORT":
		return Token{Type: TokenImport, Value: ident}
	case "INTO":
		return Token{Type: TokenInto, Value: ident}
	case "WITH":
		return Token{Type: TokenWith, Value: ident}
	case "PARSER":
		return Token{Type: TokenParser, Value: ident}
	case "PIPELINE":
		return Token{Type: TokenPipeline, Value: ident}
	case "SEARCH":
		return Token{Type: TokenSearch, Value: ident}
	case "CURRENT":
		return Token{Type: TokenCurrent, Value: ident}
	case "LLM":
		return Token{Type: TokenLLM, Value: ident}
	case "VLM":
		return Token{Type: TokenVLM, Value: ident}
	case "EMBEDDING":
		return Token{Type: TokenEmbedding, Value: ident}
	case "RERANKER":
		return Token{Type: TokenReranker, Value: ident}
	case "ASR":
		return Token{Type: TokenASR, Value: ident}
	case "TTS":
		return Token{Type: TokenTTS, Value: ident}
	case "ASYNC":
		return Token{Type: TokenAsync, Value: ident}
	case "SYNC":
		return Token{Type: TokenSync, Value: ident}
	case "BENCHMARK":
		return Token{Type: TokenBenchmark, Value: ident}
	case "PING":
		return Token{Type: TokenPing, Value: ident}
	default:
		return Token{Type: TokenIdentifier, Value: ident}
	}
}

func isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch))
}

func isDigit(ch byte) bool {
	return unicode.IsDigit(rune(ch))
}
