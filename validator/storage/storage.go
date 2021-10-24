package storage

import (
	"encoding/hex"
	"github.com/bloxapp/ssv/beacon"
	"github.com/bloxapp/ssv/storage/basedb"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"sync"
)

// ICollection interface for validator storage
type ICollection interface {
	LoadMultipleFromConfig(items []ShareOptions)
	LoadFromConfig(options ShareOptions) (string, error)
	SaveValidatorShare(share *Share) error
	GetValidatorShare(key []byte) (*Share, bool, error)
	GetAllValidatorsShare() ([]*Share, error)
	CleanAllShares() error
}

func collectionPrefix() []byte {
	return []byte("share-")
}

// CollectionOptions struct
type CollectionOptions struct {
	DB     basedb.IDb
	Logger *zap.Logger
}

// Collection struct
type Collection struct {
	db     basedb.IDb
	logger *zap.Logger
	lock   sync.RWMutex
}

// NewCollection creates new share storage
func NewCollection(options CollectionOptions) ICollection {
	collection := Collection{
		db:     options.DB,
		logger: options.Logger,
		lock:   sync.RWMutex{},
	}
	return &collection
}

// LoadMultipleFromConfig fetch multiple validators share from config and save it to db
func (s *Collection) LoadMultipleFromConfig(items []ShareOptions) {
	var addedValidators []string
	if len(items) > 0 {
		s.logger.Info("loading validators share from config", zap.Int("count", len(items)))
		for _, opts := range items {
			pubkey, err := s.LoadFromConfig(opts)
			if err != nil {
				s.logger.Error("failed to load validator share data from config", zap.Error(err))
				continue
			}
			addedValidators = append(addedValidators, pubkey)
		}
		s.logger.Info("successfully loaded validators from config", zap.Strings("pubkeys", addedValidators))
	}
}

// LoadFromConfig fetch validator share from config and save it to db
func (s *Collection) LoadFromConfig(options ShareOptions) (string, error) {
	if len(options.PublicKey) == 0 || len(options.ShareKey) == 0 || len(options.Committee) == 0 {
		return "", errors.New("one or more fields are missing (PublicKey, ShareKey, Committee)")
	}
	share, err := options.ToShare()
	if err != nil {
		return "", errors.WithMessage(err, "failed to create share object")
	} else if share != nil {
		pubKey := share.ShareKey.SerializeToHexStr()
		err := s.SaveValidatorShare(share)
		return pubKey, err
	}
	return "", errors.New("returned nil share")
}

// SaveValidatorShare save validator share to db
func (s *Collection) SaveValidatorShare(share *Share) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.saveUnsafe(share)
}

// SaveValidatorShare save validator share to db
func (s *Collection) saveUnsafe(share *Share) error {
	value, err := share.Serialize()
	if err != nil {
		s.logger.Error("failed serialized validator", zap.Error(err))
		return err
	}
	return s.db.Set(collectionPrefix(), share.PublicKey.Serialize(), value)
}

// GetValidatorShare by key
func (s *Collection) GetValidatorShare(key []byte) (*Share, bool, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return s.getUnsafe(key)
}

// GetValidatorShare by key
func (s *Collection) getUnsafe(key []byte) (*Share, bool, error) {
	obj, found, err := s.db.Get(collectionPrefix(), key)
	if !found {
		return nil, false, nil
	}
	if err != nil {
		return nil, found, err
	}
	share, err := (&Share{}).Deserialize(obj)
	return share, found, err
}

// CleanAllShares cleans all existing shares from DB
func (s *Collection) CleanAllShares() error {
	return s.db.RemoveAllByCollection(collectionPrefix())
}

// GetAllValidatorsShare returns all shares
func (s *Collection) GetAllValidatorsShare() ([]*Share, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	objs, err := s.db.GetAllByCollection(collectionPrefix())
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get val share")
	}
	var res []*Share
	for _, obj := range objs {
		val, err := (&Share{}).Deserialize(obj)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to deserialize validator")
		}
		res = append(res, val)
	}

	return res, nil
}

// UpdateValidatorMetadata updates the metadata of the given validator
func (s *Collection) UpdateValidatorMetadata(pk string, metadata *beacon.ValidatorMetadata) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	key, err := hex.DecodeString(pk)
	if err != nil {
		return err
	}
	share, found, err := s.getUnsafe(key)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	share.Metadata = metadata
	return s.saveUnsafe(share)
}
