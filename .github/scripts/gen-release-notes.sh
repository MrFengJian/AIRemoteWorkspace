#!/usr/bin/env bash
# gen-release-notes.sh — 依据提交说明生成语义化的 Release 说明（markdown 至 stdout）。
#
# 仓库直接提交到 main、没有 PR，GitHub 自动的 generate_release_notes 只会
# 给出一个 compare 链接。本脚本把上一 tag → 当前 tag 区间内的提交按
# conventional commits 前缀（feat/fix/…）分组成小节，跳过纯版本号提升的
# chore 提交，末尾附 compare 链接。
#
# 可用环境变量覆盖（便于本地测试）：
#   GITHUB_REF_NAME    当前发布的 tag（默认取最近一个 tag）
#   GITHUB_REPOSITORY  owner/repo（默认从 origin remote 推导）

set -euo pipefail

TAG="${GITHUB_REF_NAME:-$(git describe --tags --abbrev=0)}"
REPO="${GITHUB_REPOSITORY:-}"
if [ -z "$REPO" ]; then
	origin=$(git remote get-url origin 2>/dev/null || true)
	REPO=$(printf '%s' "$origin" | sed -E 's#.*[:/]([^/]+/[^/.]+?)(\.git)?$#\1#')
fi

# 上一个 tag：HEAD 的前驱（parent）上最近的 v* tag —— 即当前发布 tag 的
# 上一个版本；仓库首个 tag 时为空，从仓库起点开始。
# 注意不能用「最新的其他 tag」：给旧分支补 tag 或本地传任意 tag 时会选错基准。
PREVIOUS=$(git describe --tags --abbrev=0 --match='v*' HEAD^ 2>/dev/null || true)
if [ -n "$PREVIOUS" ]; then
	RANGE="$PREVIOUS..HEAD"
else
	RANGE="HEAD"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# 每类提交先写入独立文件，稍后按固定顺序拼装小节
declare -A FILE_OF=(
	[feat]="$tmp/feat" [fix]="$tmp/fix" [perf]="$tmp/perf"
	[refactor]="$tmp/refactor" [docs]="$tmp/docs" [test]="$tmp/test"
	[build]="$tmp/build" [ci]="$tmp/ci" [style]="$tmp/style"
	[revert]="$tmp/revert" [other]="$tmp/other"
)
declare -A TITLE=(
	[feat]="🚀 新功能"
	[fix]="🐛 问题修复"
	[perf]="⚡ 性能优化"
	[refactor]="♻️ 重构"
	[docs]="📝 文档"
	[test]="✅ 测试"
	[build]="📦 构建"
	[ci]="👷 CI"
	[style]="🎨 风格"
	[revert]="⏪ 回滚"
	[other]="🔧 其他"
)
ORDER=(feat fix perf refactor docs test build ci style revert other)

# conventional commits：type(scope)!?: subject
re='^([A-Za-z]+)(\([^)]*\))?(!)?:[[:space:]]*(.*)$'

# `git log --pretty=format:` 的最后一行没有换行符：read 返回 EOF 但变量已
# 填充，`|| [ -n "$hash" ]` 保证最后一个提交不被丢弃。
while IFS=$'\t' read -r hash subject || [ -n "$hash" ]; do
	[ -n "$hash" ] || continue

	type="other"
	scope=""
	breaking=""
	rest="$subject"
	if [[ "$subject" =~ $re ]]; then
		type="${BASH_REMATCH[1],,}"
		scope="${BASH_REMATCH[2]:1:${#BASH_REMATCH[2]}-2}" # 去掉括号
		[ "${BASH_REMATCH[3]}" = "!" ] && breaking=" ⚠️"
		rest="${BASH_REMATCH[4]}"
	fi

	# 版本号提升类 chore 提交是发布噪音，不进说明
	if [ "$type" = "chore" ] &&
		[[ "$rest" =~ (版本号升至|[Bb]ump[[:space:]]+version|release[[:space:]]+v?[0-9]) ]]; then
		continue
	fi

	case "$type" in
		feat | fix | perf | refactor | docs | test | build | ci | style | revert) ;;
		*) type="other" ;;
	esac

	if [ -n "$scope" ]; then
		entry="- **$scope**: $rest$breaking"
	else
		entry="- $rest$breaking"
	fi
	entry="$entry (\`$hash\`)"

	printf '%s\n' "$entry" >>"${FILE_OF[$type]}"
done < <(git log --no-merges --pretty=format:'%h%x09%s' "$RANGE")

shown=$(cat "${FILE_OF[@]}" 2>/dev/null | wc -l || true)
shown=$(printf '%s' "$shown" | tr -d '[:space:]')

echo "# $TAG"
echo
if [ "$shown" -gt 0 ]; then
	echo "本版包含 $shown 个提交，以下说明由提交记录自动分组生成。"
	echo
fi

for t in "${ORDER[@]}"; do
	f="${FILE_OF[$t]}"
	[ -s "$f" ] || continue
	echo "### ${TITLE[$t]}"
	echo
	cat "$f"
	echo
done

if [ -n "$PREVIOUS" ] && [ -n "$REPO" ]; then
	echo "---"
	echo
	echo "**完整变更**: https://github.com/$REPO/compare/$PREVIOUS...$TAG"
fi
