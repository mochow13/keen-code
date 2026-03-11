package repl

type PermissionChoice int

const (
	PermissionChoiceAllow PermissionChoice = iota
	PermissionChoiceAllowSession
	PermissionChoiceDeny
)

func permissionChoices(isDangerous bool) []string {
	if isDangerous {
		return []string{"Allow", "Deny"}
	}
	return []string{"Allow", "Allow for this session", "Deny"}
}

func permissionChoiceAt(cursor int, isDangerous bool) PermissionChoice {
	if isDangerous {
		if cursor == 0 {
			return PermissionChoiceAllow
		}
		return PermissionChoiceDeny
	}
	return PermissionChoice(cursor)
}
