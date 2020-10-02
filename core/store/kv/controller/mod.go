package controller

import (
	"path/filepath"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/store/kv"
	"golang.org/x/xerrors"
)

type minimalController struct{}

// NewController returns a minimal controller that will setup a key/value
// database.
func NewController() node.Initializer {
	return minimalController{}
}

func (m minimalController) SetCommands(builder node.Builder) {}

func (m minimalController) OnStart(flags cli.Flags, inj node.Injector) error {
	db, err := kv.New(filepath.Join(flags.String("config"), "dela.db"))
	if err != nil {
		return xerrors.Errorf("db: %v", err)
	}

	inj.Inject(db)

	return nil
}

func (m minimalController) OnStop(inj node.Injector) error {
	var db kv.DB
	err := inj.Resolve(&db)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	err = db.Close()
	if err != nil {
		return xerrors.Errorf("while closing db: %v", err)
	}

	return nil
}
