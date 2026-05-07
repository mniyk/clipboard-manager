# Clipboard Manager

Go + Fyne で作ったシンプルなクリップボード履歴マネージャー (Windows 向け)。

## 機能

- **自動監視**: テキストのコピーを 500ms ごとに検知して履歴に追加
- **履歴一覧**: 直近 100 件をリスト表示 (新しい順)
- **再コピー**: 履歴アイテムをクリックしてクリップボードに再コピー
- **検索**: 上部の検索ボックスで履歴をリアルタイム絞り込み
- **個別削除**: 各アイテム右側の削除ボタンで削除
- **永続化**: SQLite に保存し、再起動後も履歴を維持

## ビルド要件

| ツール | 備考 |
|--------|------|
| Go 1.21+ | [golang.org](https://golang.org/dl/) |
| GCC (MinGW-w64) | Fyne が CGO を必要とするため必須 |

### MinGW-w64 のインストール (MSYS2 経由)

```powershell
# MSYS2 をインストール
winget install MSYS2.MSYS2

# MinGW GCC をインストール
C:\msys64\usr\bin\bash.exe -lc "pacman -S --noconfirm mingw-w64-x86_64-gcc"
```

## ビルド

```powershell
# MinGW を PATH に追加してビルド
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"

git clone https://github.com/mniyk/clipboard-manager
cd clipboard-manager
go build -o clipboard-manager.exe .
```

## 実行

```powershell
# GCC DLL が必要なため MinGW の bin を PATH に含める
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
.\clipboard-manager.exe
```

> **ヒント**: スタートアップに登録する場合は、上記の PATH 設定を含む `.bat` ファイルを作成してください。

## データ保存先

履歴データベースは OS のユーザー設定ディレクトリに保存されます。

```
%APPDATA%\clipboard-manager\history.db
```

## 技術スタック

| 項目 | 内容 |
|------|------|
| 言語 | Go |
| GUI | [Fyne v2](https://fyne.io/) |
| クリップボード | [golang.design/x/clipboard](https://pkg.go.dev/golang.design/x/clipboard) |
| 永続化 | [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (Pure Go SQLite) |
