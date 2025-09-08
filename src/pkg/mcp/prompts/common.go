package prompts

const postPrompt = "Can you deploy my application now."

func getStringArg(args map[string]string, key, defaultValue string) string {
	if val, exists := args[key]; exists {
		return val
	}
	return defaultValue
}
