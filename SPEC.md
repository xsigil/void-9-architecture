# SPEC.md: Void-9 Architecture CLI Specification

## 1. Overview
`void` は、Linuxカーネルプリミティブ（名前空間、chroot/pivot_root、バインドマウント）を活用し、不要なランタイムや重厚なデーモンを一切排した極小のコンテナレス隔離実行・配布環境を提供するUnixミニマリズムツールである。

本仕様は `void-9` アーキテクチャにおける4つのコアサブコマンドのインターフェース、責務、内部動作要件を定義する。

---

## 2. Command Interfaces

```bash
void scaffold <app dir>
void ldd <linked executable> <lib dir>
void jail [--bind=<src>:<dst>[:ro],...] --lib <lib dir> <exec> [args...]
void export [--bind=<src>:<dst>[:ro],...] --lib <lib dir> --output <tarball|dir> <exec>

```

---

## 3. Subcommand Specifications

### 3.1. `void scaffold <app dir>`

新規アプリケーション実行環境の標準骨格ディレクトリツリーを生成する。

* **引数:**
* `<app dir>`: 作成対象のベースディレクトリパス


* **生成構造:**
```text
<app dir>/
├── bin/          # 実行可能バイナリ配置領域
├── lib/          # void ldd によって収集される共有ライブラリ (.so) 配置領域
├── rootfs/       # jail 実行時に chroot/pivot_root の基点となるルートファイルシステム
│   ├── dev/
│   ├── proc/
│   ├── etc/
│   └── tmp/
└── Voidfile      # 構成・ビルド・実行引数定義（任意）

```



---

### 3.2. `void ldd <linked executable> <lib dir>`

対象の動的リンクバイナリ（ELF）を解析し、chroot環境下での自律実行に必須な共有ライブラリ群をインクリメンタルに追跡・収集して `<lib dir>` に複製・配置する。

* **引数:**
* `<linked executable>`: 解析対象の実行可能バイナリ
* `<lib dir>`: ライブラリの配置先ディレクトリ


* **動作要件:**
1. **動的リンカ／ローダーの自動収集（必須）**:
* ELFヘッダ（`PT_INTERP` / Program Interpreter、例: `/lib64/ld-linux-x86-64.so.2`）を抽出し、`<lib dir>` 内の対応するパス階層へ必ず複製する。


2. **再帰的依存解決（インクリメンタル収集）**:
* `DT_NEEDED` タグを再帰的にトラバースし、依存するすべての `.so` を特定する。
* 既に `<lib dir>` に存在するファイルはスキップし、新規・更新分のみを複製する。


3. **シンボリックリンクの保存**:
* ソナー名（soname）およびバージョン付き実体ファイルの両方を保持し、シンボリックリンク関係を破綻させずに複製する。





---

### 3.3. `void jail`

指定したバイナリを、極小の分離環境（Linux Namespaces + chroot/pivot_root）上で直接実行する。

* **構文:**
```bash
void jail [--bind=<src>:<dst>[:ro],...] --lib <lib dir> <exec> [args...]

```


* **オプション:**
* `--lib <lib dir>` (必須): `void ldd` で生成されたライブラリディレクトリ。chroot内のシステムライブラリパス（`/lib`, `/lib64`, `/usr/lib` 等）としてマウントまたは統合される。
* `--bind=<mappings>` (任意): カンマ区切りのバインドマウント指定。
* 形式: `<src>:<dst>[:ro]`
* `:ro` フラグが付与されている場合は読み取り専用（MS_RDONLY）でバインドする。


* **内部暗黙処理（ベースライン保証）:**
1. **Namespace 分離**: `CLONE_NEWNS`（マウント名前空間）および `CLONE_NEWPID`（PID名前空間）をデフォルトで適用。
2. **疑似ファイルシステム自動マウント**:
* `/proc`: 分離されたPID名前空間用のprocfsを自動マウント。
* `/dev`: 最小限の疑似デバイス（`/dev/null`, `/dev/urandom`, `/dev/zero`）を提供。
* `/tmp`: tmpfs をマウント（メモリ内即時破棄）。


3. **エグゼクティブ切り替え**: ルート切り替え完了後、指定された `<exec>` に直接 `execve` し、PID 1 または監視プロセスとして起動する。

---

### 3.4. `void export`

`jail` でテスト・実行可能な環境一式を、他ホストへそのまま持ち出し可能なスタンドアロン配布物（tarballまたはディレクトリ）として出力する。

* **構文:**
```bash
void export [--bind=<src>:<dst>[:ro],...] --lib <lib dir> --output <tarball|dir> <exec>

```

* **オプション:**
* `--lib <lib dir>`: ライブラリディレクトリ。
* `--bind=<mappings>`: 環境内に固定展開すべき設定ファイルや静的アセット（実体ファイルとして展開領域にコピーされる）。
* `--output <path>`: 出力先の tar.gz アーカイブパス、またはルートディレクトリパス。
* `<exec>`: エントリポイントバイナリ。


* **動作要件:**
* 生成されたアーカイブ/ディレクトリは、外部依存ゼロのルートファイルシステム（rootfs）となり、`void` 単体または標準の `chroot` / `unshare` のみで即座に起動可能であること。

---

## 4. Operating Constraints & OPSEC Rules

* **Statelessness**: ホストマシンのストレージへの永続書き込みを最小化し、一時データはすべて tmpfs 上で完結させること。
* **Zero Residuals**: `jail` 終了時、残存プロセスおよびマウントポイントを完全にクリーンアップすること。
