package prompts

const postPrompt = "Please deploy my application with Defang now."

func getStringArg(args map[string]string, key, defaultValue string) string {
	if val, exists := args[key]; exists {
		return val
	}
	return defaultValue
}
