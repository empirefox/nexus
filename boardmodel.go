package nexus

import (
	"fmt"

	"github.com/empirefox/firmata-table/pintable"
	"github.com/empirefox/firmata-table/stm32f407vet6"
)

type boardModel struct {
	name  string
	model *pintable.Board
	pins  map[string]byte
}

func newBoardModel(modelName string) (*boardModel, error) {
	bm := boardModel{
		name: modelName,
	}
	switch modelName {
	case "stm32f407vet6":
		bm.model = stm32f407vet6.Board
	default:
		return nil, fmt.Errorf("Board model not found: %s", modelName)
	}

	bm.pins = make(map[string]byte, bm.model.PinEnd)
	for i := 0; i < bm.model.PinEnd; i++ {
		bm.pins[bm.model.Stringer(i).String()] = byte(i)
	}
	return &bm, nil
}
