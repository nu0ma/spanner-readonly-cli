# spanner-ro — Read-Only Spanner CLI 設計

日付: 2026-07-07
ステータス: 承認済み

## 目的

[nu0ma/spanner-readonly-mcp](https://github.com/nu0ma/spanner-readonly-mcp) の CLI 版。
Cloud Spanner に対して**構造的に書き込み不可能**な読み取り専用クエリツールを提供する。
主な利用者は AI エージェントとシェルスクリプト。

## Read-only 保証(核心)

すべてのクエリを Go クライアントの read-only snapshot transaction
(`client.Single()` = single-use read-only transaction) 内で実行する。
`spanner.ReadOnlyTransaction` 型は書き込み系メソッドを一切持たないため、
正規表現による DML ブロック等に頼らず**型レベルで read-only が保証**される。
DML/DDL を渡した場合は Spanner サーバー側がエラーを返す。

## コマンド体系

MCP 版の4ツールと1対1対応:

| コマンド | 対応する MCP ツール | 内容 |
|---|---|---|
| `spanner-ro query "SELECT ..."` | execute_query | 任意の SELECT を実行。`--param name=value`(STRING 型、繰り返し可) |
| `spanner-ro tables` | list_tables | information_schema からユーザーテーブル一覧 |
| `spanner-ro describe <table>` | describe_table | カラム定義(名前・型・NULL 可否・序数) |
| `spanner-ro indexes [--table <t>]` | list_indexes | インデックス一覧(テーブルで絞り込み可) |

## 接続設定

優先順: フラグ > 環境変数(MCP 版と互換)。

- `--project` / `SPANNER_PROJECT`
- `--instance` / `SPANNER_INSTANCE`
- `--database` / `SPANNER_DATABASE`
- `--endpoint` / `SPANNER_ENDPOINT`: Spanner Omni 等セルフホスト環境向け。
  設定時は認証なし・平文 gRPC(`IsExperimentalHost`)で接続する。
  Omni では project / instance は共に `default` 固定
- `SPANNER_EMULATOR_HOST`(Go クライアントがネイティブ対応)
- 認証は Application Default Credentials

## 出力

エージェントが読みやすい JSON を stdout に1オブジェクトで出力:

```json
{"columns": ["id", "name"], "rows": [{"id": 1, "name": "foo"}], "rowCount": 1}
```

型変換規則(すべて JSON 安全):

- INT64 → JSON 数値(json.Number、精度損失なし)
- FLOAT64/FLOAT32 → 数値 / BOOL → 真偽値 / STRING → 文字列
- BYTES → base64 文字列
- NUMERIC / TIMESTAMP / DATE → 文字列
- JSON → 生の JSON(json.RawMessage)
- ARRAY → 配列、STRUCT → オブジェクト、NULL → null

エラーは stderr に `{"error": "..."}` + exit code 1。

## その他

- クエリタイムアウト: 30秒デフォルト、`--timeout`(Go duration 形式)で変更可
- 依存は `cloud.google.com/go/spanner` のみ。CLI パースは標準 `flag`
- 構成: `main.go`(dispatch) / `internal/cli`(実行・シリアライズ)
- テスト: シリアライズ・フラグ解決のユニットテスト。E2E テストはローカルの
  Spanner Omni に対して実行(`SPANNER_ENDPOINT` 未設定時はスキップ)。
  使い捨てデータベースを作成し、4コマンド + DML/DDL 拒否を検証して削除する
