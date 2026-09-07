package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var sensitiveHomeRelativePaths = []string{
	".ssh",
	".aws",
	".azure",
	".kube",
	".docker",
	".gnupg",
	".config/gcloud",
	".config/gh",
	"Library/Keychains",
	".netrc",
	".git-credentials",
	".npmrc",
	".pypirc",
	".keen",
	".claude",
	".agents",
}

var sensitiveSystemPaths = []string{
	"/etc/shadow",
	"/etc/sudoers",
	"/proc",
}

var sensitiveFileNameRegexps = []*regexp.Regexp{
	regexp.MustCompile(`^(.*/)?\.env(\..*)?$`),
	regexp.MustCompile(`^(.*/)?id_(rsa|dsa|ecdsa|ed25519)$`),
	regexp.MustCompile(`^.*\.(key|pem|p12|pfx)$`),
}

var alwaysDangerousBaseCommands = map[string]struct{}{
	"rm": {}, "rmdir": {}, "shred": {}, "dd": {}, "unlink": {},
	"chown": {},

	"sudo": {}, "su": {}, "doas": {}, "pkexec": {}, "chroot": {},

	"kill": {}, "killall": {}, "pkill": {}, "xkill": {},
	"shutdown": {}, "reboot": {}, "halt": {}, "poweroff": {},

	"eval": {},
}

var dangerousGitSubcommands = map[string]struct{}{
	"clean": {}, "push": {},
}

var dangerousDockerSubcommands = map[string]struct{}{
	"rm": {}, "rmi": {},
}

var dangerousKubectlSubcommands = map[string]struct{}{
	"apply": {}, "delete": {}, "patch": {}, "edit": {},
}

var dangerousSystemctlSubcommands = map[string]struct{}{
	"stop": {}, "restart": {}, "disable": {},
}

var dangerousPackageManagerRemovalFlags = map[string]map[string]struct{}{
	"apt-get": {"remove": {}, "purge": {}, "autoremove": {}},
	"apt":     {"remove": {}, "purge": {}, "autoremove": {}},
	"yum":     {"remove": {}},
	"dnf":     {"remove": {}},
	"pacman":  {"-R": {}, "-Rs": {}, "-Rns": {}},
}

var conditionalDangerousFlags = map[string]map[string]struct{}{
	"rsync": {"--delete": {}, "--force": {}},
}

var sensitiveEnvVarKeywords = []string{"AWS", "SECRET", "TOKEN", "PASSWORD", "PRIVATE", "KEY"}
var sensitiveEnvVarPattern = regexp.MustCompile(`\$(?:[A-Z_]*(?:` + strings.Join(sensitiveEnvVarKeywords, "|") + `)[A-Z_0-9]*|\{[A-Z_]*(?:` + strings.Join(sensitiveEnvVarKeywords, "|") + `)[A-Z_0-9]*\})`)

func IsDangerousCommand(command string) bool {
	return ContainsBashSecretExposure(command) || containsDestructiveBashCommand(command)
}

func ContainsBashSecretExposure(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}

	if sensitiveEnvVarPattern.MatchString(command) {
		return true
	}

	for _, segment := range splitCommandSegments(command) {
		if isDangerousEnvCommand(tokenize(segment)) {
			return true
		}
	}

	for _, sub := range extractSubshells(command) {
		if ContainsBashSecretExposure(sub) {
			return true
		}
	}

	return false
}

func containsDestructiveBashCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}

	if containsPrivilegeEscalation(command) {
		return true
	}

	for _, segment := range splitCommandSegments(command) {
		if isDangerousSegment(segment) {
			return true
		}
	}

	for _, sub := range extractSubshells(command) {
		if containsDestructiveBashCommand(sub) {
			return true
		}
	}

	return false
}

func containsPrivilegeEscalation(command string) bool {
	for _, cmd := range []string{"sudo", "su", "doas", "pkexec", "chroot"} {
		if isCommandWord(command, cmd) {
			return true
		}
	}
	return false
}

func isCommandWord(command, name string) bool {
	for _, seg := range splitCommandSegments(command) {
		tokens := tokenize(seg)
		if len(tokens) > 0 && tokens[0] == name {
			return true
		}
	}
	return false
}

func splitCommandSegments(command string) []string {
	separators := []string{"&&", "||", ";", "|"}
	segments := []string{command}
	for _, sep := range separators {
		var next []string
		for _, s := range segments {
			parts := strings.Split(s, sep)
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					next = append(next, p)
				}
			}
		}
		segments = next
	}
	return segments
}

func tokenize(segment string) []string {
	return strings.Fields(segment)
}

func isDangerousSegment(segment string) bool {
	tokens := tokenize(segment)
	if len(tokens) == 0 {
		return false
	}

	base := tokens[0]

	if _, ok := alwaysDangerousBaseCommands[base]; ok {
		return true
	}

	if base == "mkfs" || strings.HasPrefix(base, "mkfs.") {
		return true
	}

	if isDangerousGit(tokens) {
		return true
	}

	if base == "docker" && len(tokens) > 1 {
		if _, ok := dangerousDockerSubcommands[tokens[1]]; ok {
			return true
		}
	}

	if base == "kubectl" && len(tokens) > 1 {
		if _, ok := dangerousKubectlSubcommands[tokens[1]]; ok {
			return true
		}
	}

	if base == "systemctl" && len(tokens) > 1 {
		if _, ok := dangerousSystemctlSubcommands[tokens[1]]; ok {
			return true
		}
	}

	if isDangerousPackageManager(tokens) {
		return true
	}

	if isDangerousInterpreter(tokens) {
		return true
	}

	if flags, ok := conditionalDangerousFlags[base]; ok {
		for _, tok := range tokens[1:] {
			if _, ok := flags[tok]; ok {
				return true
			}
		}
	}

	if hasSensitiveFilePath(tokens[1:]) {
		return true
	}

	return false
}

func isDangerousGit(tokens []string) bool {
	if tokens[0] != "git" || len(tokens) < 2 {
		return false
	}
	sub := tokens[1]

	if _, ok := dangerousGitSubcommands[sub]; ok {
		return true
	}

	if sub == "checkout" && hasFlag(tokens[2:], "-f", "--force", "--") {
		return true
	}

	if sub == "reset" && hasFlag(tokens[2:], "--hard") {
		return true
	}

	if sub == "restore" {
		return true
	}

	return false
}

func isDangerousPackageManager(tokens []string) bool {
	base := tokens[0]
	flags, ok := dangerousPackageManagerRemovalFlags[base]
	if !ok {
		return false
	}
	for _, tok := range tokens[1:] {
		if _, ok := flags[tok]; ok {
			return true
		}
	}
	return false
}

func isDangerousInterpreter(tokens []string) bool {
	base := tokens[0]
	switch base {
	case "bash", "sh", "zsh":
		return hasFlag(tokens[1:], "-c")
	case "python", "python3":
		return hasFlag(tokens[1:], "-c")
	case "perl":
		return hasFlag(tokens[1:], "-e")
	case "ruby":
		return hasFlag(tokens[1:], "-e")
	}
	return false
}

func isDangerousEnvCommand(tokens []string) bool {
	base := tokens[0]

	if base == "env" {
		// env with no command dumps the whole environment
		if len(tokens) == 1 {
			return true
		}
		hasCommand := false
		for _, tok := range tokens[1:] {
			if !strings.Contains(tok, "=") {
				hasCommand = true
				break
			}
		}
		return !hasCommand
	}

	if base == "printenv" {
		if len(tokens) == 1 {
			return true // bare printenv dumps all env
		}
		for _, tok := range tokens[1:] {
			if isSensitiveEnvVarName(tok) {
				return true
			}
		}
	}

	if base == "export" {
		for _, tok := range tokens[1:] {
			name := strings.SplitN(tok, "=", 2)[0]
			if isSensitiveEnvVarName(name) {
				return true
			}
		}
	}

	return false
}

func isSensitiveEnvVarName(name string) bool {
	upper := strings.ToUpper(name)
	for _, keyword := range sensitiveEnvVarKeywords {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

func hasSensitiveFilePath(args []string) bool {
	home, _ := os.UserHomeDir()
	for _, arg := range args {
		for _, candidate := range sensitivePathCandidates(arg, home) {
			if isSensitivePath(candidate, home) {
				return true
			}
		}
	}
	return false
}

func sensitivePathCandidates(arg, home string) []string {
	arg = strings.ReplaceAll(arg, "$HOME", home)
	if value, ok := flagValue(arg); ok {
		arg = value
	} else if strings.HasPrefix(arg, "-") {
		return nil
	}
	candidates := []string{arg}
	if strings.HasPrefix(arg, "~") {
		candidates = append(candidates, filepath.Join(home, arg[1:]))
	}
	return candidates
}

func flagValue(arg string) (string, bool) {
	if !strings.HasPrefix(arg, "-") {
		return "", false
	}
	_, value, found := strings.Cut(arg, "=")
	return value, found && value != ""
}

func isSensitivePath(path, home string) bool {
	for _, pattern := range sensitiveFileNameRegexps {
		if pattern.MatchString(path) {
			return true
		}
	}
	for _, relative := range sensitiveHomeRelativePaths {
		lexical := "~/" + relative
		if path == lexical || strings.HasPrefix(path, lexical+"/") {
			return true
		}
		if matchesSensitiveSubtree(path, filepath.Join(home, relative)) {
			return true
		}
	}
	for _, system := range sensitiveSystemPaths {
		if matchesSensitiveSubtree(path, system) {
			return true
		}
	}
	return false
}

func matchesSensitiveSubtree(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}

func hasFlag(flags []string, targets ...string) bool {
	set := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		set[t] = struct{}{}
	}
	for _, f := range flags {
		if _, ok := set[f]; ok {
			return true
		}
	}
	return false
}

func extractSubshells(command string) []string {
	var results []string

	for i := 0; i < len(command); i++ {
		if command[i] == '$' && i+1 < len(command) && command[i+1] == '(' {
			content := extractBalancedParens(command[i+2:])
			if content != "" {
				results = append(results, content)
			}
		} else if command[i] == '`' {
			end := strings.IndexByte(command[i+1:], '`')
			if end >= 0 {
				content := command[i+1 : i+1+end]
				if content != "" {
					results = append(results, content)
				}
			}
		}
	}

	return results
}

// s starts right after "$("; returns content up to the matching ")".
func extractBalancedParens(s string) string {
	depth := 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[:i]
			}
		}
	}
	return ""
}
