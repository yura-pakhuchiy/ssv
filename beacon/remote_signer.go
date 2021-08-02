package beacon

import (
	spec "github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/bloxapp/ssv/ibft/proto"
)

type RemoteSigner interface {
	DutySigner
	IBFTSigner
}

type IBFTSigner interface {
	// SignIBFTMessage returns a signed iBFT message or error
	SignIBFTMessage(message proto.Message, pk []byte) (proto.SignedMessage, error)
	// AddNewValidatorShare adds a new encrypted share for a validator, returns error if invalid/ error
	AddNewValidatorShare(encryptedShare []byte, sharePK []byte, pk []byte) error
	// RemoveValidatorShare will remove a share or return error
	RemoveValidatorShare(sharePK []byte, pk []byte) error
}

type DutySigner interface {
	// SignAttestation returns a signed eth2 spec attestation or error
	SignAttestation(data *spec.AttestationData, domain []byte, pk []byte) (spec.Attestation, error)
	// SignProposal returns a signed eth2 spec beacon block or error
	SignProposal(data *spec.BeaconBlock, domain []byte, pk []byte) (spec.SignedBeaconBlock, error)
	// SignAggregateAndProof returns a signed eth2 spec aggregate and proof or error
	SignAggregateAndProof(data *spec.AggregateAndProof, domain []byte, pk []byte) (spec.SignedAggregateAndProof, error)
	// SignSlot returns a signed uint64 slot or error
	SignSlot(slot uint64, domain []byte, pk []byte) ([]byte, error)
	// SignEpoch returns a signed uint64 epoch or error
	SignEpoch(epoch uint64, domain []byte, pk []byte) ([]byte, error)
}
