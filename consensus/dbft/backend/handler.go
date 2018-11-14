package backend

import (
	"errors"

	lru "github.com/hashicorp/golang-lru"
	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/consensus"
	bft "github.com/linkeye/linkeye/consensus/dbft"
	"github.com/linkeye/linkeye/p2p"
)

const (
	bftMsg = 0x15
)

var (
	// errDecodeFailed is returned when decode message fails
	errDecodeFailed = errors.New("fail to decode bft message")
)

// Protocol implements consensus.Engine.Protocol
func (sb *backend) Protocol() consensus.Protocol {
	return consensus.Protocol{
		Name:     "dbft",
		Versions: []uint{64},
		Lengths:  []uint64{18},
	}
}

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *backend) HandleMsg(addr common.Address, msg p2p.Msg) (bool, error) {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()

	if msg.Code == bftMsg {
		if !sb.coreStarted {
			return true, bft.ErrStoppedEngine
		}

		var data []byte
		if err := msg.Decode(&data); err != nil {
			return true, errDecodeFailed
		}

		hash := bft.RLPHash(data)

		// Mark peer's message
		ms, ok := sb.recentMessages.Get(addr)
		var m *lru.ARCCache
		if ok {
			m, _ = ms.(*lru.ARCCache)
		} else {
			m, _ = lru.NewARC(inmemoryMessages)
			sb.recentMessages.Add(addr, m)
		}
		m.Add(hash, true)

		// Mark self known message
		if _, ok := sb.knownMessages.Get(hash); ok {
			return true, nil
		}
		sb.knownMessages.Add(hash, true)

		go sb.bftEventMux.Post(bft.MessageEvent{
			Payload: data,
		})

		return true, nil
	}
	return false, nil
}

// SetBroadcaster implements consensus.Handler.SetBroadcaster
func (sb *backend) SetBroadcaster(broadcaster consensus.Broadcaster) {
	sb.broadcaster = broadcaster
}

func (sb *backend) NewChainHead() error {
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if !sb.coreStarted {
		return bft.ErrStoppedEngine
	}
	go sb.bftEventMux.Post(bft.FinalCommittedEvent{})
	return nil
}
