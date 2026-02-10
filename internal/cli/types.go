package cli

// Command represents a parsed command from the CLI
type Command struct {
	Type   string
	Params map[string]interface{}
}

// Token types for the lexer
const (
	// Keywords
	TokenLogin = iota
	TokenRegister
	TokenList
	TokenServices
	TokenShow
	TokenCreate
	TokenService
	TokenShutdown
	TokenStartup
	TokenRestart
	TokenUsers
	TokenDrop
	TokenUser
	TokenAlter
	TokenActive
	TokenAdmin
	TokenPassword
	TokenDataset
	TokenDatasets
	TokenOf
	TokenAgents
	TokenRole
	TokenRoles
	TokenDescription
	TokenGrant
	TokenRevoke
	TokenAll
	TokenPermission
	TokenTo
	TokenFrom
	TokenFor
	TokenResources
	TokenOn
	TokenSet
	TokenReset
	TokenVersion
	TokenVar
	TokenVars
	TokenConfigs
	TokenEnvs
	TokenKey
	TokenKeys
	TokenGenerate
	TokenModel
	TokenModels
	TokenProvider
	TokenProviders
	TokenDefault
	TokenChats
	TokenChat
	TokenFiles
	TokenAs
	TokenParse
	TokenImport
	TokenInto
	TokenWith
	TokenParser
	TokenPipeline
	TokenSearch
	TokenCurrent
	TokenLLM
	TokenVLM
	TokenEmbedding
	TokenReranker
	TokenASR
	TokenTTS
	TokenAsync
	TokenSync
	TokenBenchmark
	TokenPing

	// Literals
	TokenIdentifier
	TokenQuotedString
	TokenNumber

	// Special
	TokenSemicolon
	TokenComma
	TokenEOF
	TokenIllegal
)

// Token represents a lexical token
type Token struct {
	Type  int
	Value string
}

// NewCommand creates a new command with the given type
func NewCommand(cmdType string) *Command {
	return &Command{
		Type:   cmdType,
		Params: make(map[string]interface{}),
	}
}
