package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.design/x/clipboard"
	_ "modernc.org/sqlite"
)

const (
	maxHistory   = 100
	truncateLen  = 50
	pollInterval = 500 * time.Millisecond
	appID        = "com.github.mniyk.clipboard-manager"
)

type item struct {
	id      int64
	content string
}

// --- storage ---

func openDB() (*sql.DB, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "clipboard-manager")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "history.db"))
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS history (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			content    TEXT    NOT NULL UNIQUE,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`)
	return db, err
}

func dbUpsert(db *sql.DB, content string) error {
	now := time.Now().UnixNano()
	_, err := db.Exec(`
		INSERT INTO history(content, updated_at) VALUES(?,?)
		ON CONFLICT(content) DO UPDATE SET updated_at = excluded.updated_at
	`, content, now)
	if err != nil {
		return err
	}
	// 最大件数を超えた古いエントリを削除
	_, err = db.Exec(`
		DELETE FROM history WHERE id NOT IN (
			SELECT id FROM history ORDER BY updated_at DESC LIMIT ?
		)
	`, maxHistory)
	return err
}

func dbList(db *sql.DB, filter string) ([]item, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if filter == "" {
		rows, err = db.Query(
			`SELECT id, content FROM history ORDER BY updated_at DESC LIMIT ?`,
			maxHistory,
		)
	} else {
		rows, err = db.Query(
			`SELECT id, content FROM history WHERE content LIKE ? ORDER BY updated_at DESC LIMIT ?`,
			"%"+filter+"%", maxHistory,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.content); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func dbDelete(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM history WHERE id = ?`, id)
	return err
}

// --- helpers ---

// truncate collapses whitespace and limits to n runes.
func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// --- main ---

func main() {
	if err := clipboard.Init(); err != nil {
		log.Fatal("clipboard:", err)
	}

	db, err := openDB()
	if err != nil {
		log.Fatal("db:", err)
	}
	defer db.Close()

	a := app.NewWithID(appID)
	w := a.NewWindow("Clipboard Manager")
	w.Resize(fyne.NewSize(620, 520))

	var (
		mu         sync.RWMutex
		items      []item
		filter     string
		lastSeen   string // 直近の既知クリップボード内容 (再コピー時のループ防止)
		lastSeenMu sync.Mutex
	)

	// 起動時のクリップボード内容は履歴に追加しない
	if data := clipboard.Read(clipboard.FmtText); len(data) > 0 {
		lastSeen = string(data)
	}

	reload := func() {
		mu.RLock()
		f := filter
		mu.RUnlock()

		its, err := dbList(db, f)
		if err != nil {
			log.Println("query:", err)
			return
		}
		mu.Lock()
		items = its
		mu.Unlock()
	}
	reload()

	var list *widget.List

	list = widget.NewList(
		func() int {
			mu.RLock()
			defer mu.RUnlock()
			return len(items)
		},
		// 各行のテンプレートを生成
		func() fyne.CanvasObject {
			copyBtn := widget.NewButton("", nil)
			copyBtn.Importance = widget.LowImportance
			copyBtn.Alignment = widget.ButtonAlignLeading
			delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), nil)
			delBtn.Importance = widget.LowImportance
			// container.NewBorder: Objects = [copyBtn(center), delBtn(right)]
			return container.NewBorder(nil, nil, nil, delBtn, copyBtn)
		},
		// 各行をデータで更新
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			mu.RLock()
			if int(id) >= len(items) {
				mu.RUnlock()
				return
			}
			it := items[id]
			mu.RUnlock()

			c := obj.(*fyne.Container)
			copyBtn := c.Objects[0].(*widget.Button)
			delBtn := c.Objects[1].(*widget.Button)

			copyBtn.SetText(truncate(it.content, truncateLen))
			copyBtn.OnTapped = func() {
				lastSeenMu.Lock()
				lastSeen = it.content
				lastSeenMu.Unlock()
				clipboard.Write(clipboard.FmtText, []byte(it.content))
			}
			delBtn.OnTapped = func() {
				if err := dbDelete(db, it.id); err != nil {
					log.Println("delete:", err)
					return
				}
				reload()
				list.Refresh()
			}
		},
	)

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Search clipboard history...")
	searchEntry.OnChanged = func(q string) {
		mu.Lock()
		filter = q
		mu.Unlock()
		reload()
		list.Refresh()
	}

	w.SetContent(container.NewBorder(
		container.NewPadded(searchEntry),
		nil, nil, nil,
		list,
	))

	ctx, cancel := context.WithCancel(context.Background())
	w.SetOnClosed(func() {
		cancel()
		a.Quit()
	})

	// クリップボード監視ゴルーチン
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				data := clipboard.Read(clipboard.FmtText)
				text := string(data)
				if text == "" {
					continue
				}
				lastSeenMu.Lock()
				if text == lastSeen {
					lastSeenMu.Unlock()
					continue
				}
				lastSeen = text
				lastSeenMu.Unlock()

				if err := dbUpsert(db, text); err != nil {
					log.Println("upsert:", err)
					continue
				}
				reload()
				list.Refresh()
			}
		}
	}()

	w.ShowAndRun()
}
