package changeround

import (
	ibft2 "github.com/bloxapp/ssv/ibft/instance"
	"github.com/bloxapp/ssv/ibft/instance/spectesting"
	"github.com/bloxapp/ssv/ibft/proto"
	"github.com/bloxapp/ssv/network"
	"github.com/bloxapp/ssv/network/msgqueue"
	"github.com/stretchr/testify/require"
	"testing"
)

// PartialQuorum tests partial round change behaviour
type PartialQuorum struct {
	instances  []*ibft2.Instance
	inputValue []byte
	lambda     []byte
}

// Name returns test name
func (test *PartialQuorum) Name() string {
	return "receive f+1 change round messages -> bump round -> set timer -> broadcast round change"
}

// Prepare prepares the test
func (test *PartialQuorum) Prepare(t *testing.T) {
	test.lambda = []byte{1, 2, 3, 4}
	test.inputValue = spectesting.TestInputValue()

	test.instances = make([]*ibft2.Instance, 0)
	for i, msgs := range test.MessagesSequence(t) {
		instance := spectesting.TestIBFTInstance(t, test.lambda)
		test.instances = append(test.instances, instance)
		instance.State().Round.Set(uint64(i))

		// load messages to queue
		for _, msg := range msgs {
			instance.MsgQueue.AddMessage(&network.Message{
				SignedMessage: msg,
				Type:          network.NetworkMsg_IBFTType,
			})
			spectesting.RequireReturnedTrueNoError(t, instance.ProcessMessage)
		}
	}
}

// MessagesSequence includes all test messages
func (test *PartialQuorum) MessagesSequence(t *testing.T) [][]*proto.SignedMessage {
	return [][]*proto.SignedMessage{
		{ // f+1 points to 2
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 2, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 3, 2),
		},
		{ // f+1 points to 3
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 0, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 0, 2),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 3, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 3, 2),
		},
		{ // f+1 points to 4
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 0, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 0, 2),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 4, 2),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 5, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 6, 2),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 7, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 8, 2),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 9, 1),
		},
		{ // f+1 points not pointing anywhere
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 0, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 1, 2),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 2, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 3, 2),
		},
		{ // f points to 2, no partial quorum
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 10, 1),
		},
		{ // f+1 points to 7
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 0, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 0, 2),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 4, 2),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 5, 1),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[2], test.lambda, 4, 3),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[3], test.lambda, 7, 4),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[1], test.lambda, 1, 2),
			spectesting.ChangeRoundMsg(t, spectesting.TestSKs()[0], test.lambda, 9, 1),
		},
	}
}

// Run runs the test
func (test *PartialQuorum) Run(t *testing.T) {
	require.Len(t, test.instances, 6)

	require.NoError(t, test.instances[0].ChangeRoundPartialQuorumMsgPipeline().Run(nil))
	require.EqualValues(t, 2, test.instances[0].State().Round.Get())
	test.instances[0].MsgQueue.PurgeIndexedMessages(msgqueue.IBFTMessageIndexKey(
		test.instances[0].State().Lambda.Get(),
		test.instances[0].State().SeqNumber.Get()))

	require.NoError(t, test.instances[0].ChangeRoundPartialQuorumMsgPipeline().Run(nil))
	require.EqualValues(t, 3, test.instances[1].State().Round.Get())
	test.instances[1].MsgQueue.PurgeIndexedMessages(msgqueue.IBFTMessageIndexKey(
		test.instances[1].State().Lambda.Get(),
		test.instances[1].State().SeqNumber.Get()))

	require.NoError(t, test.instances[2].ChangeRoundPartialQuorumMsgPipeline().Run(nil))
	require.EqualValues(t, 8, test.instances[2].State().Round.Get())
	test.instances[2].MsgQueue.PurgeIndexedMessages(msgqueue.IBFTMessageIndexKey(
		test.instances[2].State().Lambda.Get(),
		test.instances[2].State().SeqNumber.Get()))

	require.NoError(t, test.instances[3].ChangeRoundPartialQuorumMsgPipeline().Run(nil))
	require.EqualValues(t, 3, test.instances[3].State().Round.Get())

	require.NoError(t, test.instances[4].ChangeRoundPartialQuorumMsgPipeline().Run(nil))
	require.EqualValues(t, 4, test.instances[4].State().Round.Get())

	require.NoError(t, test.instances[5].ChangeRoundPartialQuorumMsgPipeline().Run(nil))
	require.EqualValues(t, 7, test.instances[5].State().Round.Get())
}
