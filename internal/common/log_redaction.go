package common

import (
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const redactedLogValue = "[REDACTED]"

// Redact before encoding so Logger, Sugar, child loggers, and every sink share
// the same policy. Opaque user payloads must be omitted at their call sites.
type redactingLogCore struct {
	zapcore.Core
}

func (c redactingLogCore) With(fields []zapcore.Field) zapcore.Core {
	return redactingLogCore{Core: c.Core.With(redactLogFields(fields))}
}

func (c redactingLogCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c redactingLogCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	entry.Message = RedactLogText(entry.Message)
	entry.Stack = RedactLogText(entry.Stack)
	return c.Core.Write(entry, redactLogFields(fields))
}

func redactLogFields(fields []zapcore.Field) []zapcore.Field {
	result := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		sensitive := IsSensitiveLogKey(field.Key)
		if sensitive || isPrivateLogPayloadKey(field.Key) {
			// Numeric fields named e.g. "messages" are counts, not payloads.
			if sensitive || field.Type == zapcore.StringType || field.Interface != nil {
				result = append(result, zap.String(field.Key, redactedLogValue))
				continue
			}
		}
		switch field.Type {
		case zapcore.StringType:
			field.String = RedactLogText(field.String)
			result = append(result, field)
		case zapcore.ByteStringType:
			result = append(result, zap.String(field.Key, RedactLogText(string(field.Interface.([]byte)))))
		case zapcore.BinaryType:
			result = append(result, zap.String(field.Key, redactedLogValue))
		case zapcore.ReflectType, zapcore.ObjectMarshalerType, zapcore.ArrayMarshalerType, zapcore.StringerType, zapcore.ErrorType:
			encoder := zapcore.NewMapObjectEncoder()
			field.AddTo(encoder)
			data, err := json.Marshal(encoder.Fields)
			if err != nil {
				result = append(result, zap.String(field.Key, redactedLogValue))
				continue
			}
			var value map[string]any
			decoder := json.NewDecoder(strings.NewReader(string(data)))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				result = append(result, zap.String(field.Key, redactedLogValue))
				continue
			}
			redactLogValue(value)
			for key, child := range value {
				result = append(result, zap.Any(key, child))
			}
		default:
			result = append(result, field)
		}
	}
	return result
}

var (
	logURLPattern        = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s<>"']+`)
	logEmailPattern      = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9-]+(?:\.[a-z0-9-]+)+`)
	logAuthPattern       = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[a-z0-9._~+/=-]+`)
	logCredentialPattern = regexp.MustCompile(`(?i)(\b(?:[a-z0-9_.-]*(?:password|passwd|secret|token|api[_-]?key|app[_-]?key|access[_-]?key|private[_-]?key)|authorization|cookie|set-cookie|credential|username|user_name|email|otp|dsn)\b["']?\s*[:=]\s*)(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s,;}&]+)`)
	logPrivateKeyPattern = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`)
	logTokenPattern      = regexp.MustCompile(`\b(?:eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+|(?:sk-|ghp_|github_pat_|xox[baprs]-)[a-zA-Z0-9_-]+|AKIA[A-Z0-9]{16})\b`)
	logDataURIPattern    = regexp.MustCompile(`(?i)data:[^\s"']+`)
	logHeaderPattern     = regexp.MustCompile(`(?im)\b(?:authorization|proxy-authorization|cookie|set-cookie)\s*[:=]\s*[^\r\n]+`)
	logSQLPattern        = regexp.MustCompile(`(?is)\b(?:SQL\s*[:=]|query:\s*(?:select|insert|update|delete)\b).*`)
	logBodyPattern       = regexp.MustCompile(`(?is)(\b(?:request[_ -]?body|response[_ -]?body|body|payload)\s*[:=]\s*).*`)
	logKeyNormalizer     = strings.NewReplacer("-", "", "_", "", ".", "", " ", "")
)

// RedactLogText removes recognizable credentials and email addresses from log
// text. Arbitrary user payloads must still be omitted instead of relying on it.
func RedactLogText(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(value))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err == nil {
			// Only accept a complete JSON value; do not silently lose trailing
			// diagnostics when a message starts with a JSON fragment.
			if decoder.Decode(new(any)) == io.EOF {
				redactLogValue(decoded)
				if data, err := json.Marshal(decoded); err == nil {
					return string(data)
				}
			}
		}
	}
	value = logPrivateKeyPattern.ReplaceAllString(value, redactedLogValue)
	value = logDataURIPattern.ReplaceAllString(value, redactedLogValue)
	value = logHeaderPattern.ReplaceAllString(value, redactedLogValue)
	value = logSQLPattern.ReplaceAllString(value, redactedLogValue)
	value = logBodyPattern.ReplaceAllString(value, "${1}"+redactedLogValue)
	value = logURLPattern.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return redactedLogValue
		}
		// Paths and query strings can carry signed URLs or credentials too.
		return parsed.Scheme + "://" + parsed.Host
	})
	value = logAuthPattern.ReplaceAllString(value, "$1 "+redactedLogValue)
	value = logCredentialPattern.ReplaceAllString(value, "${1}\""+redactedLogValue+"\"")
	value = logTokenPattern.ReplaceAllString(value, redactedLogValue)
	return logEmailPattern.ReplaceAllString(value, redactedLogValue)
}

func isPrivateLogPayloadKey(key string) bool {
	normalized := normalizeLogKey(key)
	for _, suffix := range []string{"question", "questions", "query", "prompt", "content", "snippet", "preview"} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	switch normalized {
	case "embedding", "embeddings", "vector", "vectors", "documents", "texts", "thinking", "completion",
		"generatedtext", "transcript", "transcription", "b64json", "body", "payload", "request", "response",
		"requestbody", "responsebody", "raw", "text", "messages", "message", "answer", "reasoning", "arguments",
		"input", "output", "inputs", "outputs", "tooloutput", "stdout", "stderr", "httpresult", "structuredresult",
		"resolvedvalue", "rawresult", "firsthit", "hit", "result", "results", "sampleraw", "sampleresolved", "dsl",
		"sql", "whereclause", "filter", "requestfilter", "searchfilters", "promptconfigsystem",
		"promptconfigparameters", "param", "params", "value", "newvalue", "userinput", "contexts", "keywords",
		"config", "requestgenerationconfig":
		return true
	default:
		return false
	}
}

func normalizeLogKey(key string) string {
	return logKeyNormalizer.Replace(strings.ToLower(key))
}

func redactLogValue(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if IsSensitiveLogKey(key) || isPrivateLogPayloadKey(key) {
				value[key] = redactedLogValue
				continue
			}
			redactLogValue(child)
			if text, ok := child.(string); ok {
				value[key] = RedactLogText(text)
			}
		}
	case []any:
		for i, child := range value {
			redactLogValue(child)
			if text, ok := child.(string); ok {
				value[i] = RedactLogText(text)
			}
		}
	}
}

// IsSensitiveLogKey identifies credential and personal-identity fields in logs.
func IsSensitiveLogKey(key string) bool {
	normalized := normalizeLogKey(key)
	for _, suffix := range []string{"password", "passwordhash", "passwd", "secret", "secretkey", "apikey", "appkey", "token", "accesskey", "accesskeyid", "privatekey", "clientsecret", "username", "email", "nickname"} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	switch normalized {
	case "authorization", "proxyauthorization", "key", "credentials", "credential", "cookie", "setcookie",
		"secretid", "mail", "user", "login", "mailfromaddress", "mailfromname", "to", "from", "otp", "resetcode",
		"verificationcode", "dsn", "connectionstring", "serviceaccountjson", "serviceaccountkey", "sshkey":
		return true
	default:
		return false
	}
}
