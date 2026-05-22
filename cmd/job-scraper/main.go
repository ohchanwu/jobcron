// Command job-scraper runs the 신입 IT Job Briefing — a local web app that
// scrapes 점핏, scores new-grad IT job postings against a user profile, and
// renders a calm one-page daily briefing.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/pkg/browser"

	"github.com/ohchanwu/job-scraper/internal/scraper/jumpit"
	"github.com/ohchanwu/job-scraper/internal/server"
	"github.com/ohchanwu/job-scraper/internal/storage"
)

// version is the build version, overridden by GoReleaser via -ldflags.
var version = "dev"

func main() {
	port := flag.Int("port", 7777, "preferred port; the next ten are tried if it is busy")
	noOpen := flag.Bool("no-open", false, "do not open a browser window on startup")
	dbPath := flag.String("db", "", "database file path (default: under the OS config dir)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("job-scraper", version)
		return
	}

	store, err := openStore(*dbPath)
	if err != nil {
		log.Fatalf("job-scraper: %v", err)
	}
	defer store.Close()

	srv := server.New(store, jumpit.New())

	ln, addr, err := listen(*port)
	if err != nil {
		log.Fatalf("job-scraper: %v", err)
	}
	url := "http://" + addr
	fmt.Printf("job-scraper: %s 에서 실행 중입니다. 종료하려면 Ctrl+C를 누르세요.\n", url)

	if !*noOpen {
		_ = browser.OpenURL(url) // best effort — failure is non-fatal
	}
	if err := http.Serve(ln, srv.Handler()); err != nil {
		log.Fatalf("job-scraper: %v", err)
	}
}

// openStore opens the database at path, or at the default OS-config-dir
// location when path is empty.
func openStore(path string) (*storage.Store, error) {
	if path != "" {
		return storage.OpenAt(path)
	}
	return storage.Open()
}

// listen binds 127.0.0.1 on the preferred port, falling back to the next ten
// if it is busy. It returns the listener and the bound "host:port" address.
func listen(preferred int) (net.Listener, string, error) {
	for p := preferred; p <= preferred+10; p++ {
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		if ln, err := net.Listen("tcp", addr); err == nil {
			return ln, addr, nil
		}
	}
	return nil, "", fmt.Errorf("no free port in %d..%d", preferred, preferred+10)
}
