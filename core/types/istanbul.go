package types

import (
	"errors"
	"io"

	"github.com/linkeye/linkeye/common"
	"github.com/linkeye/linkeye/rlp"
)

var (
	// BFTDigest represents a hash of "BFT practical byzantine fault tolerance"
	// to identify whether the block is from BFT consensus engine
	BFTDigest = common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	BFTExtraVanity = 32 // Fixed number of extra-data bytes reserved for validator vanity
	BFTExtraSeal   = 65 // Fixed number of extra-data bytes reserved for validator seal

	// ErrInvalidBFTHeaderExtra is returned if the length of extra-data is less than 32 bytes
	ErrInvalidBFTHeaderExtra = errors.New("invalid bft header extra-data")
)

type BFTExtra struct {
	Validators    []common.Address
	Seal          []byte
	CommittedSeal [][]byte
}

// EncodeRLP serializes ist into the Ethereum RLP format.
func (ist *BFTExtra) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		ist.Validators,
		ist.Seal,
		ist.CommittedSeal,
	})
}

// DecodeRLP implements rlp.Decoder, and load the bft fields from a RLP stream.
func (ist *BFTExtra) DecodeRLP(s *rlp.Stream) error {
	var bftExtra struct {
		Validators    []common.Address
		Seal          []byte
		CommittedSeal [][]byte
	}
	if err := s.Decode(&bftExtra); err != nil {
		return err
	}
	ist.Validators, ist.Seal, ist.CommittedSeal = bftExtra.Validators, bftExtra.Seal, bftExtra.CommittedSeal
	return nil
}

// ExtractBFTExtra extracts all values of the BFTExtra from the header. It returns an
// error if the length of the given extra-data is less than 32 bytes or the extra-data can not
// be decoded.
func ExtractBFTExtra(h *Header) (*BFTExtra, error) {
	if len(h.Extra) < BFTExtraVanity {
		return nil, ErrInvalidBFTHeaderExtra
	}

	var bftExtra *BFTExtra
	err := rlp.DecodeBytes(h.Extra[BFTExtraVanity:], &bftExtra)
	if err != nil {
		return nil, err
	}
	return bftExtra, nil
}

// BFTFilteredHeader returns a filtered header which some information (like seal, committed seals)
// are clean to fulfill the BFT hash rules. It returns nil if the extra-data cannot be
// decoded/encoded by rlp.
func BFTFilteredHeader(h *Header, keepSeal bool) *Header {
	newHeader := CopyHeader(h)
	bftExtra, err := ExtractBFTExtra(newHeader)
	if err != nil {
		return nil
	}

	if !keepSeal {
		bftExtra.Seal = []byte{}
	}
	bftExtra.CommittedSeal = [][]byte{}

	payload, err := rlp.EncodeToBytes(&bftExtra)
	if err != nil {
		return nil
	}

	newHeader.Extra = append(newHeader.Extra[:BFTExtraVanity], payload...)

	return newHeader
}
