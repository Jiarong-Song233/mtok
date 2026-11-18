package model

// builtinAliases maps canonical display names to every raw identifier spelling
// observed in local agent logs. Keys are folded with Key, so "glm-5.3",
// "GLM_5_3" and "glm53" are equivalent. Users extend this from
// ~/.config/mtok/aliases.yaml without recompiling.
var builtinAliases = map[string][]string{
	// ---- DeepSeek ----
	"DeepSeek-V4.1-Flash": {
		"deepseek-v4.1-flash", "deepseek-v4-1-flash", "deepseekv41flash",
		"deepseek~4~1~flash", "deepseek41flash", "auto-deepseek-v4-1-flash",
		"deepseek-v4.1-flash-free", "deepseek-v4.1-flash-max",
		"sn-deepseek-v4-1-flash-1017", "deepseek-v4.1-flash-1017",
		// deepseek-flash 是同一底模的短名（Magpie req 字段常见）
		"deepseek-flash", "deepseekflash", "dfmodel", "auto-deepseek-flash",
	},
	"DeepSeek-V4-Pro": {"deepseek-v4-pro", "deepseekv4pro", "dmodel", "auto-deepseek-v4-pro"},
	"DeepSeek-V4":     {"deepseek-v4", "deepseek-chat", "deepseek-reasoner"},

	// ---- GLM / Zhipu ----
	"GLM-5.3":        {"glm-5.3", "glm-5-3", "glm-5.3-max", "glm53", "gmodel", "sn-glm-5-3", "auto-glm-5.3"},
	"GLM-5.3-Flash":  {"glm-5.3-flash", "glm-5-3-flash", "glm-5.3-flash-max", "glm53flash", "gfmodel", "sn-glm-5-3-flash"},
	"GLM-5.3-FlashX": {"glm-5.3-flashx", "glm-5.3-flashx-max"},
	"GLM-5.2":        {"glm-5.2", "glm-5-2"},
	"GLM-5.1":        {"glm-5.1"},
	"GLM-5":          {"glm-5"},
	"GLM-4.6":        {"glm-4.6"},

	// ---- Qwen ----
	"Qwen3.8-Flash":     {"qwen3.8-flash", "qwen3-8-flash", "qwen3.8-flash-max", "qfmodel", "auto-qwen3-8-flash"},
	"Qwen3.8-Max":       {"qwen3.8-max", "qwen3-8-max", "qwen3.8-max-max", "qmodel_38max", "auto-qwen3-8-max"},
	"Qwen4-Max-Preview": {"qmodel_latest"},
	"Qoder-Fast":        {"fast"},

	// ---- Kimi / Moonshot ----
	"Kimi-K3.1-Pro":     {"kimi-k3.1-pro", "kimi-k3-1-pro", "kmodel_latest", "auto-kimi-k3.1-pro"},
	"Kimi-K3":           {"kimi-k3", "kimi-k3-1", "sn-kimi-k3"},
	"Kimi-K2.8-Preview": {"kimi-k2.8-preview", "kimi-k2-8-preview", "kmodel"},
	"Kimi-K2.6":         {"kimi-k2.6"},
	"Kimi-For-Coding":   {"kimi-for-coding", "kimi-code/kimi-for-coding"},

	// ---- MiniMax / Step / MiMo / SenseNova ----
	"MiniMax-M3.1-Flash":       {"minimax-m3.1-flash", "mmodel", "auto-minimax-m3.1-flash"},
	"Step-5-Preview":           {"step-5-preview", "jmodel"},
	"Xiaomi-MiMo-V2.6-Pro":     {"mimo-v2.6-pro", "mimo-v2-6-pro"},
	"SenseNova-6.8-Flash":      {"sn-sensenova-6-8-flash", "sensenova-6.8-flash"},
	"SenseNova-6.8-Flash-Lite": {"sn-sensenova-6-8-flash-lite", "sensenova-6.8-flash-lite"},

	// ---- OpenAI / Codex ----
	"GPT-6.1-Sol":   {"gpt-6.1-sol", "gpt-6-1-sol"},
	"GPT-6.1-Terra": {"gpt-6.1-terra", "gpt-6-1-terra"},
	"GPT-6-Sol":     {"gpt-6-sol"},
	"GPT-6-Luna":    {"gpt-6-luna"},
	"GPT-6-Astra":   {"gpt-6-astra"},
	"GPT-5.6-Sol":   {"gpt-5.6-sol"},
	"GPT-5.6-Luna":  {"gpt-5.6-luna"},
	"GPT-5.6-Terra": {"gpt-5.6-terra"},
	"GPT-5.5":       {"gpt-5.5"},
	"GPT-5.4":       {"gpt-5.4"},
	"GPT-5.4-Mini":  {"gpt-5.4-mini", "gpt-5.4-mini-fast"},
	"GPT-5.4-Nano":  {"gpt-5.4-nano"},
	"GPT-5.3-Codex": {"gpt-5.3-codex"},
	"GPT-Reserve":   {"gpt-reserve"},

	// ---- Anthropic ----
	"Claude-Opus-5.5":   {"claude-opus-5-5", "claude-opus-5.5", "opus-5-5"},
	"Claude-Opus-5":     {"claude-opus-5", "opus-5"},
	"Claude-Opus-4.8":   {"claude-opus-4-8", "claude-opus-4.8"},
	"Claude-Opus-4":     {"claude-opus-4", "claude opus 4"},
	"Claude-Opus-4.6":   {"claude-opus-4-6"},
	"Claude-Sonnet-5.5": {"claude-sonnet-5-5", "claude-sonnet-5.5"},
	"Claude-Haiku-5.5":  {"claude-haiku-5-5"},
	"Claude-Fable-5.2":  {"claude-fable-5-2", "claude-fable-5.2"},
	"Claude-Fable-5.1":  {"claude-fable-5-1"},
	"Claude-Haiku-4.5":  {"claude-haiku-4-5", "claude-haiku-4.5"},

	// ---- OpenAI 旧版 / 快照模型（magpie served 里会出现带日期后缀的名字）----
	"GPT-4.1":      {"gpt-4.1", "gpt-41"},
	"GPT-4.1-Mini": {"gpt-4.1-mini"},

	// ---- Google ----
	"Gemini-3.8-Flash": {
		"gemini-3.8-flash", "gemini-3-8-flash", "gemini-3.8-flash-low",
		"gemini-3.8-flash-medium", "gemini-3.8-flash-high", "gemini-3.8-flash-tiered",
		"gemini 3.8 flash",
	},
	"Gemini-3-Pro":      {"gemini-3-pro", "gemini-3.0-pro", "gemini-3-pro-preview"},
	"Gemini-4-Carbon":   {"gemini-4-carbon"},
	"Gemini-4-Hydrogen": {"gemini-4-hydrogen"},
	"Gemini-4-Argon":    {"gemini-4-argon"},

	// ---- xAI ----
	"Grok-4.7": {"grok-4.7", "grok-4-7", "grok-code-fast-1"},
	"Grok-4.6": {"grok-4.6"},

	// ---- 国内其它 ----
	"Hy4":          {"hy4", "hy4free", "hy4std"},
	"Hy4-Flash":    {"hy4-flash", "hy4flash", "hy4-flash-free", "hy4flash-free", "hy-4-flash-free", "hy-4-flash"},
	"Hy4-Preview":  {"hy4-preview"},
	"Hy3":          {"hy3"},
	"Hy3-X":        {"hy3-x"},
	"Space-Bunny":  {"space-bunny"},
	"Composer-2.5": {"composer-2.5"},
	"Muse-Wonder":  {"muse-wonder-1.0"},

	// ---- 代理/聚合别名（无底层模型信息）----
	"Raccoon-Work":          {"raccoon-090dd3", "raccoon-work"},
	"Raccoon-Work-260817-A": {"raccoon-19b265"},
	"Raccoon-Work-260817-B": {"raccoon-405a1c"},
	"Auto(未指明)":             {"auto", "auto-auto", "autoauto", "group-auto-auto", "magpie/auto/auto", "auto/auto"},
	"Unknown":               {"unknown"},
}
