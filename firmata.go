package nexus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/empirefox/firmata"
)

const (
	DefaultFirmataTimeout = time.Second * 10
)

var (
	ErrClosed     = errors.New("Firmatas closed")
	ErrConnclosed = errors.New("Request conn closed")
)

type onBoardFn func(board *firmata.Firmata, connected chan struct{})

type job struct {
	board uint
	fn    onBoardFn
}

type Firmata struct {
	idx       uint
	info      BoardInfo
	f         *firmata.Firmata
	connected chan struct{}
}

func NewFirmata(idx uint, info BoardInfo) *Firmata {
	return &Firmata{
		idx:       idx,
		info:      info,
		f:         nil,
		connected: make(chan struct{}),
	}
}

type Firmatas struct {
	boards       []*Firmata
	jobs         chan job
	boardClosed  chan uint
	boardCreated chan *Firmata
	done         chan struct{}
}

func NewFirmatas(infos []BoardInfo) *Firmatas {
	boards := make([]*Firmata, len(infos))
	for i := range boards {
		boards[i] = NewFirmata(uint(i), infos[i])
	}
	fs := &Firmatas{
		boards:       boards,
		jobs:         make(chan job, 128),
		boardClosed:  make(chan uint, 1),
		boardCreated: make(chan *Firmata),
		done:         make(chan struct{}),
	}
	return fs
}

func (fs *Firmatas) Start() {
	for _, board := range fs.boards {
		go func() {
			select {
			case fs.boardClosed <- board.idx:
			case <-fs.done:
			}
		}()
	}
	go fs.loop()
}

func (fs *Firmatas) notifyBoardClosed(board *Firmata) {
	select {
	case <-board.f.CloseNotify():
		fs.boardClosed <- board.idx
	case <-fs.done:
	}
}

func (fs *Firmatas) loop() {
	for {
		select {
		case job := <-fs.jobs:
			board := fs.boards[job.board]
			job.fn(board.f, board.connected)
		case idx := <-fs.boardClosed:
			board := fs.boards[idx]
			if board.f != nil {
				board.f = nil
				board.connected = make(chan struct{})
			}
			go func(board Firmata) {
				defer func() {
					if board.f == nil {
						time.Sleep(time.Second)
						select {
						case fs.boardClosed <- board.idx:
						case <-fs.done:
						}
					}
				}()

				var d net.Dialer
				ctx, _ := context.WithTimeout(context.Background(), DefaultFirmataTimeout)
				c, err := d.DialContext(ctx, "tcp", board.info.Addr)
				if err != nil {
					return
				}

				f := firmata.NewFirmata(c)
				err = f.Handshake(ctx)
				if err != nil {
					return
				}

				board.f = f
				select {
				case fs.boardCreated <- &board:
				case <-fs.done:
					f.Close()
				}
			}(*board)
		case board := <-fs.boardCreated:
			fs.boards[board.idx] = board
			close(board.connected)
			board.connected = nil
			go fs.notifyBoardClosed(board)
		case <-fs.done:
			return
		}
	}
}

func (fs *Firmatas) AddProxyPipe(board uint, c io.ReadWriteCloser, closeNotify <-chan bool, beforeAdd func(board *firmata.Firmata)) error {
	done := make(chan struct{}, 1)
	defer close(done)
	var boardConnected chan struct{}

	max := 3
	for i := 0; i < max; i++ {
		err := fs.dispatch(board, func(board *firmata.Firmata, connected chan struct{}) {
			defer func() { done <- struct{}{} }()
			if connected != nil {
				boardConnected = connected
				return
			}
			beforeAdd(board)
			board.AddProxyPipe_l(c)
		})
		if err != nil {
			return err
		}

		<-done
		if boardConnected == nil {
			return nil
		}

		if i == max-1 {
			return fmt.Errorf("Proxy to board failed after %d tries", max)
		}

		if closeNotify != nil {
			select {
			case <-boardConnected:
			case <-closeNotify:
				return ErrConnclosed
			case <-fs.done:
				return ErrClosed
			}
		} else {
			select {
			case <-boardConnected:
			case <-fs.done:
				return ErrClosed
			}
		}
	}
	return fmt.Errorf("Bug: Proxy to board failed")
}

func (fs *Firmatas) Dispatch(board uint, fn func(board *firmata.Firmata)) error {
	done := make(chan struct{}, 1)
	defer close(done)
	var boardConnected chan struct{}

	max := 3
	for i := 0; i < max; i++ {
		err := fs.dispatch(board, func(board *firmata.Firmata, connected chan struct{}) {
			defer func() { done <- struct{}{} }()
			if connected != nil {
				boardConnected = connected
				return
			}
			fn(board)
		})
		if err != nil {
			return err
		}

		<-done
		if boardConnected == nil {
			return nil
		}

		if i == max-1 {
			return fmt.Errorf("Proxy to board failed after %d tries", max)
		}

		select {
		case <-boardConnected:
		case <-fs.done:
			return ErrClosed
		}
	}
	return fmt.Errorf("Bug: Proxy to board failed")
}

func (fs *Firmatas) dispatch(board uint, fn onBoardFn) error {
	if board >= uint(len(fs.boards)) {
		return fmt.Errorf("Board out of index: %d of %d", board, len(fs.boards))
	}
	select {
	case fs.jobs <- job{board, fn}:
		return nil
	case <-fs.done:
		return ErrClosed
	}
}
