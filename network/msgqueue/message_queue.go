package msgqueue

import (
	"github.com/bloxapp/ssv/network"
	"github.com/bloxapp/ssv/utils/logex"
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
	msgMutex    sync.Mutex
	indexFuncs  []IndexFunc
	queue       map[string][]messageContainer // = map[index][messageContainer.id]messageContainer
	allMessages map[string]messageContainer
}

// New is the constructor of MessageQueue
func New() *MessageQueue {
	return &MessageQueue{
		msgMutex:    sync.Mutex{},
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
		if q.queue[idx] == nil {
			q.queue[idx] = make([]messageContainer, 0)
		}
		q.queue[idx] = append(q.queue[idx], msgContainer)
	}
	q.allMessages[msgContainer.id] = msgContainer
}

// MessagesForIndex returns all messages for an index
func (q *MessageQueue) MessagesForIndex(index string) map[string]*network.Message {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()

	ret := make(map[string]*network.Message)
	for _, cont := range q.queue[index] {
		ret[cont.id] = cont.msg
	}

	return ret
}

// PopMessage will return a message by its index if found, will also delete all other index occurrences of that message
func (q *MessageQueue) PopMessage(index string) *network.Message {
	start := time.Now()
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()

	if len(q.queue[index]) > 0 {
		logex.GetLogger().Debug("pop message start after mutex", zap.Int64("duration", time.Since(start).Milliseconds()))
		start = time.Now()
		c := q.queue[index][0]
		// delete the msg from all the indexes
		q.deleteMessageFromAllIndexes(c.indexes, c.id)
		logex.GetLogger().Debug("pop message done", zap.Int64("duration", time.Since(start).Milliseconds()))
		return c.msg
	}
	return nil
}

// MsgCount will return a count of messages by their index
func (q *MessageQueue) MsgCount(index string) int {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()
	return len(q.queue[index])
}

// Len will return a count of messages by their index
func (q *MessageQueue) Len() int {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()
	return len(q.queue)
}

// DeleteMessagesWithIds deletes all msgs by the given id
func (q *MessageQueue) DeleteMessagesWithIds(ids []string) {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()
	for _, id := range ids {
		if msg, found := q.allMessages[id]; found {
			q.deleteMessageFromAllIndexes(msg.indexes, id)
		}
	}
}

func (q *MessageQueue) deleteMessageFromAllIndexes(indexes []string, id string) {
	for _, indx := range indexes {
		newIndexQ := make([]messageContainer, 0)
		for _, msg := range q.queue[indx] {
			if msg.id != id {
				newIndexQ = append(newIndexQ, msg)
			}
		}
		if len(newIndexQ) == 0 {
			logex.GetLogger().Debug("newIndexQ is empty!", zap.Strings("indexes", indexes), zap.String("id", id))
		}
		q.queue[indx] = newIndexQ
	}
	delete(q.allMessages, id)
}

// PurgeIndexedMessages will delete all indexed messages for the given index
func (q *MessageQueue) PurgeIndexedMessages(index string) {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()

	q.queue[index] = make([]messageContainer, 0)
}

type QueueData struct {
	Q       map[string][]*messageContainer
	Msgs map[string]*messageContainer
}

func (q *MessageQueue) Dump() QueueData {
	q.msgMutex.Lock()
	defer q.msgMutex.Unlock()

	return QueueData{
		q.queue,
		q.allMessages,
	}
}