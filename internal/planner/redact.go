package planner

import "strings"

const RedactedEnvValue = "<redacted>"

var sensitiveEnvSubstrings = []string{
	"PASSWORD",
	"SECRET",
	"TOKEN",
	"API_KEY",
	"ACCESS_KEY",
	"PRIVATE_KEY",
	"CREDENTIAL",
	"AUTH",
}

func IsSensitiveEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, sub := range sensitiveEnvSubstrings {
		if strings.Contains(upper, sub) {
			return true
		}
	}
	return false
}

func RedactCommandArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	for i := 0; i < len(out); i++ {
		if out[i] != "-e" || i+1 >= len(out) {
			continue
		}
		key, _, ok := strings.Cut(out[i+1], "=")
		if ok && IsSensitiveEnvKey(key) {
			out[i+1] = key + "=" + RedactedEnvValue
		}
	}
	return out
}
