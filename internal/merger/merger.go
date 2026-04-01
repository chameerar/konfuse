// Package merger implements the pure kubeconfig merge logic and backup utility.
package merger

import (
	"fmt"
	"io"
	"os"
	"time"
)

// NamedEntry is the common shape of a cluster, user, or context list item.
// The Name field maps to "name", and Body captures all remaining keys inline
// (e.g. "cluster", "user", "context") without losing unknown fields.
type NamedEntry struct {
	Name string                 `yaml:"name"`
	Body map[string]interface{} `yaml:",inline"`
}

// KubeConfig is the top-level kubeconfig structure.
type KubeConfig struct {
	APIVersion     string                 `yaml:"apiVersion"`
	Kind           string                 `yaml:"kind"`
	Clusters       []NamedEntry           `yaml:"clusters"`
	Users          []NamedEntry           `yaml:"users"`
	Contexts       []NamedEntry           `yaml:"contexts"`
	CurrentContext string                 `yaml:"current-context"`
	Preferences    map[string]interface{} `yaml:"preferences"`
}

// SectionResult records which entries were added vs replaced in one section.
type SectionResult struct {
	Added    []string `json:"added"`
	Replaced []string `json:"replaced"`
}

// MergeResult is the structured output of MergeKubeconfig.
type MergeResult struct {
	Clusters SectionResult `json:"clusters"`
	Users    SectionResult `json:"users"`
	Contexts SectionResult `json:"contexts"`
}

// MergeKubeconfig merges incoming into existing and returns the merged config
// along with a summary of what changed. existing may be nil (fresh start).
// Non-empty rename* strings rename only the first entry in that section of
// incoming; cluster/user cross-references inside the first context are updated
// to match. No I/O is performed.
func MergeKubeconfig(existing, incoming *KubeConfig, renameContext, renameCluster, renameUser string) (*KubeConfig, MergeResult) {
	result := MergeResult{
		Clusters: SectionResult{Added: []string{}, Replaced: []string{}},
		Users:    SectionResult{Added: []string{}, Replaced: []string{}},
		Contexts: SectionResult{Added: []string{}, Replaced: []string{}},
	}

	if existing == nil {
		existing = &KubeConfig{
			APIVersion:     "v1",
			Kind:           "Config",
			Clusters:       []NamedEntry{},
			Users:          []NamedEntry{},
			Contexts:       []NamedEntry{},
			CurrentContext: "",
			Preferences:    map[string]interface{}{},
		}
	}

	// Ensure sections are never nil.
	if existing.Clusters == nil {
		existing.Clusters = []NamedEntry{}
	}
	if existing.Users == nil {
		existing.Users = []NamedEntry{}
	}
	if existing.Contexts == nil {
		existing.Contexts = []NamedEntry{}
	}
	if incoming.Clusters == nil {
		incoming.Clusters = []NamedEntry{}
	}
	if incoming.Users == nil {
		incoming.Users = []NamedEntry{}
	}
	if incoming.Contexts == nil {
		incoming.Contexts = []NamedEntry{}
	}

	// Determine original → new names for the first entry in each section.
	var origClusterName, newClusterName string
	var origContextName, newContextName string
	var origUserName, newUserName string

	if len(incoming.Clusters) > 0 {
		origClusterName = incoming.Clusters[0].Name
		newClusterName = origClusterName
		if renameCluster != "" {
			newClusterName = renameCluster
		}
	}
	if len(incoming.Contexts) > 0 {
		origContextName = incoming.Contexts[0].Name
		newContextName = origContextName
		if renameContext != "" {
			newContextName = renameContext
		}
	}
	if len(incoming.Users) > 0 {
		origUserName = incoming.Users[0].Name
		newUserName = origUserName
		if renameUser != "" {
			newUserName = renameUser
		}
	}

	// Merge clusters.
	for _, c := range incoming.Clusters {
		name := c.Name
		if renameCluster != "" && c.Name == origClusterName {
			name = newClusterName
		}
		entry := NamedEntry{Name: name, Body: c.Body}
		idx := findByName(existing.Clusters, name)
		if idx >= 0 {
			existing.Clusters = append(existing.Clusters[:idx], existing.Clusters[idx+1:]...)
			result.Clusters.Replaced = append(result.Clusters.Replaced, name)
		} else {
			result.Clusters.Added = append(result.Clusters.Added, name)
		}
		existing.Clusters = append(existing.Clusters, entry)
	}

	// Merge users.
	for _, u := range incoming.Users {
		name := u.Name
		if renameUser != "" && u.Name == origUserName {
			name = newUserName
		}
		entry := NamedEntry{Name: name, Body: u.Body}
		idx := findByName(existing.Users, name)
		if idx >= 0 {
			existing.Users = append(existing.Users[:idx], existing.Users[idx+1:]...)
			result.Users.Replaced = append(result.Users.Replaced, name)
		} else {
			result.Users.Added = append(result.Users.Added, name)
		}
		existing.Users = append(existing.Users, entry)
	}

	// Merge contexts, updating cluster/user cross-references for the first entry.
	for _, ctx := range incoming.Contexts {
		name := ctx.Name
		if renameContext != "" && ctx.Name == origContextName {
			name = newContextName
		}

		// Deep-copy the body so we don't mutate the incoming struct.
		bodyCopy := make(map[string]interface{}, len(ctx.Body))
		for k, v := range ctx.Body {
			bodyCopy[k] = v
		}

		// Update cluster/user refs in the "context" sub-map.
		if ctxData, ok := bodyCopy["context"].(map[string]interface{}); ok {
			ctxDataCopy := make(map[string]interface{}, len(ctxData))
			for k, v := range ctxData {
				ctxDataCopy[k] = v
			}
			if renameCluster != "" {
				if val, ok := ctxDataCopy["cluster"].(string); ok && val == origClusterName {
					ctxDataCopy["cluster"] = newClusterName
				}
			}
			if renameUser != "" {
				if val, ok := ctxDataCopy["user"].(string); ok && val == origUserName {
					ctxDataCopy["user"] = newUserName
				}
			}
			bodyCopy["context"] = ctxDataCopy
		}

		entry := NamedEntry{Name: name, Body: bodyCopy}
		idx := findByName(existing.Contexts, name)
		if idx >= 0 {
			existing.Contexts = append(existing.Contexts[:idx], existing.Contexts[idx+1:]...)
			result.Contexts.Replaced = append(result.Contexts.Replaced, name)
		} else {
			result.Contexts.Added = append(result.Contexts.Added, name)
		}
		existing.Contexts = append(existing.Contexts, entry)
	}

	return existing, result
}

// BackupConfig creates a timestamped copy of the file at path.
// Returns the backup path, or an empty string if the file does not exist.
func BackupConfig(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	timestamp := time.Now().Format("20060102T150405")
	backupPath := fmt.Sprintf("%s.backup.%s", path, timestamp)
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.Create(backupPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return backupPath, nil
}

// ListResult holds the entries found in a kubeconfig.
type ListResult struct {
	CurrentContext string       `json:"current_context"`
	Contexts       []ContextInfo `json:"contexts"`
	Clusters       []string     `json:"clusters"`
	Users          []string     `json:"users"`
}

// ContextInfo describes a single context entry.
type ContextInfo struct {
	Name    string `json:"name"`
	Cluster string `json:"cluster"`
	User    string `json:"user"`
}

// ListEntries returns a summary of all entries in the kubeconfig.
func ListEntries(cfg *KubeConfig) ListResult {
	result := ListResult{
		CurrentContext: cfg.CurrentContext,
		Contexts:       []ContextInfo{},
		Clusters:       []string{},
		Users:          []string{},
	}
	for _, ctx := range cfg.Contexts {
		info := ContextInfo{Name: ctx.Name}
		if ctxData, ok := ctx.Body["context"].(map[string]interface{}); ok {
			info.Cluster, _ = ctxData["cluster"].(string)
			info.User, _ = ctxData["user"].(string)
		}
		result.Contexts = append(result.Contexts, info)
	}
	for _, c := range cfg.Clusters {
		result.Clusters = append(result.Clusters, c.Name)
	}
	for _, u := range cfg.Users {
		result.Users = append(result.Users, u.Name)
	}
	return result
}

// DeleteResult records what was removed by a delete operation.
type DeleteResult struct {
	Context string `json:"context"`
	Cluster string `json:"cluster,omitempty"`
	User    string `json:"user,omitempty"`
}

// DeleteContext removes the named context from the kubeconfig. If the context's
// cluster or user are not referenced by any other context, they are also removed.
// Returns the modified config, a summary of what was deleted, and an error if the
// context was not found.
func DeleteContext(cfg *KubeConfig, contextName string) (*KubeConfig, DeleteResult, error) {
	idx := findByName(cfg.Contexts, contextName)
	if idx < 0 {
		return cfg, DeleteResult{}, fmt.Errorf("context %q not found", contextName)
	}

	// Extract cluster/user references from the context being deleted.
	var clusterRef, userRef string
	if ctxData, ok := cfg.Contexts[idx].Body["context"].(map[string]interface{}); ok {
		clusterRef, _ = ctxData["cluster"].(string)
		userRef, _ = ctxData["user"].(string)
	}

	// Remove the context.
	cfg.Contexts = append(cfg.Contexts[:idx], cfg.Contexts[idx+1:]...)

	result := DeleteResult{Context: contextName}

	// If current-context pointed to the deleted context, clear it.
	if cfg.CurrentContext == contextName {
		cfg.CurrentContext = ""
	}

	// Remove orphaned cluster (not referenced by any remaining context).
	if clusterRef != "" && !isClusterReferenced(cfg.Contexts, clusterRef) {
		if ci := findByName(cfg.Clusters, clusterRef); ci >= 0 {
			cfg.Clusters = append(cfg.Clusters[:ci], cfg.Clusters[ci+1:]...)
			result.Cluster = clusterRef
		}
	}

	// Remove orphaned user (not referenced by any remaining context).
	if userRef != "" && !isUserReferenced(cfg.Contexts, userRef) {
		if ui := findByName(cfg.Users, userRef); ui >= 0 {
			cfg.Users = append(cfg.Users[:ui], cfg.Users[ui+1:]...)
			result.User = userRef
		}
	}

	return cfg, result, nil
}

// isClusterReferenced returns true if any context references the given cluster.
func isClusterReferenced(contexts []NamedEntry, cluster string) bool {
	for _, ctx := range contexts {
		if ctxData, ok := ctx.Body["context"].(map[string]interface{}); ok {
			if c, _ := ctxData["cluster"].(string); c == cluster {
				return true
			}
		}
	}
	return false
}

// isUserReferenced returns true if any context references the given user.
func isUserReferenced(contexts []NamedEntry, user string) bool {
	for _, ctx := range contexts {
		if ctxData, ok := ctx.Body["context"].(map[string]interface{}); ok {
			if u, _ := ctxData["user"].(string); u == user {
				return true
			}
		}
	}
	return false
}

// findByName returns the index of the entry with the given name, or -1.
func findByName(entries []NamedEntry, name string) int {
	for i, e := range entries {
		if e.Name == name {
			return i
		}
	}
	return -1
}
