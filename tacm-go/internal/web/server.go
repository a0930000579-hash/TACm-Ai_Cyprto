package web

import (
	"embed"
	"io/fs"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/flosch/pongo2/v6"

	"tacm/internal/wallet"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// Server 為 TAC 鏈 Web 服務。
type Server struct {
	ds        DataSource
	tplSet    *pongo2.TemplateSet
	mux       *http.ServeMux
	community map[string]string
	// M64：全網監控探測的 seed 列表（New 時拷貝自 serverSeeds；測試可注入替身）。
	seedURLs []SeedInfo
	// M65：網路段（mainnet | testnet）——決定 seed 列表與加入指令的展示。
	network string
}

// New 構造 Web 服務（mainnet 網段）：加載嵌入模板、註冊 filter 與路由。
func New(ds DataSource) (*Server, error) {
	return NewWithNetwork(ds, "mainnet")
}

// NewWithNetwork 構造 Web 服務（M65）：依網段（mainnet/testnet）選擇 seed 列表與展示。
func NewWithNetwork(ds DataSource, network string) (*Server, error) {
	tplRoot, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		return nil, err
	}
	tplSet := pongo2.NewSet("web", pongo2.NewFSLoader(tplRoot))
	registerFilters()

	if network == "" {
		network = "mainnet"
	}
	s := &Server{ds: ds, tplSet: tplSet, mux: http.NewServeMux(), network: network}
	s.seedURLs = append([]SeedInfo(nil), s.serverSeeds()...)
	// 社群連結（env 配置：TAC_COMMUNITY_TELEGRAM / TAC_COMMUNITY_X / TAC_COMMUNITY_DISCORD / TAC_COMMUNITY_SITE）。
	s.community = map[string]string{
		"telegram": os.Getenv("TAC_COMMUNITY_TELEGRAM"),
		"x":        os.Getenv("TAC_COMMUNITY_X"),
		"discord":  os.Getenv("TAC_COMMUNITY_DISCORD"),
		"site":     os.Getenv("TAC_COMMUNITY_SITE"),
	}
	s.routes()
	return s, nil
}

func registerFilters() {
	fns := map[string]pongo2.FilterFunction{
		"shortaddr": func(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
			addr := in.String()
			if len(addr) > 14 {
				return pongo2.AsValue(addr[:6] + "…" + addr[len(addr)-4:]), nil
			}
			return in, nil
		},
		"negdelta": func(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
			neg := len(in.String()) > 0 && in.String()[0] == '-'
			return pongo2.AsValue(neg), nil
		},
		"trunc64": func(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
			t := in.String()
			if len(t) > 64 {
				t = t[:64] + "…"
			}
			return pongo2.AsValue(t), nil
		},
		"trunc40": func(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
			t := in.String()
			if len(t) > 40 {
				t = t[:40] + "…"
			}
			return pongo2.AsValue(t), nil
		},
		"tshort": func(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
			ts := in.Integer()
			if ts <= 0 {
				return pongo2.AsValue("-"), nil
			}
			return pongo2.AsValue(time.Unix(int64(ts), 0).Format("01-02 15:04:05")), nil
		},
		"timefmt": func(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
			ts := in.Integer()
			if ts <= 0 {
				return pongo2.AsValue("-"), nil
			}
			return pongo2.AsValue(time.Unix(int64(ts), 0).Format("2006-01-02 15:04:05")), nil
		},
		// M62：鏈上金額（raw 最小單位）→ 十進制（如 1000000000000000000 → 1）。
		"fmtamt": func(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
			v, ok := new(big.Int).SetString(in.String(), 10)
			if !ok {
				return in, nil
			}
			return pongo2.AsValue(wallet.FormatAmountBig(v)), nil
		},
	}
	for name, fn := range fns {
		if pongo2.FilterExists(name) {
			_ = pongo2.ReplaceFilter(name, fn)
		} else {
			_ = pongo2.RegisterFilter(name, fn)
		}
	}
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.handleIndex)
	// M62：公開區塊瀏覽器（公鏈門面）——鏈統計＋搜尋＋最新區塊/交易。
	s.mux.HandleFunc("GET /explorer", s.handleExplorer)
	s.mux.HandleFunc("GET /api/search", s.handleSearchAPI)
	// M63：節點加入指引＋公開 seed 列表（去中心化多節點入口）。
	s.mux.HandleFunc("GET /join", s.handleJoin)
	s.mux.HandleFunc("GET /api/peers", s.handlePeersAPI)
	// M64：節點健康度監控（全網狀態統一視界）。
	s.mux.HandleFunc("GET /network", s.handleNetwork)
	s.mux.HandleFunc("GET /api/network", s.handleNetworkAPI)
	s.mux.HandleFunc("GET /block/{height}", s.handleBlock)
	s.mux.HandleFunc("GET /address/{address}", s.handleAddress)
	s.mux.HandleFunc("GET /tx/{hash}", s.handleTx)
	s.mux.HandleFunc("GET /wallet", s.handleWallet)
	s.mux.HandleFunc("GET /exchange", s.handleExchange)
	s.mux.HandleFunc("GET /dashboard", s.handleDashboard)
	s.mux.HandleFunc("GET /mining", s.handleMining)
	s.mux.HandleFunc("GET /community", s.handleCommunity)
	s.mux.HandleFunc("GET /governance", s.handleGovernance)
	s.mux.HandleFunc("GET /defi", s.handleDefi)
	s.mux.HandleFunc("GET /c2c", s.handleC2C)
	s.mux.HandleFunc("GET /tokens", s.handleTokens)
	s.mux.HandleFunc("GET /dex", s.handleDex)
	s.mux.HandleFunc("GET /auth/login", s.handleAuthLoginPage)
	s.mux.HandleFunc("GET /auth/register", s.handleAuthRegisterPage)
	// M39：節點健康檢查與運行指標（商業營運/監控告警必備）。
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/metrics", s.handleMetrics)
	// 白皮書（中/英）——直接服務嵌入 static 成品（TAC 自主智能鏈，無第三方品牌）。
	s.mux.HandleFunc("GET /whitepaper", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/whitepaper_zh.html", http.StatusFound)
	})
	s.mux.HandleFunc("GET /whitepaper-en", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/whitepaper_en.html", http.StatusFound)
	})
	// M41：技術黃皮書（完整技術規格，中英雙語內容單頁）。
	s.mux.HandleFunc("GET /yellowpaper", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/yellowpaper_en.html", http.StatusFound)
	})

	staticRoot, _ := fs.Sub(staticFS, "static")
	fileServer := http.FileServer(http.FS(staticRoot))
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))
}

// Handler 返回可掛載的 HTTP 處理器。
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) render(w http.ResponseWriter, name string, ctx pongo2.Context) {
	// M38：HTML 一律不緩存——避免瀏覽器/Service Worker 快取舊版頁面導致「功能沒更新」。
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	tpl, err := s.tplSet.FromCache(name)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	ctx["status"] = s.ds.Status()
	ctx["community"] = s.community
	if err := tpl.ExecuteWriter(ctx, w); err != nil {
		http.Error(w, "render error: "+err.Error(), http.StatusInternalServerError)
	}
}
