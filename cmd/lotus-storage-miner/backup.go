package main

import (
	"fmt"
	"os"

	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/backupds"
	"github.com/filecoin-project/lotus/node/repo"
)

var backupCmd = &cli.Command{
	Name:  "backup",
	Usage: "Create node metadata backup",
	Description: `The backup command writes a copy of node metadata under the specified path

Online backups:
For security reasons, the daemon must be have LOTUS_BACKUP_BASE_PATH env var set
to a path where backup files are supposed to be saved, and the path specified in
this command must be within this base path`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "offline",
			Usage: "create backup without the node running",
		},
	},
	ArgsUsage: "[backup file path]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("expected 1 argument")
		}

		if cctx.Bool("offline") {
			return offlineBackup(cctx)
		}

		return onlineBackup(cctx)
	},
}

func offlineBackup(cctx *cli.Context) error {
	logging.SetLogLevel("badger", "ERROR") // nolint:errcheck

	repoPath := cctx.String(FlagMinerRepo)
	r, err := repo.NewFS(repoPath)
	if err != nil {
		return err
	}

	ok, err := r.Exists()
	if err != nil {
		return err
	}
	if !ok {
		return xerrors.Errorf("repo at '%s' is not initialized", cctx.String(FlagMinerRepo))
	}

	lr, err := r.Lock(repo.StorageMiner)
	if err != nil {
		return xerrors.Errorf("locking repo: %w", err)
	}
	defer lr.Close() // nolint:errcheck

	mds, err := lr.Datastore("/metadata")
	if err != nil {
		return xerrors.Errorf("getting metadata datastore: %w", err)
	}

	bds := backupds.Wrap(mds)

	fpath, err := homedir.Expand(cctx.Args().First())
	if err != nil {
		return xerrors.Errorf("expanding file path: %w", err)
	}

	out, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return xerrors.Errorf("opening backup file %s: %w", fpath, err)
	}

	if err := bds.Backup(out); err != nil {
		if cerr := out.Close(); cerr != nil {
			log.Errorw("error closing backup file while handling backup error", "closeErr", cerr, "backupErr", err)
		}
		return xerrors.Errorf("backup error: %w", err)
	}

	if err := out.Close(); err != nil {
		return xerrors.Errorf("closing backup file: %w", err)
	}

	return nil
}

func onlineBackup(cctx *cli.Context) error {
	api, closer, err := lcli.GetStorageMinerAPI(cctx)
	if err != nil {
		return xerrors.Errorf("getting api: %w (if the node isn't running you can use the --offline flag)", err)
	}
	defer closer()

	err = api.CreateBackup(lcli.ReqContext(cctx), cctx.Args().First())
	if err != nil {
		return err
	}

	fmt.Println("Success")

	return nil
}