// Package version はビルドのバージョン情報を持つ。
//
// Version は release-please がリリースごとに更新する（下の x-release-please-version マーカー）ので、
// リリースタグからのソースビルドもその版を名乗る。goreleaser のリリースビルドは
// 3 つとも -ldflags -X で上書きする。
package version

var (
	// Version は先頭の "v" を付けないセマンティックバージョン。
	Version = "0.1.0" // x-release-please-version
	// Commit はビルド元の git コミット。
	Commit = "none"
	// Date はビルド日時（RFC 3339）。
	Date = "unknown"
)
