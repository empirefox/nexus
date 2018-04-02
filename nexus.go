//go:generate protoc --go_out=. ./nexus-models.proto
package nexus

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/empirefox/firmata"
	proto "github.com/golang/protobuf/proto"
	"go.uber.org/zap"
)

const (
	NexusSubscribeHostSuffix = ".nc:80"
)

type Nexus struct {
	log      *zap.Logger
	config   Config
	firmatas *Firmatas
	groups   []byte
}

func NewNexus(log *zap.Logger, config Config) (*Nexus, error) {
	gs, err := config.ToProtobuf()
	if err != nil {
		return nil, err
	}

	groups, err := proto.Marshal(&GetGroupsResponse{
		Groups: gs,
	})
	if err != nil {
		return nil, err
	}

	n := &Nexus{
		log:      log,
		config:   config,
		firmatas: NewFirmatas(config.BoardInfos),
		groups:   groups,
	}

	return n, nil
}

func (n *Nexus) Start() {
	n.firmatas.Start()
}

func (n *Nexus) GetGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
		w.Write(n.groups)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (n *Nexus) ConnectSubscribe(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.Host, NexusSubscribeHostSuffix) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	id, err := strconv.ParseUint(strings.TrimSuffix(r.Host, NexusSubscribeHostSuffix), 10, 32)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var rwc ReadWriteCloser
	var closeNotify <-chan bool
	hijacked := false
	if r.Body == http.NoBody {
		w.WriteHeader(http.StatusOK)
		hijack, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		c, _, err := hijack.Hijack()
		if err != nil {
			return
		}

		rwc = ReadWriteCloser{
			ReadCloser: c,
			w:          c,
			flush:      getFlush(c),
		}
		closeNotify = getCloseNotify(c)
		hijacked = true
	} else {
		rwc = ReadWriteCloser{
			ReadCloser: r.Body,
			w:          w,
			flush:      getFlush(w),
		}
		closeNotify = getCloseNotify(w)
	}

	var boardCloseNotifier <-chan struct{}
	err = n.firmatas.AddProxyPipe(uint(id), rwc, closeNotify, func(board *firmata.Firmata) {
		if !hijacked {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
		}
		boardCloseNotifier = board.CloseNotify()
	})
	if err != nil {
		if !hijacked {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		return
	}

	select {
	case <-w.(http.CloseNotifier).CloseNotify():
	case <-boardCloseNotifier:
	}
}

func getFlush(i interface{}) func() {
	flusher, ok := i.(http.Flusher)
	if ok {
		return flusher.Flush
	}
	return func() {}
}

func getCloseNotify(i interface{}) <-chan bool {
	cn, ok := i.(http.CloseNotifier)
	if ok {
		return cn.CloseNotify()
	}
	return nil
}
