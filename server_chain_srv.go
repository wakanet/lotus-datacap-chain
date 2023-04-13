package main

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/gwaylib/errors"
	"github.com/gwaylib/eweb"
	"github.com/ipfs/go-cidutil/cidenc"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/multiformats/go-multibase"
	"github.com/urfave/cli/v2"
)

type chainSrvHandle struct {
	repo string
	cctx *cli.Context

	nodeApi    api.FullNode
	nodeCloser jsonrpc.ClientCloser
	nodeMu     sync.Mutex
}

func (srv *chainSrvHandle) closeNodeApi() {
	if srv.nodeCloser != nil {
		srv.nodeCloser()
	}
	srv.nodeApi = nil
	srv.nodeCloser = nil
}

func (srv *chainSrvHandle) ReleaseNodeApi(shutdown bool) {
	srv.nodeMu.Lock()
	defer srv.nodeMu.Unlock()
	time.Sleep(3e9)

	if srv.nodeApi == nil {
		return
	}

	if shutdown {
		srv.closeNodeApi()
		return
	}

	ctx := lcli.ReqContext(srv.cctx)

	// try reconnection
	_, err := srv.nodeApi.Version(ctx)
	if err != nil {
		srv.closeNodeApi()
		return
	}
}

func (srv *chainSrvHandle) GetNodeApi() (api.FullNode, error) {
	srv.nodeMu.Lock()
	defer srv.nodeMu.Unlock()

	if srv.nodeApi != nil {
		return srv.nodeApi, nil
	}

	nApi, closer, err := lcli.GetFullNodeAPIV1(srv.cctx)
	if err != nil {
		srv.closeNodeApi()
		return nil, errors.As(err)
	}
	srv.nodeApi = nApi
	srv.nodeCloser = closer

	return srv.nodeApi, nil
}

func (srv *chainSrvHandle) handleChainApi(c echo.Context) error {
	nApi, err := srv.GetNodeApi()
	if err != nil {
		srv.ReleaseNodeApi(false)
		return errors.As(err)
	}
	method := c.FormValue("method")
	if len(method) == 0 {
		return c.String(403, "method not found")
	}
	params := c.FormValue("params")
	if len(params) == 0 {
		return c.String(403, "params not found")
	}
	switch method {
	case "ClientStatelessDeal":
		param := &lapi.StartDealParams{}
		if err := json.Unmarshal([]byte(params), param); err != nil {
			log.Warn(errors.As(err))
			return c.String(403, "decode params failed, is it not json format?")
		}
		dcap, err := nApi.StateVerifiedClientStatus(srv.cctx.Context, param.Wallet, types.EmptyTSK)
		if err != nil {
			log.Warn(errors.As(err, *param))
			return c.String(403, errors.As(err).Code())
		}
		isVerified := dcap != nil
		param.VerifiedDeal = isVerified
		propCid, err := nApi.ClientStatelessDeal(srv.cctx.Context, param)
		if err != nil {
			log.Warn(errors.As(err))
			return c.String(403, errors.As(err).Code())
		}
		e := cidenc.Encoder{Base: multibase.MustNewEncoder(multibase.Base32)}
		return c.JSON(200, eweb.H{
			"PropCid":  e.Encode(*propCid),
			"Verified": isVerified,
		})

	default:
		return c.String(403, "unsupported method")
	}

	panic("not reach here")
}

var runChainSrvCmd = &cli.Command{
	Name:  "chain-srv",
	Usage: "run the chain server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "repo",
			EnvVars: []string{"LOTUS_PATH"},
			Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
		},
		&cli.StringFlag{
			Name:  "listen",
			Value: ":9081",
		},
	},
	Action: func(cctx *cli.Context) error {
		listenAddr := cctx.String("listen")

		srv := &chainSrvHandle{
			repo: cctx.String("repo"),
			cctx: cctx,
		}

		// support http server
		e := eweb.Default()
		// middle ware
		e.Use(middleware.Gzip())
		// filter
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				req := c.Request()
				uri := req.URL.Path
				switch {
				case strings.HasPrefix(uri, "/hacheck"):
					return c.String(200, "1")
				}

				// TODO: auth the request
				// next route
				return next(c)
			}
		})

		// register handler
		e.POST("/chain/api", srv.handleChainApi)

		if err := e.Start(listenAddr); err != nil {
			return errors.As(err, listenAddr)
		}
		return nil
	},
}
