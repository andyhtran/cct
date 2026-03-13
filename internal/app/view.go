package app

import (
	"github.com/andyhtran/cct/internal/session"
	"github.com/andyhtran/cct/internal/tui"
)

type ViewCmd struct {
	ID string `arg:"" help:"Session ID or prefix"`
}

func (cmd *ViewCmd) Run(globals *Globals) error {
	s, err := session.FindByPrefixFull(cmd.ID)
	if err != nil {
		return err
	}

	return tui.Run(s)
}
