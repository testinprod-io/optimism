package merge

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path"
	"path/filepath"

	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/convert"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
)

type Config struct {
	Start, End       uint64
	InDirectory      string
	OutDirectory     string
	GenesisTimestamp uint64
	ChainID          *big.Int
}

type Result struct {
	BatchV1sUncompressedSize  int
	SpanBatchUncompressedSize int

	BatchV1sCompressedSize  int
	SpanBatchCompressedSize int

	BatchV1sMetadataSize int // every data size except tx
	BatchV1sTxSize       int

	SpanBatchMetadataSize int // every data size except tx
	SpanBatchTxSize       int

	SpanBatchPrefixSize  int
	SpanBatchPayloadSize int

	UncompressedSizeReductionPercent float64
	CompressedReductionPercent       float64

	L2TxCount int

	L2StartNum   uint64
	L2EndNum     uint64
	L2BlockCount uint64
}

type MergeResult struct {
	Result []Result
}

func LoadBatchV1File(file string) derive.BatchV1 {
	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var out derive.BatchV1
	if err := dec.Decode(&out); err != nil {
		log.Fatalf("Failed to decode %v. Err: %v\n", file, err)
	}
	return out
}

func LoadBatchV1s(config Config) []derive.BatchV1 {
	var batchV1s []derive.BatchV1
	for i := config.Start; i < config.End; i++ {
		file := filepath.Join(config.InDirectory, fmt.Sprintf("%d.json", i))
		batchV1 := LoadBatchV1File(file)
		batchV1s = append(batchV1s, batchV1)
	}
	return batchV1s
}

func writeResult(r MergeResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	return enc.Encode(r)
}

func calcCompressedSize(data []byte) int {
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		log.Fatal(err)
	}
	_, err = w.Write(data)
	if err != nil {
		log.Fatal(err)
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
	return buf.Len()
}

func (r *Result) AnalyzeBatchV1s(batchV1s *[]derive.BatchV1, i int, prevUncompressedSize int) error {
	var buf bytes.Buffer
	if err := rlp.Encode(&buf, (*batchV1s)[i]); err != nil {
		return err
	}
	batchV1sUncompressedSize := prevUncompressedSize + buf.Len()
	buf.Reset()
	r.BatchV1sUncompressedSize = batchV1sUncompressedSize

	var batchV1sEncoded []byte
	for _, batchV1 := range (*batchV1s)[:i+1] {
		batchData := derive.BatchData{
			BatchV1: batchV1,
		}
		batchV1Encoded, err := batchData.MarshalBinary()
		if err != nil {
			return err
		}
		batchV1sEncoded = append(batchV1sEncoded, batchV1Encoded...)
		for _, tx := range batchV1.Transactions {
			r.BatchV1sTxSize += len(tx)
		}
	}
	r.BatchV1sCompressedSize = calcCompressedSize(batchV1sEncoded)
	r.BatchV1sMetadataSize = r.BatchV1sUncompressedSize - r.BatchV1sTxSize
	return nil
}

func (r *Result) AnalyzeBatchV2(batchV2 *derive.BatchV2) error {
	batchV2Encoded, err := batchV2.EncodeBytes()
	r.SpanBatchCompressedSize = calcCompressedSize(batchV2Encoded)
	if err != nil {
		return err
	}
	r.SpanBatchUncompressedSize = len(batchV2Encoded)
	r.SpanBatchMetadataSize, err = batchV2.MetadataSize()
	if err != nil {
		return err
	}
	r.SpanBatchTxSize = r.SpanBatchUncompressedSize - r.SpanBatchMetadataSize
	r.SpanBatchPrefixSize, err = batchV2.PrefixSize()
	if err != nil {
		return err
	}
	r.SpanBatchPayloadSize = r.SpanBatchUncompressedSize - r.SpanBatchPrefixSize
	for _, blockTxCount := range batchV2.BlockTxCounts {
		r.L2TxCount += int(blockTxCount)
	}
	return nil
}

func (r *Result) AnalyzeBatch(config Config, batchV2 *derive.BatchV2, batchV1s *[]derive.BatchV1, i int, prevUncompressedSize int) error {
	if err := r.AnalyzeBatchV1s(batchV1s, i, prevUncompressedSize); err != nil {
		return err
	}
	if err := r.AnalyzeBatchV2(batchV2); err != nil {
		return err
	}
	r.UncompressedSizeReductionPercent = 100 * (1.0 - float64(r.SpanBatchUncompressedSize)/float64(r.BatchV1sUncompressedSize))
	r.CompressedReductionPercent = 100 * (1.0 - float64(r.SpanBatchCompressedSize)/float64(r.BatchV1sCompressedSize))
	r.L2StartNum = config.Start
	r.L2EndNum = config.Start + uint64(i)
	r.L2BlockCount = uint64(i + 1)
	return nil
}

func Merge(client *ethclient.Client, config Config) error {
	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		return err
	}

	batchV1s := LoadBatchV1s(config)
	if len(batchV1s) == 0 {
		return errors.New("batchV1 not available")
	}
	startParent, err := convert.GetL2StartParentBlock(client, batchV1s[0].ParentHash)
	if err != nil {
		return err
	}
	l1OriginHash := batchV1s[0].EpochHash
	originChangedBit, err := convert.GetOriginChangedBit(startParent, l1OriginHash)
	if err != nil {
		return err
	}
	prevUncompressedSize := 0
	var result MergeResult
	// O(N ** 2)
	for i := 0; i < len(batchV1s); i++ {
		var batchV2 derive.BatchV2
		if err := batchV2.MergeBatchV1s(batchV1s[:i+1], originChangedBit, config.GenesisTimestamp); err != nil {
			return err
		}

		var r Result
		r.AnalyzeBatch(config, &batchV2, &batchV1s, i, prevUncompressedSize)
		prevUncompressedSize = r.BatchV1sUncompressedSize

		result.Result = append(result.Result, r)
	}

	filename := path.Join(config.OutDirectory, fmt.Sprintf("%d_%d.json", config.Start, config.End))
	if err := writeResult(result, filename); err != nil {
		return err
	}

	return nil
}
