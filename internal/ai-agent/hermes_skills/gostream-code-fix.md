# GoStream Code Fix Skill

## Purpose
Make code changes to GoStream safely using git worktrees. Triggered when the maintenance skill determines that a code fix is needed.

## Workflow

### Step 1: Assess current state
```bash
cd /Users/lorenzo/VSCodeWorkspace/gostream
git status
git log -5 --oneline
git diff HEAD
```

### Step 2: Create isolated worktree
```bash
# Create a unique branch and worktree
WORKTREE="/Users/lorenzo/VSCodeWorkspace/gostream-fix-$(date +%s)"
BRANCH="fix/$(echo $ISSUE_TYPE | tr '[:upper:]' '[:lower:]' | tr ' ' '-')-$(date +%s)"
git worktree add "$WORKTREE" -b "$BRANCH"
cd "$WORKTREE"
```

### Step 3: Make the fix
- Make focused, atomic changes only
- One issue = one commit
- Follow existing code patterns (read surrounding code first)
- Add comments explaining the fix

### Step 4: Build and test
```bash
go build -o /dev/null .
go test ./internal/ai-agent/... -v -count=1
# If modifying other packages:
go test ./... -count=1
```

### Step 5: Commit
```bash
git add <changed files>
git commit -m "fix: <what was changed> (<issue_type>)

<2-3 line description of the problem and solution>"
```

### Step 6: Report
Send Telegram message:
```
🔧 Code fix ready

Branch: fix/<issue-type>-<timestamp>
Worktree: /Users/lorenzo/VSCodeWorkspace/gostream-fix-<timestamp>
Commit: <short hash>
Message: <commit message>

Build: ✅ PASS / ❌ FAIL
Tests: ✅ PASS / ❌ FAIL

Review and merge when ready:
  cd /Users/lorenzo/VSCodeWorkspace/gostream
  git merge fix/<issue-type>-<timestamp>
```

## Rules
- **NEVER** commit to the main worktree (use worktrees always)
- **NEVER** force-push or amend commits
- **ALWAYS** build before committing
- **ALWAYS** run relevant tests before committing
- If build or test fails → report failure to user, do NOT commit
- One issue per worktree — if multiple fixes needed, create separate worktrees

## Cleanup
After user merges the fix:
```bash
git worktree remove "$WORKTREE"
git branch -d "$BRANCH"
```

## Error Handling
- If worktree creation fails (branch exists): use different timestamp suffix
- If build fails: report exact errors, ask user for guidance
- If tests fail: fix test failures before reporting, or report as "partial fix"
