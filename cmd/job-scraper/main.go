// Command job-scraper runs the 신입 IT Job Briefing — a local web app that
// scrapes 점핏, scores new-grad IT job postings against a user profile, and
// renders a calm one-page daily briefing.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pkg/browser"

	"github.com/ohchanwu/job-scraper/internal/scraper"
	"github.com/ohchanwu/job-scraper/internal/scraper/alio"
	"github.com/ohchanwu/job-scraper/internal/scraper/jumpit"
	"github.com/ohchanwu/job-scraper/internal/scraper/naver"
	"github.com/ohchanwu/job-scraper/internal/scraper/rallit"
	"github.com/ohchanwu/job-scraper/internal/scraper/worknet"
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
	worknetKey := flag.String("worknet-api-key", os.Getenv("JOBSCRAPER_WORKNET_KEY"),
		"워크넷 OpenAPI key (free at data.go.kr). Disables the 워크넷 source when empty.")
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

	sources := []scraper.Scraper{jumpit.New(), rallit.New(), naver.New(), alio.New()}
	if *worknetKey != "" {
		wn, err := worknet.New(*worknetKey)
		if err != nil {
			log.Fatalf("job-scraper: %v", err)
		}
		sources = append(sources, wn)
	} else {
		fmt.Println("job-scraper: 워크넷 key가 설정되지 않아 점핏만 활성화돼요.",
			"전체 출처를 보려면 --worknet-api-key 플래그나 JOBSCRAPER_WORKNET_KEY 환경변수를 설정하세요.")
	}
	srv := server.New(store, sources...)

	ln, addr, err := listen(*port)
	if err != nil {
		log.Fatalf("job-scraper: %v", err)
	}
	url := "http://" + addr
	fmt.Printf("job-scraper: %s 에서 실행 중입니다. 종료하려면 Ctrl+C를 누르세요.\n", url)

	if !*noOpen {
		_ = browser.OpenURL(url) // best effort — failure is non-fatal
	}

	// Graceful shutdown on Ctrl+C / SIGTERM. Without this, http.Serve never
	// returns and the process dies on the signal with exit 128+N — which the
	// shipping binary should not do (the user expects clean termination, and
	// scripts that run the binary do too). The five-second budget lets any
	// in-flight scrape SSE stream finish its current frame.
	httpSrv := &http.Server{Handler: srv.Handler()}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		fmt.Println("\njob-scraper: 종료 중...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	}()
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
