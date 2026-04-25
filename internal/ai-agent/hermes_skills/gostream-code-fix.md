# GoStream Code Fix Skill

## Purpose
Make code changes to GoStream when the maintenance skill determines a code/config fix is needed. Uses git worktrees for isolated development.

## When Activated
When the gostream-maintenance skill determines that a GoStream code change or config modification is needed to fix an issue.

## Workflow

### Step 1: Check current state
```bash
cd /Users/lorenzo/VSCodeWorkspace/gostream
git status
git log -10 --oneline
git branch --show-current
```

### Step 2: Create isolated worktree
```bash
# Create unique branch name based on issue type and timestamp
BRANCH="fix/ai-{issue_type}-$(date +%Y%m%d%H%M)"
git worktree add ../gostream-worktrees/"$(echo $BRANCH | tr '/' '-')" -b $BRANCH HEAD
```

If worktree directory doesn't exist, create it first:
```bash
mkdir -p ../gostream-worktrees
```

### Step 3: Make the fix
Navigate to the worktree:
```bash
cd ../gostream-worktrees/"$(echo $BRANCH | tr '/' '-')"
```

Make focused, atomic changes:
- Edit only the files necessary for this specific fix
- Follow existing code patterns (read surrounding code first)
- Add structured logging where needed

### Step 4: Build and test
```bash
go build -o /dev/null .
go test ./internal/ai-agent/... -v -count=1
```
If build fails → fix the issue. If tests fail → investigate and fix.

### Step 5: Commit
```bash
git add <changed files>
git commit -m "fix: <description> ({issue_type})"
```
Commit message format: `fix: <what was fixed> (<issue_type from batch>)`

### Step 6: Report
Report back to the maintenance skill with:
- Branch name
- Commit hash
- Summary of changes
- Build/test results

The maintenance skill will then decide whether to:
- Merge the branch manually
- Create a PR
- Keep as-is for later review

## Rules
- **NEVER** commit to the main worktree
- **ALWAYS** create a new worktree per fix
- **NEVER** force-push or amend commits
- **ALWAYS** build and test before committing
- If build or test fails after 2 attempts → report failure to user, don't commit
- Keep changes minimal and focused — one fix per branch

## Common Fixes
1. **Config tuning**: Adjust concurrency limits, cache sizes, timeouts in `config.go`
2. **Logging**: Add structured log entries to under-instrumented areas
3. **Error handling**: Add retry logic, timeout handling, graceful degradation
4. **Detector additions**: New issue detection patterns in `internal/ai-agent/`

## Error Handling
- If `git worktree add` fails (branch exists): try with different timestamp
- If build fails: read error output, fix, retry (max 2 attempts)
- If test fails: read failure details, fix, retry (max 2 attempts)
- If all retries fail: report to user with error details
