package main

import (
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/lotuslog"
)

var log = logging.Logger("main")

const (
	FlagMinerRepo = "miner-repo"
)

func main() {
	lotuslog.SetupLogLevels()

	cmds := []*cli.Command{
		runChainSrvCmd,
	}

	app := &cli.App{
		Name:    "lotus-datacap-chain",
		Usage:   "lotus datacap of chain",
		Version: build.UserVersion(),
		Flags: []cli.Flag{
			cliutil.FlagVeryVerbose,
		},
		Commands: cmds,
	}
	app.Setup()
	lcli.RunApp(app)
}
