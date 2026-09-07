package tools

import "testing"

func TestIsDangerousCommand_AlwaysDangerous(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"rm", "rm file.txt"},
		{"rmdir", "rmdir dir"},
		{"shred", "shred file.txt"},
		{"dd", "dd if=/dev/zero of=/dev/sda"},
		{"mkfs", "mkfs.ext4 /dev/sda1"},
		{"unlink", "unlink file.txt"},
		{"chown", "chown user:group file"},
		{"shutdown", "shutdown now"},
		{"eval", "eval rm -rf /"},
		{"kill", "kill 1234"},
		{"killall", "killall myserver"},
		{"pkill", "pkill -f my-dev-server"},
		{"xkill", "xkill"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsDangerousCommand(tt.command) {
				t.Errorf("expected %q to be dangerous", tt.command)
			}
		})
	}
}

func TestIsDangerousCommand_PrivilegeEscalation(t *testing.T) {
	tests := []string{
		"sudo ls",
		"su - user",
		"doas command",
		"pkexec program",
		"chroot /path command",
		"ls ~/.ssh",
		"cat $HOME/.aws/credentials",
		"cat /home/alice/.ssh/id_rsa",
		"cat server.pem",
		"cat .env",
		"cat .env.local",
	}

	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			if !IsDangerousCommand(command) {
				t.Errorf("expected %q to be dangerous", command)
			}
		})
	}
}

func TestIsDangerousCommand_GitDangerous(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"push", "git push"},
		{"push force", "git push --force"},
		{"clean", "git clean -fd"},
		{"checkout pathspec", "git checkout -- file.txt"},
		{"reset hard", "git reset --hard HEAD~1"},
		{"restore", "git restore file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsDangerousCommand(tt.command) {
				t.Errorf("expected %q to be dangerous", tt.command)
			}
		})
	}
}

func TestIsDangerousCommand_GitSafe(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"status", "git status"},
		{"log", "git log"},
		{"diff", "git diff"},
		{"checkout branch", "git checkout feature"},
		{"checkout new branch", "git checkout -b feature"},
		{"branch list", "git branch"},
		{"branch safe delete", "git branch -d feature"},
		{"branch force delete", "git branch -D feature"},
		{"tag delete", "git tag -d v1"},
		{"add", "git add file.txt"},
		{"commit", "git commit -m msg"},
		{"revert", "git revert abc123"},
		{"reset", "git reset HEAD~1"},
		{"rm", "git rm file.txt"},
		{"fixtures", "ls test/fixtures"},
		{"pem boundary", "cat notes.pembroke"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsDangerousCommand(tt.command) {
				t.Errorf("expected %q to be safe", tt.command)
			}
		})
	}
}

func TestIsDangerousCommand_ConditionalFlags(t *testing.T) {
	tests := []struct {
		name    string
		command string
		danger  bool
	}{
		{"cp safe", "cp a b", false},
		{"cp force", "cp -f a b", false},
		{"cp force long", "cp --force a b", false},
		{"rsync safe", "rsync -av a b", false},
		{"rsync delete", "rsync --delete a b", true},
		{"rsync force", "rsync --force a b", true},
		{"docker run", "docker run nginx", false},
		{"docker stop", "docker stop mycontainer", false},
		{"docker exec", "docker exec mycontainer ls", false},
		{"docker rm", "docker rm mycontainer", true},
		{"docker rmi", "docker rmi myimage", true},
		{"chmod safe", "chmod +x script.sh", false},
		{"chmod 755", "chmod 755 file", false},
		{"chmod recursive", "chmod -R 755 dir", false},
		{"chmod 777", "chmod 777 file", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDangerousCommand(tt.command)
			if got != tt.danger {
				t.Errorf("IsDangerousCommand(%q) = %v, want %v", tt.command, got, tt.danger)
			}
		})
	}
}

func TestIsDangerousCommand_Redirection(t *testing.T) {
	tests := []struct {
		name    string
		command string
		danger  bool
	}{
		{"overwrite", "echo test > file.txt", false},
		{"overwrite stderr", "cmd 2> file.txt", false},
		{"overwrite clobber", "echo test >| file.txt", false},
		{"stderr to stdout", "ls file1 file2 2>&1", false},
		{"stderr to devnull", "grep pattern dir/ 2>/dev/null", false},
		{"append", "echo test >> file.txt", false},
		{"stdout only", "echo test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDangerousCommand(tt.command)
			if got != tt.danger {
				t.Errorf("IsDangerousCommand(%q) = %v, want %v", tt.command, got, tt.danger)
			}
		})
	}
}

func TestIsDangerousCommand_SafeCommands(t *testing.T) {
	tests := []string{
		"ls -la",
		"cat file.txt",
		"echo hello",
		"go test ./...",
		"go build",
		"go clean -cache",
		"npm test",
		"make",
		"make clean",
		"make install",
		"mkdir dir",
		"touch file",
		"pwd",
		"find . -name '*.go'",
		"grep foo file.txt",
		"python3 -m pytest",
		"mv a b",
		"chmod +x script.sh",
		"git merge feature",
		"git rebase main",
		"git cherry-pick abc123",
		"docker kill mycontainer",
		"install a b",
		"npm uninstall left-pad",
		"pip uninstall requests",
		"pip3 uninstall requests",
		"cargo remove serde",
		"yarn remove left-pad",
	}

	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			if IsDangerousCommand(command) {
				t.Errorf("expected %q to be safe", command)
			}
		})
	}
}

func TestIsDangerousCommand_Compound(t *testing.T) {
	tests := []struct {
		name    string
		command string
		danger  bool
	}{
		{"one dangerous", "echo hello && rm file", true},
		{"all safe", "echo hello && echo world", false},
		{"piped dangerous", "cat file | rm -", true},
		{"or dangerous", "false || sudo ls", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDangerousCommand(tt.command)
			if got != tt.danger {
				t.Errorf("IsDangerousCommand(%q) = %v, want %v", tt.command, got, tt.danger)
			}
		})
	}
}

func TestIsDangerousCommand_SensitiveFilePaths(t *testing.T) {
	tests := []struct {
		name    string
		command string
		danger  bool
	}{
		{"ssh key", "cat ~/.ssh/id_rsa", true},
		{"ssh known_hosts", "cat ~/.ssh/known_hosts", true},
		{"aws credentials", "cat ~/.aws/credentials", true},
		{"netrc", "cat ~/.netrc", true},
		{"git credentials", "cat ~/.git-credentials", true},
		{"etc shadow", "cat /etc/shadow", true},
		{"etc sudoers", "cat /etc/sudoers", true},
		{"proc environ", "cat /proc/1/environ", true},
		{"less sensitive", "less ~/.ssh/id_rsa", true},
		{"head sensitive", "head ~/.aws/credentials", true},
		{"grep sensitive", "grep token ~/.netrc", true},
		{"safe file", "cat file.txt", false},
		{"safe path", "cat /etc/hosts", false},
		{"safe home", "cat ~/project/readme.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDangerousCommand(tt.command)
			if got != tt.danger {
				t.Errorf("IsDangerousCommand(%q) = %v, want %v", tt.command, got, tt.danger)
			}
		})
	}
}

func TestIsDangerousCommand_Subshells(t *testing.T) {
	tests := []struct {
		name    string
		command string
		danger  bool
	}{
		{"dollar paren rm", "echo $(rm file.txt)", true},
		{"dollar paren sudo", "result=$(sudo ls)", true},
		{"backtick rm", "echo `rm file.txt`", true},
		{"backtick sudo", "result=`sudo ls`", true},
		{"nested safe", "echo $(echo hello)", false},
		{"backtick safe", "echo `date`", false},
		{"safe no subshell", "echo hello", false},
		{"chained with subshell", "ls && echo $(chmod 777 f)", false},
		{"subshell with eval", "cat $(eval echo hi)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDangerousCommand(tt.command)
			if got != tt.danger {
				t.Errorf("IsDangerousCommand(%q) = %v, want %v", tt.command, got, tt.danger)
			}
		})
	}
}

func TestIsDangerousCommand_EnvSecrets(t *testing.T) {
	tests := []struct {
		name    string
		command string
		danger  bool
	}{
		{"bare env", "env", true},
		{"env with command", "env GOOS=linux go build", false},
		{"bare printenv", "printenv", true},
		{"printenv common", "printenv HOME", false},
		{"printenv ssh key", "printenv SSH_KEY", true},
		{"export my key", "export MY_KEY=abc", true},
		{"printenv secret", "printenv GITHUB_TOKEN", true},
		{"export safe", "export PATH=$PATH:/foo", false},
		{"export sensitive", "export AWS_SECRET_KEY=abc", true},
		{"unset", "unset VAR", false},
		{"sensitive var", "echo $AWS_SECRET_ACCESS_KEY", true},
		{"common var", "echo $HOME", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDangerousCommand(tt.command)
			if got != tt.danger {
				t.Errorf("IsDangerousCommand(%q) = %v, want %v", tt.command, got, tt.danger)
			}
		})
	}
}

func TestContainsBashSecretExposure(t *testing.T) {
	tests := []struct {
		name    string
		command string
		expose  bool
	}{
		{"sensitive var", "echo $AWS_SECRET_ACCESS_KEY", true},
		{"braced var", "echo ${GITHUB_TOKEN}", true},
		{"api key in header", "curl -H \"Authorization: Bearer $API_KEY\" https://example.com", true},
		{"bare env", "env", true},
		{"printenv secret", "printenv GITHUB_TOKEN", true},
		{"printenv common", "printenv HOME", false},
		{"printenv ssh key", "printenv SSH_KEY", true},
		{"export my key", "export MY_KEY=abc", true},
		{"export sensitive", "export AWS_SECRET_KEY=abc", true},
		{"export safe", "export PATH=$PATH:/foo", false},
		{"subshell dump", "echo $(printenv GITHUB_TOKEN)", true},
		{"chained", "go test ./... && printenv AWS_SESSION_TOKEN", true},
		{"plain command", "echo hello", false},
		{"common var", "echo $HOME", false},
		{"destructive only", "rm file.txt", false},
		{"git push only", "git push", false},
		{"empty", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsBashSecretExposure(tt.command)
			if got != tt.expose {
				t.Errorf("ContainsBashSecretExposure(%q) = %v, want %v", tt.command, got, tt.expose)
			}
		})
	}
}
