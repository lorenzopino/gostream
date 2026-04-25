# GoStream Code Fix Skill

**Trigger:** When maintenance skill determines a code/config change is needed

## Workflow

1. Check current state:
```bash
cd /Users/lorenzo/VSCodeWorkspace/gostream
git status
git log -10 --oneline
git diff HEAD
```

2. Create isolated worktree:
```bash
ISSUE_TYPE="<type>"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
BRANCH="fix/${ISSUE_TYPE}-${TIMESTAMP}"
WORKTREE="../gostream-fix-${ISSUE_TYPE}-${TIMESTAMP}"

git worktree add "$WORKTREE" -b "$BRANCH"
cd "$WORKTREE"
```

3. Make focused, atomic change in the worktree

4. Build:
```bash
go build -o /dev/null .
```
If build fails → STOP. Report error to user.

5. Test:
```bash
go test ./internal/ai-agent/... -v -count=1
```
If tests fail → STOP. Report failure to user.

6. Commit:
```bash
git add <changed files>
git commit -m "fix: <description> (${ISSUE_TYPE})"
```

7. Report to user:
```
🔧 Code fix ready

Branch: ${BRANCH}
Worktree: ${WORKTREE}
Commit: $(git log -1 --oneline)
Changes: $(git diff HEAD~1 --stat)

Please review. To merge:
  cd /Users/lorenzo/VSCodeWorkspace/gostream
  git merge ${BRANCH}
  git worktree remove ${WORKTREE}
```

## Rules

- NEVER commit to the main worktree
- ALWAYS create a new branch per fix
- NEVER force-push or amend commits
- ALWAYS build and test before committing
- If build/test fails → report to user, don't commit
- Clean up worktree after merge

## GoStream-Specific Guidelines

- Follow existing code patterns (standard library log, net/http on DefaultServeMux)
- Use `safeGo()` for background goroutines
- Use `sync.Map` or atomic operations for hot-path concurrency
- Never block the FUSE hot path
- Follow Go conventions (error handling, naming, etc.)
- Comment changes clearly
