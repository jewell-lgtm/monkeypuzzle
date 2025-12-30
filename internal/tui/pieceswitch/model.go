package pieceswitch

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

type Model struct {
	Pieces    []piece.PieceListItem
	Selected  int
	Cancelled bool
}

func New(pieces []piece.PieceListItem) Model {
	return Model{
		Pieces:   pieces,
		Selected: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}
