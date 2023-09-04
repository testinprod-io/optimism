package derive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
	"io"
	"math/big"
	"sort"
)

type spanBatchPrefix struct {
	relTimestamp  uint64
	l1OriginNum   uint64
	parentCheck   []byte
	l1OriginCheck []byte
}

type spanBatchSignature struct {
	v uint64
	r *uint256.Int
	s *uint256.Int
}

type spanBatchPayload struct {
	blockCount    uint64
	originBits    *big.Int
	blockTxCounts []uint64
	txs           *spanBatchTxs
	feeRecipients []common.Address
}

type spanBatchDerivedFields struct {
	isDerived       bool
	batchTimestamp  uint64
	blockTimestamps []uint64
	blockOriginNums []uint64
}

type SpanBatch struct {
	batchType int
	spanBatchPrefix
	spanBatchPayload
	spanBatchDerivedFields
}

func (b *SpanBatch) GetBatchType() int {
	return b.batchType
}

func (b *SpanBatch) GetTimestamp() uint64 {
	if !b.isDerived {
		panic("Span batch fields are not derived yet")
	}
	return b.batchTimestamp
}

func (b *SpanBatch) GetEpochNum() rollup.Epoch {
	if !b.isDerived {
		panic("Span batch fields are not derived yet")
	}
	return rollup.Epoch(b.blockOriginNums[0])
}

func (b *SpanBatch) GetLogContext(log log.Logger) log.Logger {
	return log.New(
		"batch_timestamp", b.batchTimestamp,
		"parent_check", b.parentCheck,
		"origin_check", b.l1OriginCheck,
		"origin_number", b.l1OriginNum,
		"epoch_number", b.GetEpochNum(),
		"blocks", b.blockCount,
	)
}

func (b *SpanBatch) CheckOriginHash(hash common.Hash) bool {
	return bytes.Equal(b.l1OriginCheck, hash.Bytes()[:20])
}

func (b *SpanBatch) CheckParentHash(hash common.Hash) bool {
	return bytes.Equal(b.parentCheck, hash.Bytes()[:20])
}

func (b *spanBatchPayload) decodeOriginBits(originBitBuffer []byte, blockCount uint64) {
	originBits := new(big.Int)
	for i := 0; i < int(blockCount); i += 8 {
		end := i + 8
		if end < int(blockCount) {
			end = int(blockCount)
		}
		bits := originBitBuffer[i/8]
		for j := i; j < end; j++ {
			bit := uint((bits >> (j - i)) & 1)
			originBits.SetBit(originBits, j, bit)
		}
	}
	b.originBits = originBits
}

func (b *SpanBatch) decodeFeeRecipients(r *bytes.Reader) error {
	if b.batchType < SpanBatchV2Type {
		return nil
	}
	var idxs []uint64
	cardinalFeeRecipientsCount := uint64(0)
	for i := 0; i < int(b.blockCount); i++ {
		idx, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read fee recipent index: %w", err)
		}
		idxs = append(idxs, idx)
		if cardinalFeeRecipientsCount < idx+1 {
			cardinalFeeRecipientsCount = idx + 1
		}
	}
	var cardinalFeeRecipients []common.Address
	for i := 0; i < int(cardinalFeeRecipientsCount); i++ {
		feeRecipient := make([]byte, common.AddressLength)
		_, err := io.ReadFull(r, feeRecipient)
		if err != nil {
			return fmt.Errorf("failed to read fee recipent address: %w", err)
		}
		cardinalFeeRecipients = append(cardinalFeeRecipients, common.BytesToAddress(feeRecipient))
	}
	b.feeRecipients = make([]common.Address, 0)
	for _, idx := range idxs {
		feeRecipient := cardinalFeeRecipients[idx]
		b.feeRecipients = append(b.feeRecipients, feeRecipient)
	}
	return nil
}

// decodePrefix parses data into b.spanBatchPrefix
func (b *SpanBatch) decodePrefix(r *bytes.Reader) error {
	relTimestamp, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("failed to read rel timestamp: %w", err)
	}
	b.relTimestamp = relTimestamp
	L1OriginNum, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("failed to read l1 origin num: %w", err)
	}
	b.l1OriginNum = L1OriginNum
	b.parentCheck = make([]byte, 20)
	_, err = io.ReadFull(r, b.parentCheck)
	if err != nil {
		return fmt.Errorf("failed to read parent check: %w", err)
	}
	b.l1OriginCheck = make([]byte, 20)
	_, err = io.ReadFull(r, b.l1OriginCheck)
	if err != nil {
		return fmt.Errorf("failed to read l1 origin check: %w", err)
	}
	return nil
}

// decodePayload parses data into b.spanBatchPayload
func (b *SpanBatch) decodePayload(r *bytes.Reader) error {
	blockCount, err := binary.ReadUvarint(r)
	// TODO: check block count is not too large
	if err != nil {
		return fmt.Errorf("failed to read block count: %w", err)
	}
	originBitBufferLen := blockCount / 8
	if blockCount%8 != 0 {
		originBitBufferLen++
	}
	originBitBuffer := make([]byte, originBitBufferLen)
	_, err = io.ReadFull(r, originBitBuffer)
	if err != nil {
		return fmt.Errorf("failed to read origin bits: %w", err)
	}
	b.decodeOriginBits(originBitBuffer, blockCount)
	blockTxCounts := make([]uint64, blockCount)
	totalBlockTxCount := uint64(0)
	for i := 0; i < int(blockCount); i++ {
		blockTxCount, err := binary.ReadUvarint(r)
		// TODO: check blockTxCount is not too large
		if err != nil {
			return fmt.Errorf("failed to read block tx count: %w", err)
		}
		blockTxCounts[i] = blockTxCount
		totalBlockTxCount += blockTxCount
	}
	b.blockCount = blockCount
	b.blockTxCounts = blockTxCounts
	b.txs = &spanBatchTxs{}
	b.txs.totalBlockTxCount = totalBlockTxCount
	if err = b.txs.decode(r); err != nil {
		return err
	}
	if err = b.decodeFeeRecipients(r); err != nil {
		return err
	}
	return nil
}

// decodeBytes parses data into b from data
func (b *SpanBatch) decodeBytes(data []byte) error {
	r := bytes.NewReader(data)
	if err := b.decodePrefix(r); err != nil {
		return err
	}
	if err := b.decodePayload(r); err != nil {
		return err
	}
	return nil
}

func (b *SpanBatch) encodePrefix(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], b.relTimestamp)
	if _, err := w.Write(buf[:n]); err != nil {
		return fmt.Errorf("cannot write rel timestamp: %w", err)
	}
	n = binary.PutUvarint(buf[:], b.l1OriginNum)
	if _, err := w.Write(buf[:n]); err != nil {
		return fmt.Errorf("cannot write l1 origin number: %w", err)
	}
	if _, err := w.Write(b.parentCheck); err != nil {
		return fmt.Errorf("cannot write parent check: %w", err)
	}
	if _, err := w.Write(b.l1OriginCheck); err != nil {
		return fmt.Errorf("cannot write l1 origin check: %w", err)
	}
	return nil
}

func (b *spanBatchPayload) encodeOriginBits() []byte {
	originBitBufferLen := b.blockCount / 8
	if b.blockCount%8 != 0 {
		originBitBufferLen++
	}
	originBitBuffer := make([]byte, originBitBufferLen)
	for i := 0; i < int(b.blockCount); i += 8 {
		end := i + 8
		if end < int(b.blockCount) {
			end = int(b.blockCount)
		}
		var bits uint = 0
		for j := i; j < end; j++ {
			bits |= b.originBits.Bit(j) << (j - i)
		}
		originBitBuffer[i/8] = byte(bits)
	}
	return originBitBuffer
}

// encodeFeeRecipients parses data into b.feeRecipients
func (b *SpanBatch) encodeFeeRecipients(w io.Writer) error {
	if b.batchType < SpanBatchV2Type {
		return nil
	}
	var buf [binary.MaxVarintLen64]byte
	acc := uint64(0)
	indexs := make(map[common.Address]uint64)
	var cardinalFeeRecipents []common.Address
	for _, feeRecipent := range b.feeRecipients {
		_, exists := indexs[feeRecipent]
		if exists {
			continue
		}
		cardinalFeeRecipents = append(cardinalFeeRecipents, feeRecipent)
		indexs[feeRecipent] = acc
		acc++
	}
	for _, feeReceipt := range b.feeRecipients {
		idx := indexs[feeReceipt]
		n := binary.PutUvarint(buf[:], idx)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write fee recipent index: %w", err)
		}
	}
	for _, cardinalFeeReceipt := range cardinalFeeRecipents {
		if _, err := w.Write(cardinalFeeReceipt[:]); err != nil {
			return fmt.Errorf("cannot write fee recipent address: %w", err)
		}
	}
	return nil
}

func (b *SpanBatch) encodePayload(w io.Writer) error {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], b.blockCount)
	if _, err := w.Write(buf[:n]); err != nil {
		return fmt.Errorf("cannot write block count: %w", err)
	}
	originBitBuffer := b.encodeOriginBits()
	if _, err := w.Write(originBitBuffer); err != nil {
		return fmt.Errorf("cannot write origin bits: %w", err)
	}
	for _, blockTxCount := range b.blockTxCounts {
		n = binary.PutUvarint(buf[:], blockTxCount)
		if _, err := w.Write(buf[:n]); err != nil {
			return fmt.Errorf("cannot write block tx count: %w", err)
		}
	}
	if err := b.txs.encode(w); err != nil {
		return err
	}
	if err := b.encodeFeeRecipients(w); err != nil {
		return err
	}
	return nil
}

// encode writes the byte encoding of b to w
func (b *SpanBatch) encode(w io.Writer) error {
	if err := b.encodePrefix(w); err != nil {
		return err
	}
	if err := b.encodePayload(w); err != nil {
		return err
	}
	return nil
}

// encodeBytes returns the byte encoding of b
func (b *SpanBatch) encodeBytes() ([]byte, error) {
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()
	if err := b.encode(buf); err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

// NewSpanBatch merges SingularBatch List and initialize single SpanBatch
func NewSpanBatch(singularBatches []*SingularBatch, originChangedBit uint, genesisTimestamp uint64, chainId *big.Int) (*SpanBatch, error) {
	if len(singularBatches) == 0 {
		return nil, errors.New("cannot merge empty singularBatch list")
	}
	b := SpanBatch{batchType: SpanBatchType}
	// Sort by timestamp of L2 block
	sort.Slice(singularBatches, func(i, j int) bool {
		return singularBatches[i].Timestamp < singularBatches[j].Timestamp
	})
	// spanBatchPrefix
	span_start := singularBatches[0]
	span_end := singularBatches[len(singularBatches)-1]
	b.relTimestamp = span_start.Timestamp - genesisTimestamp
	b.l1OriginNum = uint64(span_end.EpochNum)
	b.parentCheck = make([]byte, 20)
	copy(b.parentCheck, span_start.ParentHash[:20])
	b.l1OriginCheck = make([]byte, 20)
	copy(b.l1OriginCheck, span_end.EpochHash[:20])
	// spanBatchPayload
	b.blockCount = uint64(len(singularBatches))
	b.originBits = new(big.Int)
	b.originBits.SetBit(b.originBits, 0, originChangedBit)
	for i := 1; i < len(singularBatches); i++ {
		bit := uint(0)
		if singularBatches[i-1].EpochNum < singularBatches[i].EpochNum {
			bit = 1
		}
		b.originBits.SetBit(b.originBits, i, bit)
	}
	var blockTxCounts []uint64
	var txs [][]byte
	var blockTimstamps []uint64
	var blockOriginNums []uint64
	for _, singularBatch := range singularBatches {
		blockTxCount := uint64(len(singularBatch.Transactions))
		blockTxCounts = append(blockTxCounts, blockTxCount)
		blockTimstamps = append(blockTimstamps, singularBatch.Timestamp)
		blockOriginNums = append(blockOriginNums, uint64(singularBatch.EpochNum))
		for _, rawTx := range singularBatch.Transactions {
			txs = append(txs, rawTx)
		}
	}
	b.blockTxCounts = blockTxCounts
	stxs, err := newSpanBatchTxs(txs, chainId)
	if err != nil {
		return nil, err
	}
	b.txs = stxs
	b.batchTimestamp = blockTimstamps[0]
	b.blockTimestamps = blockTimstamps
	b.blockOriginNums = blockOriginNums
	b.isDerived = true
	return &b, nil
}

// splitSpanBatch splits single SpanBatch and initialize SingularBatch lists
// Cannot fill every SingularBatch parent hash
func (b *SpanBatch) splitSpanBatch(l1Origins []eth.L1BlockRef) ([]*SingularBatch, error) {
	if !b.isDerived {
		panic("Span batch fields are not derived yet")
	}
	singularBatches := make([]*SingularBatch, b.blockCount)
	originIdx := -1
	for i := 0; i < len(l1Origins); i++ {
		if l1Origins[i].Number == b.blockOriginNums[0] {
			originIdx = i
			break
		}
	}
	if originIdx == -1 {
		return nil, fmt.Errorf("cannot find L1 origin")
	}
	txs, err := b.txs.fullTxs()
	if err != nil {
		return nil, err
	}
	txIdx := 0
	for i := 0; i < int(b.blockCount); i++ {
		singularBatch := SingularBatch{}
		singularBatch.Timestamp = b.blockTimestamps[i]
		singularBatch.EpochNum = rollup.Epoch(b.blockOriginNums[i])
		if b.originBits.Bit(i) == 1 && i > 0 {
			originIdx += 1
		}
		singularBatch.EpochHash = l1Origins[originIdx].Hash

		for j := 0; j < int(b.blockTxCounts[i]); j++ {
			singularBatch.Transactions = append(singularBatch.Transactions, txs[txIdx])
			txIdx++
		}
		singularBatches[i] = &singularBatch
	}
	return singularBatches, nil
}

func (b *SpanBatch) deriveSpanBatchFields(blockTime, genesisTimestamp uint64, chainId *big.Int) {
	b.batchTimestamp = b.relTimestamp + genesisTimestamp
	b.blockTimestamps = make([]uint64, b.blockCount)
	b.blockOriginNums = make([]uint64, b.blockCount)

	var l1OriginBlockNumber = b.l1OriginNum
	for i := int(b.blockCount) - 1; i >= 0; i-- {
		b.blockTimestamps[i] = b.batchTimestamp + uint64(i)*blockTime
		b.blockOriginNums[i] = l1OriginBlockNumber
		if b.originBits.Bit(i) == 1 && i > 0 {
			l1OriginBlockNumber--
		}
	}

	b.txs.chainID = chainId
	b.txs.recoverV()
	b.isDerived = true
}

func (b *SpanBatch) AppendSingularBatch(singularBatch *SingularBatch) error {
	b.blockCount += 1
	originBit := uint(0)
	if b.l1OriginNum != uint64(singularBatch.EpochNum) {
		originBit = 1
	}
	b.originBits.SetBit(b.originBits, int(b.blockCount-1), originBit)
	b.l1OriginNum = uint64(singularBatch.EpochNum)
	b.l1OriginCheck = singularBatch.EpochHash.Bytes()[:20]
	b.blockTxCounts = append(b.blockTxCounts, uint64(len(singularBatch.Transactions)))
	for _, rawTx := range singularBatch.Transactions {
		if err := b.txs.appendTx(rawTx); err != nil {
			return err
		}
	}
	b.blockTimestamps = append(b.blockTimestamps, singularBatch.Timestamp)
	b.blockOriginNums = append(b.blockOriginNums, uint64(singularBatch.EpochNum))
	return nil
}

type SpanBatchBuilder struct {
	parentEpochHash  common.Hash
	genesisTimestamp uint64
	chainId          *big.Int
	spanBatch        *SpanBatch
}

func NewSpanBatchBuilder(parentEpochHash common.Hash, genesisTimestamp uint64, chainId *big.Int) *SpanBatchBuilder {
	return &SpanBatchBuilder{
		parentEpochHash:  parentEpochHash,
		genesisTimestamp: genesisTimestamp,
		chainId:          chainId,
		spanBatch:        &SpanBatch{},
	}
}

func (b *SpanBatchBuilder) AppendSingularBatch(singularBatch *SingularBatch) error {
	if b.spanBatch.blockCount == 0 {
		originChangedBit := 0
		if singularBatch.EpochHash != b.parentEpochHash {
			originChangedBit = 1
		}
		spanBatch, err := NewSpanBatch([]*SingularBatch{singularBatch}, uint(originChangedBit), b.genesisTimestamp, b.chainId)
		if err != nil {
			return err
		}
		b.spanBatch = spanBatch
		return nil
	}
	return b.spanBatch.AppendSingularBatch(singularBatch)
}

func (b *SpanBatchBuilder) GetSpanBatch() *SpanBatch {
	return b.spanBatch
}

func (b *SpanBatchBuilder) GetBlockCount() uint64 {
	return b.spanBatch.blockCount
}

func (b *SpanBatchBuilder) Reset() {
	b.spanBatch = &SpanBatch{}
}

// ReadTxData reads raw RLP tx data from reader
func ReadTxData(r *bytes.Reader) ([]byte, int, error) {
	var txData []byte
	offset, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to seek tx reader: %w", err)
	}
	b, err := r.ReadByte()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read tx initial byte: %w", err)
	}
	txType := byte(0)
	if int(b) <= 0x7F {
		// EIP-2718: non legacy tx so write tx type
		txType = byte(b)
		txData = append(txData, txType)
	} else {
		// legacy tx: seek back single byte to read prefix again
		_, err = r.Seek(offset, io.SeekStart)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to seek tx reader: %w", err)
		}
	}
	// TODO: set maximum inputLimit
	s := rlp.NewStream(r, 0)
	var txPayload []byte
	kind, _, err := s.Kind()
	switch {
	case err != nil:
		return nil, 0, fmt.Errorf("failed to read tx RLP prefix: %w", err)
	case kind == rlp.List:
		if txPayload, err = s.Raw(); err != nil {
			return nil, 0, fmt.Errorf("failed to read tx RLP payload: %w", err)
		}
	default:
		return nil, 0, errors.New("tx RLP prefix type must be list")
	}
	txData = append(txData, txPayload...)
	return txData, int(txType), nil
}
