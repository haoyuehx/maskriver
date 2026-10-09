// Command fixture creates only .local/m1a-synthetic.db from invented data.
// This is a test-environment tool, not the MaskRiver CLI or database adapter.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/haoyuehx/maskriver/internal/testenv"
	_ "modernc.org/sqlite"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 1 {
		return fmt.Errorf("fixture takes no arguments; run from MaskRiver root")
	}
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("run from MaskRiver root")
	}
	if err := os.MkdirAll(".local", 0700); err != nil {
		return fmt.Errorf("cannot create local directory")
	}
	info, err := os.Lstat(".local")
	if err != nil || !info.IsDir() {
		return fmt.Errorf(".local must be a real directory, not a symlink")
	}
	path, err := filepath.Abs(".local/m1a-synthetic.db")
	if err != nil {
		return fmt.Errorf("invalid fixture path")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("fixture already exists or cannot be created; refusing overwrite")
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("fixture file close failed")
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "rw")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return fmt.Errorf("sqlite open failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = testenv.Seed(ctx, db, "sqlite"); err != nil {
		return err
	}
	fmt.Println("Created .local/m1a-synthetic.db: 25 invented people, 25 composite-key links, 1 keyless row; no upstream data copied.")
	return nil
}
