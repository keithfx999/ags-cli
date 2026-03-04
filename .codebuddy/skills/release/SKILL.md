---
name: release
description: This skill automates the version release process including changelog generation and bilingual documentation updates via release branch workflow. Tags are created automatically by GitHub Actions after the release PR is merged. Use this skill when the user wants to create a new release, bump version, generate changelog, or prepare a release PR.
---

# Release Automation Skill

This skill handles the complete release workflow for this Go CLI project, ensuring bilingual documentation stays in sync. The `main` branch is protected and only accepts changes via Pull Requests. Tags are created automatically by GitHub Actions.

## When to Use

- User requests to "release a new version"
- User wants to "bump version" or "prepare a release"
- User asks to "generate changelog" or "update changelog"
- User mentions "prepare release" or "cut a release"

## Release Workflow

### Step 1: Determine Version

Ask the user for the version type if not specified.

#### Stable Releases

- **patch**: Bug fixes (v1.0.0 → v1.0.1)
- **minor**: New features (v1.0.0 → v1.1.0)
- **major**: Breaking changes (v1.0.0 → v2.0.0)

#### Pre-release Versions

- **alpha**: Early development (v0.0.1-alpha → v0.0.1-alpha.1 → v0.0.1-alpha.2)
- **beta**: Feature complete, testing (v0.0.1-beta → v0.0.1-beta.1)
- **rc**: Release candidate (v0.0.1-rc.1 → v0.0.1-rc.2)

#### Pre-release Progression

```
v0.0.1-alpha → v0.0.1-alpha.1 → v0.0.1-alpha.2
    ↓
v0.0.1-beta → v0.0.1-beta.1 → v0.0.1-beta.2
    ↓
v0.0.1-rc.1 → v0.0.1-rc.2
    ↓
v0.0.1 (stable)
```

To get the current version:

```bash
git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"
```

#### Version Calculation Examples

| Current Tag | Release Type | New Version |
|-------------|--------------|-------------|
| v0.0.1-alpha.2 | alpha | v0.0.1-alpha.3 |
| v0.0.1-alpha.2 | beta | v0.0.1-beta |
| v0.0.1-beta.1 | beta | v0.0.1-beta.2 |
| v0.0.1-beta.2 | rc | v0.0.1-rc.1 |
| v0.0.1-rc.2 | stable | v0.0.1 |
| v0.0.1 | patch | v0.0.2 |
| v0.0.1 | minor | v0.1.0 |
| v0.0.1 | major | v1.0.0 |

### Step 2: Create Release Branch

Create a release branch from the latest `main`:

```bash
git checkout main && git pull origin main
git checkout -b release/vX.Y.Z
```

**Important**: The branch MUST be named `release/vX.Y.Z` (e.g., `release/v0.1.3`). The GitHub Actions workflow extracts the version from this branch name.

### Step 3: Gather Changes

Collect commits since last tag:

```bash
git log $(git describe --tags --abbrev=0 2>/dev/null || echo "")..HEAD --oneline --no-merges
```

Categorize commits into:

- **Added**: New features (commits with "feat:", "add:", "new:")
- **Changed**: Changes (commits with "change:", "update:", "refactor:")
- **Fixed**: Bug fixes (commits with "fix:", "bug:", "patch:")
- **Removed**: Removals (commits with "remove:", "delete:", "deprecate:")

### Step 4: Update CHANGELOG Files

Update both `CHANGELOG.md` and `CHANGELOG-zh.md` simultaneously on the release branch.

#### English CHANGELOG.md Format

```markdown
# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [X.Y.Z] - YYYY-MM-DD

### Added
- Feature description

### Changed
- Change description

### Fixed
- Bug fix description

### Removed
- Removal description
```

#### Chinese CHANGELOG-zh.md Format

```markdown
# 更新日志

本项目的所有重要更改都将记录在此文件中。

## [未发布]

## [X.Y.Z] - YYYY-MM-DD

### 新增
- 功能描述

### 变更
- 变更描述

### 修复
- 修复描述

### 移除
- 移除描述
```

### Step 5: Commit and Push Release Branch

```bash
git add -A
git commit -m "chore: release vX.Y.Z"
git push origin release/vX.Y.Z
```

### Step 6: Create Pull Request

Create a PR from `release/vX.Y.Z` to `main`:

- **Title**: `chore: release vX.Y.Z`
- **Description**: Include the changelog entries for this version

If `gh` CLI is available:

```bash
gh pr create --base main --head release/vX.Y.Z \
  --title "chore: release vX.Y.Z" \
  --body "## Release vX.Y.Z

<paste changelog entries here>"
```

### Step 7: Verification Checklist

Before merging the PR, verify:

1. [ ] CHANGELOG.md has new version section
2. [ ] CHANGELOG-zh.md has matching Chinese version
3. [ ] Both changelogs have same structure and content
4. [ ] Release branch named `release/vX.Y.Z`
5. [ ] PR title follows format: `chore: release vX.Y.Z`
6. [ ] All CI checks pass

### Step 8: After PR Merge (Automatic)

Once the PR is merged into `main`, GitHub Actions will automatically:

1. Create an annotated tag `vX.Y.Z` on the merge commit
2. Create a GitHub Release with auto-generated release notes

**No manual tag creation or pushing is needed.**

## Section Header Translations

| English | Chinese |
|---------|---------|
| Changelog | 更新日志 |
| Unreleased | 未发布 |
| Added | 新增 |
| Changed | 变更 |
| Fixed | 修复 |
| Removed | 移除 |
| Deprecated | 废弃 |
| Security | 安全 |

## Commit Message Conventions

This skill recognizes conventional commit prefixes:

| Prefix | Category |
|--------|----------|
| `feat:`, `feature:`, `add:` | Added |
| `fix:`, `bug:`, `bugfix:` | Fixed |
| `change:`, `update:`, `refactor:` | Changed |
| `remove:`, `delete:`, `deprecate:` | Removed |
| `docs:` | Documentation (usually skip) |
| `test:`, `ci:` | Testing/CI (usually skip) |
| `chore:` | Maintenance (usually skip) |

## Error Handling

- If no tags exist, start from v0.0.1-alpha
- If CHANGELOG files don't exist, create them with proper headers
- Always create both language versions together
- For pre-release versions, increment the numeric suffix (alpha.1 → alpha.2)
- When transitioning phases (alpha → beta), reset the suffix
- If `gh` CLI is not available, instruct user to create PR via GitHub web UI
