// Package internal provides shared internal helpers for core.
package internal

// Language represents the language for agent prompts.
type Language string

const (
	LanguageEnglish Language = "en"
	LanguageChinese Language = "zh"
)

var currentLanguage Language = LanguageEnglish

func SetLanguage(lang Language) { currentLanguage = lang }
func GetLanguage() Language     { return currentLanguage }

func GetPrompt(en, zh string) string {
	if currentLanguage == LanguageChinese {
		return zh
	}
	return en
}

var (
	DefaultSystemPrompt = GetPrompt(
		"You are a helpful assistant. Use available tools to accomplish tasks.",
		"你是一个有用的助手。使用可用工具完成任务。",
	)
	TransferPrompt = GetPrompt(
		"You can transfer to the following agents: ",
		"你可以转移到以下助手：",
	)
	ExitPrompt = GetPrompt(
		"Say 'FINISH' when the task is complete.",
		"完成任务后请说'完成'。",
	)
)
