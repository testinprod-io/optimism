package analyze

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ethereum-optimism/optimism/op-node/cmd/batch_decoder/reassemble"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/convert"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

type Config struct {
	InChannelDirectory   string
	InSpanBatchDirectory string
	OutDirectory         string
	TxType               uint
	ChainID              *big.Int
}

type Result struct {
	FrameCount                int
	BatchV1sCompressedSize    int // sum of frame data size
	BatchV1sUncompressedSize  int // channel size
	BatchV1sCompressionRatio  float64
	SpanBatchCompressedSize   int
	SpanBatchUncompressedSize int
	SpanBatchCompressionRatio float64

	BatchV1sMetadataSize int // every data size except tx
	BatchV1sTxSize       int

	SpanBatchMetadataSize int // every data size except tx
	SpanBatchTxSize       int

	SpanBatchPrefixSize  int
	SpanBatchPayloadSize int

	CompressedReductionPercent       float64
	UncompressedSizeReductionPercent float64

	L2TxCount int

	L1StartNum   uint64
	L1EndNum     uint64
	L1BlockCount uint64
	L2StartNum   uint64
	L2EndNum     uint64
	L2BlockCount uint64
}

// Load channel ids which can be analyzed
func LoadChannelIDs(config Config) []string {
	spanBatchFileNameSet := map[string]struct{}{}
	spanBatchFiles, err := os.ReadDir(config.InSpanBatchDirectory)
	if err != nil {
		log.Fatal(err)
	}
	for _, spanBatchFile := range spanBatchFiles {
		spanBatchFileNameSet[spanBatchFile.Name()] = struct{}{}
	}
	var out []string
	channelFiles, err := os.ReadDir(config.InChannelDirectory)
	if err != nil {
		log.Fatal(err)
	}
	for _, channelFile := range channelFiles {
		channelFilename := channelFile.Name()
		_, ok := spanBatchFileNameSet[channelFilename]
		if ok {
			channelID := strings.TrimSuffix(channelFilename, filepath.Ext(channelFilename))
			out = append(out, channelID)
		}
	}
	return out
}

func LoadSpanBatch(file string) convert.SpanBatchWithMetadata {
	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var sbm convert.SpanBatchWithMetadata
	if err := dec.Decode(&sbm); err != nil {
		log.Fatalf("Failed to decode %v. Err: %v\n", file, err)
	}
	batchV2Encoded, err := sbm.BatchV2.EncodeBytes()
	if err != nil {
		log.Fatal(err)
	}
	batchV2Hash := crypto.Keccak256(batchV2Encoded)
	if !bytes.Equal(batchV2Hash, sbm.BatchV2Hash) {
		log.Fatal("Span batch data sanity check failure")
	}
	return sbm
}

func (r *Result) AnalyzeBatchV1s(chm *reassemble.ChannelWithMetadata) {
	r.FrameCount = len(chm.Frames)
	var buf bytes.Buffer
	for _, frame := range chm.Frames {
		if err := frame.Frame.MarshalBinary(&buf); err != nil {
			log.Fatal(err)
		}
		r.BatchV1sCompressedSize += buf.Len()
		buf.Reset()
	}
	// refer to channel_out.go::AddBatch
	for _, batch := range chm.Batches {
		if err := rlp.Encode(&buf, batch); err != nil {
			log.Fatal(err)
		}
		r.BatchV1sUncompressedSize += buf.Len()
		buf.Reset()
		for _, tx := range batch.Transactions {
			r.BatchV1sTxSize += len(tx)
		}
	}
	r.BatchV1sMetadataSize = r.BatchV1sUncompressedSize - r.BatchV1sTxSize
	if r.BatchV1sCompressedSize > r.BatchV1sUncompressedSize {
		log.Fatal("batchV1s compress size is larger than uncompressed")
	}
	if r.BatchV1sCompressedSize == 0 || r.BatchV1sUncompressedSize == 0 {
		log.Fatal("batchV1s size is 0")
	}
	r.BatchV1sCompressionRatio = float64(r.BatchV1sCompressedSize) / float64(r.BatchV1sUncompressedSize)
}

func (r *Result) AnalyzeBatchV2(sbm *convert.SpanBatchWithMetadata) {
	spanBatchEncoded, err := sbm.BatchV2.EncodeBytes()
	if err != nil {
		log.Fatal(err)
	}
	r.SpanBatchUncompressedSize = len(spanBatchEncoded)
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		log.Fatal(err)
	}
	_, err = w.Write(spanBatchEncoded)
	if err != nil {
		log.Fatal(err)
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
	r.SpanBatchCompressedSize = buf.Len()
	if r.SpanBatchCompressedSize > r.SpanBatchUncompressedSize {
		log.Fatal("Span batch compress size is larger than uncompressed")
	}
	if r.SpanBatchCompressedSize == 0 || r.SpanBatchUncompressedSize == 0 {
		log.Fatal("Span batch size is 0")
	}
	for _, blockTxCount := range sbm.BatchV2.BlockTxCounts {
		r.L2TxCount += int(blockTxCount)
	}
	r.SpanBatchPrefixSize, err = sbm.BatchV2.PrefixSize()
	if err != nil {
		log.Fatal(err)
	}
	r.SpanBatchMetadataSize, err = sbm.BatchV2.MetadataSize()
	if err != nil {
		log.Fatal(err)
	}
	r.SpanBatchTxSize = r.SpanBatchUncompressedSize - r.SpanBatchMetadataSize
	r.SpanBatchPayloadSize = r.SpanBatchUncompressedSize - r.SpanBatchPrefixSize
	r.SpanBatchCompressionRatio = float64(r.SpanBatchCompressedSize) / float64(r.SpanBatchUncompressedSize)
	r.L1StartNum = sbm.L1StartNum
	r.L1EndNum = sbm.L1EndNum
	r.L1BlockCount = sbm.L1EndNum - sbm.L1StartNum + 1
	r.L2StartNum = sbm.L2StartNum
	r.L2EndNum = sbm.L2EndNum
	r.L2BlockCount = sbm.L2EndNum - sbm.L2StartNum + 1
}

func (r *Result) AnalyzeBatch(chm *reassemble.ChannelWithMetadata, sbm *convert.SpanBatchWithMetadata) {
	r.AnalyzeBatchV1s(chm)
	r.AnalyzeBatchV2(sbm)
	r.CompressedReductionPercent = 100.0 * (1.0 - float64(r.SpanBatchCompressedSize)/float64(r.BatchV1sCompressedSize))
	r.UncompressedSizeReductionPercent = 100 * (1.0 - float64(r.SpanBatchUncompressedSize)/float64(r.BatchV1sUncompressedSize))
}

func writeResult(r Result, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	return enc.Encode(r)
}

func Analyze(config Config) {
	// update global varibles. Weird but works
	derive.BatchV2TxsType = int(config.TxType)
	derive.ChainID = config.ChainID

	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		log.Fatal(err)
	}
	channelIDs := LoadChannelIDs(config)
	numChannels := len(channelIDs)
	for idx, channelID := range channelIDs {
		channelFilename := path.Join(config.InChannelDirectory, fmt.Sprintf("%s.json", channelID))
		channel := convert.LoadChannelFile(channelFilename)
		batchV2Filename := path.Join(config.InSpanBatchDirectory, fmt.Sprintf("%s.json", channelID))
		batchV2 := LoadSpanBatch(batchV2Filename)
		var result Result
		result.AnalyzeBatch(&channel, &batchV2)
		filename := path.Join(config.OutDirectory, fmt.Sprintf("%s.json", channelID))
		if err := writeResult(result, filename); err != nil {
			log.Fatal(err)
		}
		logPrefix := fmt.Sprintf("[%d/%d]", idx+1, numChannels)
		fmt.Printf(logPrefix+" Channel ID: %s, CompressedReductionPercent: %f %%\n", channelID, result.CompressedReductionPercent)
	}
}
