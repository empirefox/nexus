package nexus

import "fmt"

type BoardInfo struct {
	Model string
	Addr  string
	Name  string
}

type PinInfo struct {
	Board string
	ID    string
	Name  string
}

type GroupInfo struct {
	Name string
	Pins []PinInfo
}

type Config struct {
	Dev        bool
	BoardInfos []BoardInfo
	GroupInfos []GroupInfo
}

func (c Config) ToProtobuf() ([]*ProtoGroup, error) {
	var err error
	type boardWithModel struct {
		idx uint32
		bm  *boardModel
	}
	models := make(map[string]*boardModel)
	boards := make(map[string]boardWithModel, len(c.BoardInfos))

	for i, bi := range c.BoardInfos {
		bm, ok := models[bi.Model]
		if !ok {
			bm, err = newBoardModel(bi.Model)
			if err != nil {
				return nil, err
			}
			models[bi.Model] = bm
		}
		boards[bi.Name] = boardWithModel{uint32(i), bm}
	}

	groups := make([]*ProtoGroup, len(c.GroupInfos))
	for i, gi := range c.GroupInfos {
		pins := make([]*ProtoPin, len(gi.Pins))
		for j, pi := range gi.Pins {
			board, ok := boards[pi.Board]
			if !ok {
				return nil, fmt.Errorf("Config: Board name not found: %s", pi.Board)
			}
			pid, ok := board.bm.pins[pi.ID]
			if !ok {
				return nil, fmt.Errorf("Config: Pin ID not found: %s", pi.ID)
			}
			pins[j] = &ProtoPin{
				Board: board.idx,
				Id:    uint32(pid),
				Name:  pi.Name,
			}
		}
		groups[i] = &ProtoGroup{
			Name: gi.Name,
			Pins: pins,
		}
	}
	return groups, nil
}
