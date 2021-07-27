package msgqueue

import (
	"github.com/bloxapp/ssv/network"
	"github.com/bloxapp/ssv/utils/logex"
	"github.com/patrickmn/go-cache"
	"github.com/pborman/uuid"
	"go.uber.org/zap"
	"sync"
	"time"
)

// IndexFunc is the function that indexes messages to be later pulled by those indexes
type IndexFunc func(msg *network.Message) []string

type messageContainer struct {
	id      string
	msg     *network.Message
	indexes []string
}

// MessageQueue is a broker of messages for the IBFT instance to process.
// Messages can come in various times, even next round's messages can come "early" as other nodes can change round before this node.
// To solve this issue we have a message broker from which the instance pulls new messages, this also reduces concurrency issues as the instance is now single threaded.
// The message queue has internal logic to organize messages by their round.
type MessageQueue struct {
	msgMutex    sync.RWMutex
	indexFuncs  []IndexFunc
	queue       map[string][]messageContainer // = map[index][messageContainer.id]messageContainer
	q           *cache.Cache
	msgs        *cache.Cache
	allMessages map[string]messageContainer
}

// New is the constructor of MessageQueue
func New() *MessageQueue {
	return &MessageQueue{
		msgMutex:    sync.RWMutex{},
		q:           cache.New(time.Minute*10, time.Minute*11),
		msgs:        cache.New(time.Minute*10, time.Minute*11),
		queue:       make(map[string][]messageContainer),
		allMessages: make(map[string]messageContainer),
		indexFuncs: []IndexFunc{
			iBFTMessageIndex(),
			iBFTAllRoundChangeIndex(),
			sigMessageIndex(),
			decidedMessageIndex(),
			syncMessageIndex(),
		},
	}
}

// AddIndexFunc adds an index function that will be activated every new message the queue receives
func (q *MessageQueue) AddIndexFunc(f IndexFunc) {
	q.indexFuncs = append(q.indexFuncs, f)
}

// AddMessage adds a message the queue based on the message round.
// AddMessage is thread safe
func (q *MessageQueue) AddMessage(msg *network.Message) {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()

	// index msg
	indexes := make([]string, 0)
	for _, f := range q.indexFuncs {
		indexes = append(indexes, f(msg)...)
	}

	// add it to queue
	msgContainer := messageContainer{
		id:      uuid.New(),
		msg:     msg,
		indexes: indexes,
	}

	for _, idx := range indexes {
		var msgs []messageContainer
		if raw, exist := q.q.Get(idx); exist {
			if msgContainers, ok := raw.([]messageContainer); ok {
				msgs = msgContainers
			}
		}
		msgs = append(msgs, msgContainer)

		q.q.SetDefault(idx, msgs)
	}
	q.msgs.SetDefault(msgContainer.id, msgContainer)
}

// MessagesForIndex returns all messages for an index
func (q *MessageQueue) MessagesForIndex(index string) map[string]*network.Message {
	q.msgMutex.RLock()
	defer q.msgMutex.RUnlock()

	ret := make(map[string]*network.Message)

	if raw, exist := q.q.Get(index); exist {
		msgContainers, ok := raw.([]messageContainer)
		if ok {
			for _, cont := range msgContainers {
				ret[cont.id] = cont.msg
			}
		}
	}

	return ret
}

// PopMessage will return a message by its index if found, will also delete all other index occurrences of that message
func (q *MessageQueue) PopMessage(index string) *network.Message {
	start := time.Now()
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()

	if raw, exist := q.q.Get(index); exist {
		msgContainers, ok := raw.([]messageContainer)
		if ok && len(msgContainers) > 0 {
			c := msgContainers[0]
			// delete the msg from all the indexes
			q.deleteMessageFromAllIndexes(c.indexes, c.id)
			logex.GetLogger().Debug("pop message done", zap.Int64("duration", time.Since(start).Milliseconds()))
			return c.msg
		}
	}
	return nil
}

// MsgCount will return a count of messages by their index
func (q *MessageQueue) MsgCount(index string) int {
	q.msgMutex.RLock()
	defer q.msgMutex.RUnlock()

	if raw, exist := q.q.Get(index); exist {
		if msgContainers, ok := raw.([]messageContainer); ok {
			return len(msgContainers)
		}
	}
	return 0
}

// Len will return a count of messages by their index
func (q *MessageQueue) Len() int {
	q.msgMutex.RLock()
	defer q.msgMutex.RUnlock()

	return q.q.ItemCount()
}

// DeleteMessagesWithIds deletes all msgs by the given id
func (q *MessageQueue) DeleteMessagesWithIds(ids []string) {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()
	for _, id := range ids {
		if raw, found := q.msgs.Get(id); found {
			if msg, ok := raw.(messageContainer); ok {
				q.deleteMessageFromAllIndexes(msg.indexes, id)
			}
		}
	}
}

func (q *MessageQueue) deleteMessageFromAllIndexes(indexes []string, id string) {
	for _, indx := range indexes {
		newIndexQ := make([]messageContainer, 0)
		if raw, exist := q.q.Get(indx); exist {
			if msgContainers, ok := raw.([]messageContainer); ok {
				for _, msg := range msgContainers {
					if len(msg.id) == 0 {
						logex.GetLogger().Debug("MSG IS NIL!!!", zap.Any("msg", msg))
					}
					if msg.id != id {
						newIndexQ = append(newIndexQ, msg)
					}
				}
			}
			q.q.SetDefault(indx, newIndexQ)
		}
	}
	q.msgs.Delete(id)
}

// PurgeIndexedMessages will delete all indexed messages for the given index
func (q *MessageQueue) PurgeIndexedMessages(index string) {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()

	//q.queue[index] = make([]messageContainer, 0)
	q.q.SetDefault(index, make([]messageContainer, 0))
}

// QueueData struct to represent data in metric
type QueueData struct {
	QCache    *cache.Cache
	MsgsCache *cache.Cache
	Q    map[string][]messageContainer
	Msgs map[string]messageContainer
}

// Dump returning data
func (q *MessageQueue) Dump() QueueData {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()

	return QueueData{
		q.q,
		q.msgs,
		q.queue,
		q.allMessages,
	}
}
