package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/foqerhk/runeverything/internal/agentchat"
	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/idemirror"
)

// cmdIDEMirror runs inside a KoKo PTY:
//
//	ide-mirror [--lang zh|en] cursor <composerId>
//	ide-mirror [--lang zh|en] --new <folder> cursor
func cmdIDEMirror(args []string) int {
	log.SetOutput(io.Discard)
	fs := flag.NewFlagSet("ide-mirror", flag.ContinueOnError)
	lang := fs.String("lang", "", "zh or en (defaults to the computer language)")
	newIn := fs.String("new", "", "create a new chat in this project folder")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	composerID := ""
	switch {
	case len(rest) == 2 && rest[0] == "cursor" && *newIn == "":
		composerID = rest[1]
	case len(rest) == 1 && rest[0] == "cursor" && *newIn != "":
	default:
		fmt.Fprintln(os.Stderr, "usage: runeverything ide-mirror [--lang zh|en] [--new <folder>] cursor [<composerId>]")
		return 2
	}
	zh := i18n.IsZh()
	switch *lang {
	case "zh":
		zh = true
	case "en":
		zh = false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	err = idemirror.Run(idemirror.Options{
		ComposerID:  composerID,
		NewInFolder: *newIn,
		StateDB:     agentchat.CursorIDEStateDB(home),
		Zh:          zh,
	})
	if err != nil {
		fmt.Fprintf(os.Stdout, "\r\n%v\r\n", err)
		return 1
	}
	return 0
}
