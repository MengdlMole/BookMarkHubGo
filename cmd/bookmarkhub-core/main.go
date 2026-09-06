package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	appserver "bookmarkhubgo/internal/server"
	"bookmarkhubgo/internal/storage"
)

var version = "dev"

func main() {
	home := flag.String("home", ".", "portable BookmarkHub home directory")
	syncDir := flag.String("sync-dir", "", "override bookmark sync directory")
	port := flag.Int("port", 17836, "local HTTP port")
	noBrowser := flag.Bool("no-browser", false, "do not open the management page")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	store, err := storage.Open(*home, *syncDir, *port)
	if err != nil {
		log.Fatal(err)
	}
	settings := store.Settings()
	appserver.Version = version
	address := fmt.Sprintf("127.0.0.1:%d", settings.Port)
	url := "http://" + address + "/"
	server := &http.Server{Addr: address, Handler: appserver.New(store).Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	if !*noBrowser {
		go func() {
			time.Sleep(350 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				log.Printf("open %s manually (%v)", url, err)
			}
		}()
	}
	log.Printf("BookmarkHub %s listening at %s", version, url)
	log.Printf("Sync directory: %s", settings.SyncDir)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Start()
}
