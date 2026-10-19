package db

import "io/fs"

func fsSub() (fs.FS, error) { return fs.Sub(migrations, "migrations") }
