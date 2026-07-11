// Command jobcron runs the 신입 IT Job Briefing — a local web app that
// scrapes several Korean job boards (점핏, 랠릿, 데모데이, the 그리팅 Korean-ATS
// tenants, the Greenhouse company boards 당근·크래프톤·몰로코·센드버드, and optionally
// 워크넷), scores new-grad IT job postings against a user profile, and renders a
// calm one-page daily briefing.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/browser"

	"github.com/ohchanwu/jobcron/internal/appdata"
	"github.com/ohchanwu/jobcron/internal/config"
	"github.com/ohchanwu/jobcron/internal/scraper"
	"github.com/ohchanwu/jobcron/internal/scraper/demoday"
	"github.com/ohchanwu/jobcron/internal/scraper/greenhouse"
	"github.com/ohchanwu/jobcron/internal/scraper/greeting"
	"github.com/ohchanwu/jobcron/internal/scraper/jumpit"
	"github.com/ohchanwu/jobcron/internal/scraper/rallit"
	"github.com/ohchanwu/jobcron/internal/scraper/worknet"
	"github.com/ohchanwu/jobcron/internal/server"
	"github.com/ohchanwu/jobcron/internal/storage"
)

// version is the build version, overridden by GoReleaser via -ldflags.
var version = "dev"

func main() {
	cfg, err := config.Load(os.Args[1:], environMap(os.Environ()))
	if err != nil {
		log.Fatalf("jobcron: %v", err)
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("jobcron: locate user config dir: %v", err)
	}
	if err := prepareApplicationData(configRoot); err != nil {
		log.Fatalf("jobcron: %v", err)
	}
	if cfg.ShowVersion {
		fmt.Println("jobcron", version)
		return
	}

	store, err := openConfiguredStore(cfg)
	if err != nil {
		log.Fatalf("jobcron: %v", err)
	}
	defer store.Close()

	// 당근·크래프톤·몰로코·센드버드 all ride the shared Greenhouse adapter — each
	// is one company board registered as its own source (own badge + toggle).
	// 그리팅 is a single aggregator source over a curated Korean-ATS tenant list.
	sources := []scraper.Scraper{
		jumpit.New(), rallit.New(), demoday.New(), greeting.New(),
		greenhouse.Daangn(), greenhouse.Krafton(), greenhouse.Moloco(), greenhouse.Sendbird(),
	}
	if cfg.WorknetKey != "" {
		wn, err := worknet.New(cfg.WorknetKey)
		if err != nil {
			log.Fatalf("jobcron: %v", err)
		}
		sources = append(sources, wn)
	} else {
		fmt.Println("jobcron: 워크넷 key가 없어 워크넷 출처는 꺼져 있어요",
			"(점핏·랠릿·데모데이·그리팅·당근·크래프톤·몰로코·센드버드는 켜져 있어요).",
			"워크넷도 보려면 --worknet-api-key 플래그나 JOBCRON_WORKNET_KEY 환경변수를 설정하세요.")
	}
	srv := server.New(store, sources...)
	srv.SetSessionSecret(cfg.SessionSecret)
	srv.SetDemoMode(cfg.Demo)
	srv.SetProductionMode(cfg.Production)
	srv.SetAdminToken(cfg.AdminToken)
	srv.SetProxySecret(cfg.ProxySecret)
	// Wire BYOK AI from the saved profile + ai_keys.json. Non-fatal: any error
	// (or simply no key configured) leaves AI off and the briefing falls back to
	// the v1.5 offline scoring. The user enables AI on /profile.
	if err := srv.ReconfigureAI(context.Background()); err != nil {
		log.Printf("jobcron: AI 설정을 불러오지 못해 일반 점수로 시작해요: %v", err)
	}
	// Heal any posting left unscored by an interrupted scrape (e.g. a crash or
	// restart between insert and the end-of-run scoring) so it never renders as
	// a blank card. Runs after ReconfigureAI so cached AI deltas merge in too;
	// it never calls the provider, so there is no startup cost or token spend.
	// Non-fatal: a transient error just defers healing to the next scrape/save.
	if _, err := srv.RescoreAll(context.Background()); err != nil {
		log.Printf("jobcron: 시작 시 점수 재계산을 건너뛰었어요: %v", err)
	}
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()
	if cfg.SchedulerEnabled {
		if _, err := server.StartScheduler(appCtx, server.SchedulerConfig{
			Server:          srv,
			DailyScrapeTime: cfg.DailyScrapeTime,
		}); err != nil {
			log.Fatalf("jobcron: scheduler: %v", err)
		}
		log.Printf("jobcron: 매일 %s KST에 자동 스크랩을 실행해요.", cfg.DailyScrapeTime)
	}

	ln, addr, err := listen(cfg.Host, cfg.Port)
	if err != nil {
		log.Fatalf("jobcron: %v", err)
	}
	url := "http://" + addr
	fmt.Printf("jobcron: %s 에서 실행 중입니다. 종료하려면 Ctrl+C를 누르세요.\n", url)

	if !cfg.NoOpen {
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
		fmt.Println("\njobcron: 종료 중...")
		appCancel()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	}()
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("jobcron: %v", err)
	}
}

// openConfiguredStore opens the database from DATABASE_URL in production, or
// from the local DB path/default location outside production.
func openConfiguredStore(cfg config.Config) (*storage.Store, error) {
	if cfg.DatabaseURL != "" {
		return storage.OpenPostgres(cfg.DatabaseURL)
	}
	if cfg.DBPath != "" {
		return storage.OpenAt(cfg.DBPath)
	}
	return storage.Open()
}

func prepareApplicationData(configRoot string) error {
	return appdata.Prepare(configRoot)
}

// listen binds host on the preferred port, falling back to the next ten
// if it is busy. It returns the listener and the bound "host:port" address.
func listen(host string, preferred int) (net.Listener, string, error) {
	for p := preferred; p <= preferred+10; p++ {
		addr := net.JoinHostPort(host, fmt.Sprintf("%d", p))
		if ln, err := net.Listen("tcp", addr); err == nil {
			return ln, addr, nil
		}
	}
	return nil, "", fmt.Errorf("no free port in %d..%d", preferred, preferred+10)
}

func environMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, item := range environ {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		env[key] = value
	}
	return env
}
