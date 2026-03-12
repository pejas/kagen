# Git Authorship Configuration Plan for Kagen

## 1. Problem Statement

When using AI agents in kagen, commits are currently authored inconsistently:
- **Codex**: Uses `oai-codex` author (hardcoded somewhere)
- **OpenCode/Claude**: No specific author configuration, likely defaults to generic values

**Goal**: Implement consistent, transparent git authorship that:
1. Clearly attributes code to the specific AI agent (for GitHub stats/insights)
2. Routes notifications to the human user (you)
3. Distinguishes between who wrote the code (agent) and who committed it (you)

---

## 2. Solution Architecture

### 2.1 Author Identity Mapping

Map internal agent types to GitHub-recognized author names:

| Agent Type   | Internal Enum      | GitHub Author Name | Git Author Email          |
|--------------|-------------------|--------------------|---------------------------|
| Codex        | `agent.Codex`     | `oai-codex`        | `you+oai-codex@domain`    |
| Claude       | `agent.Claude`    | `claude`           | `you+claude@domain`       |
| OpenCode     | `agent.OpenCode`  | `opencode`         | `you+opencode@domain`     |

**Intention**: Use the GitHub-recognized names that appear in commit histories across open source projects. This ensures proper contributor attribution in GitHub Insights and stats.

### 2.2 Email Strategy (Subaddressing)

Using **RFC 5233 subaddressing** (+tag):
- **Format**: `{user}+{agent}@{domain}`
- **Example**: `you+opencode@gmail.com`
- **Benefit**: Gmail/Outlook treat as `you@gmail.com`, but you can filter by `+opencode`

### 2.3 Git Environment Variables

Per-commit, Git uses these environment variables (falls back to `user.name`/`user.email` if unset):

```bash
# Who wrote the code
GIT_AUTHOR_NAME=opencode
GIT_AUTHOR_EMAIL=you+opencode@example.com

# Who committed the code (you, from host)
GIT_COMMITTER_NAME="Your Name"
GIT_COMMITTER_EMAIL="you@example.com"
```

---

## 3. Implementation Details

### 3.1 Files to Modify

**A. `internal/agent/types.go`**
- Add `GitAuthorName` field to agent type definitions
- Update agent type constants with mapping

**B. `internal/git/config.go`** (new file)
- Add function to read host git config: `GetHostGitUser() -> (name, email, error)`
- Add function to transform email: `AddSubaddress(email, tag) -> user+tag@domain`

**C. `internal/cluster/kube.go`**
- Modify `injectAgentRuntime()` to:
  1. Read host git user config
  2. Generate author email with subaddress
  3. Inject `GIT_AUTHOR_*` and `GIT_COMMITTER_*` env vars

### 3.2 Code Flow

```go
// 1. In cluster/kube.go::injectAgentRuntime()
func injectAgentRuntime(pod *corev1.Pod, agentType, namespace string, policy *proxy.Policy) {
    // Get agent spec
    spec, err := agent.SpecFor(agent.Type(agentType))
    
    // Read host git config
    hostName, hostEmail := git.GetHostUser() // Reads from local git config
    
    // Build agent author email with subaddress
    authorEmail := git.AddSubaddress(hostEmail, spec.GitAuthorName)
    
    // Set environment variables on the agent container
    setContainerEnv(container, "GIT_AUTHOR_NAME", spec.GitAuthorName)
    setContainerEnv(container, "GIT_AUTHOR_EMAIL", authorEmail)
    setContainerEnv(container, "GIT_COMMITTER_NAME", hostName)
    setContainerEnv(container, "GIT_COMMITTER_EMAIL", hostEmail)
}
```

### 3.3 Example Git Config

**Host (your machine)**:
```bash
git config user.name "Pejas"
git config user.email "pejas@example.com"
```

**Inside kagen pod (OpenCode)**:
```bash
GIT_AUTHOR_NAME=opencode
GIT_AUTHOR_EMAIL=pejas+opencode@example.com
GIT_COMMITTER_NAME=Pejas
GIT_COMMITTER_EMAIL=pejas@example.com
```

**Resulting commit**:
```
commit abc123
Author: opencode <pejas+opencode@example.com>
Committer: Pejas <pejas@example.com>

    Add feature X
```

---

## 4. GitHub Integration Benefits

1. **Insights/Stats**: GitHub will show commits by `opencode`, `claude`, `oai-codex` as separate contributors
2. **Transparency**: Easy to see AI vs human contributions in history
3. **Notifications**: All emails route to your inbox (subaddress stripped by Gmail/Outlook)
4. **Filtering**: Can create Gmail filters:
   - `to:pejas+opencode@example.com` -> Label: OpenCode
   - `to:pejas+oai-codex@example.com` -> Label: Codex

---

## 5. Edge Cases & Fallbacks

| Scenario | Handling |
|----------|----------|
| Host has no git config | Use OS username as fallback; warn user |
| Email already has +subaddress | Append: `user+existing+agent@domain` |
| Non-Gmail email providers | Still works for most providers (RFC 5233) |
| Invalid email format | Fall back to original email; log warning |

---

## 6. Testing Checklist

- [ ] Verify `git.GetHostUser()` reads local git config correctly
- [ ] Verify `git.AddSubaddress()` transforms emails correctly
- [ ] Verify env vars are injected into pod spec
- [ ] Verify commits show correct author in GitHub
- [ ] Verify email notifications reach host email
- [ ] Test with missing git config (fallback behaviour)

---

## 7. Future Enhancements (Optional)

- Allow `.kagen.yaml` override for git author configuration
- Support custom email templates
- Support per-project author overrides
