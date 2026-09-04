package subagents

import (
	"fmt"
	"strings"

	"github.com/mochow13/keen-code/internal/tools"
)

type Profile struct {
	Name           string
	Description    string
	Permissions    []string
	PermissionsSet bool
	Provider       string
	Model          string
	ThinkingEffort string
	TimeoutSeconds int
	Hidden         bool
	Instructions   string
}

type Discovery struct {
	Profiles  []Profile
	Warnings  []string
	locations []string
}

func Find(profiles []Profile, name string) (Profile, bool) {
	for _, profile := range profiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return Profile{}, false
}

var permissionTools = map[string][]string{
	"read":  {tools.ReadFileToolName, tools.GlobToolName, tools.GrepToolName},
	"write": {tools.WriteFileToolName, tools.EditFileToolName},
	"bash":  {tools.BashToolName},
	"web":   {tools.WebFetchToolName},
}

func validatePermissions(profile Profile) error {
	if !profile.PermissionsSet {
		return nil
	}
	if len(profile.Permissions) == 0 {
		return fmt.Errorf("permissions must not be empty")
	}
	for _, permission := range profile.Permissions {
		if _, ok := permissionTools[strings.TrimSpace(permission)]; !ok {
			return fmt.Errorf("unknown permission %q", permission)
		}
	}
	return nil
}

func permissionToolNames(profile Profile, inherited []string) []string {
	if !profile.PermissionsSet {
		return filterChildToolNames(inherited)
	}
	seen := map[string]bool{}
	var names []string
	for _, permission := range profile.Permissions {
		for _, name := range permissionTools[permission] {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return filterChildToolNames(names)
}

func filterChildToolNames(names []string) []string {
	result := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || name == tools.DelegateToolName || name == tools.CallMCPToolName || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}
