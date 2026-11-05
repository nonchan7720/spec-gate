#!/usr/bin/env bash
# bin/spec-gate の固定版（hook と承認の経路で動かす spec-gate）を、指定したリリースのタグに上げる。人間が実行する
# （bin/spec-gate は guard が保護するので、エージェントは書き換えられない）。
#   scripts/pin-spec-gate.sh v0.0.0-dev.abcdef0
# 配布物ごとに checksums.txt で照らしてから展開し、中の spec-gate の sha256 を bin/spec-gate の表に書く。
set -euo pipefail
cd "$(dirname "$0")/.."
tag=${1:?使い方: scripts/pin-spec-gate.sh <リリースのタグ>}
repo=nonchan7720/spec-gate
base=https://github.com/$repo/releases/download/$tag
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

sha() { if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }

curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"
table=
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  a=spec-gate_$p.tar.gz
  curl -fsSL -o "$tmp/$a" "$base/$a"
  want=$(awk -v a="$a" '$2 == a { print $1 }' "$tmp/checksums.txt")
  [[ -n $want && $(sha "$tmp/$a") == "$want" ]] || { echo "$a の sha256 が checksums.txt と違う" >&2; exit 1; }
  mkdir -p "$tmp/$p" && tar -xzf "$tmp/$a" -C "$tmp/$p" spec-gate
  table+="    $p) echo $(sha "$tmp/$p/spec-gate") ;;"$'\n'
done

TAG=$tag TABLE=$table perl -0pi -e '
  s/^PINNED_VERSION=.*$/PINNED_VERSION=$ENV{TAG}/m;
  s/(pinned_sha256\(\) \{\n  case \$1 in\n).*?(  esac\n)/$1$ENV{TABLE}$2/s;
' bin/spec-gate
echo "bin/spec-gate の固定版を $tag にした。git diff bin/spec-gate で確かめてからコミットする"
