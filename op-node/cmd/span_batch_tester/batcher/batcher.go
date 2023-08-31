package batcher

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/convert"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
)

type Config struct {
	Start, End       uint64
	InDirectory      string
	OutDirectory     string
	GenesisTimestamp uint64
	ChannelSize      int
	ChainID          *big.Int
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

func Batcher(client *ethclient.Client, config Config) error {
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

	var singularResultBuf bytes.Buffer
	channelStartIdx := 0
	for i := 0; i < len(batchV1s); i++ {
		var compBuf bytes.Buffer
		comp, _ := zlib.NewWriterLevel(&compBuf, zlib.BestCompression)
		for _, batch := range batchV1s[channelStartIdx : i+1] {
			var buf bytes.Buffer
			rlp.Encode(&buf, derive.InitBatchDataV1(batch))
			comp.Write(buf.Bytes())
		}
		comp.Flush()
		if compBuf.Len() > config.ChannelSize {
			fmt.Println("SINGULAR_CHANNEL", channelStartIdx, i-1, i-channelStartIdx, singularResultBuf.Len())
			singularResultBuf.Reset()
			channelStartIdx = i
		} else {
			singularResultBuf.Reset()
			io.Copy(&singularResultBuf, &compBuf)
		}
	}
	fmt.Println("SINGULAR_CHANNEL", channelStartIdx, len(batchV1s)-1, len(batchV1s)-channelStartIdx, singularResultBuf.Len())

	var spanResultBuf bytes.Buffer
	channelStartIdx = 0
	for i := 0; i < len(batchV1s); i++ {
		var compBuf bytes.Buffer
		comp, _ := zlib.NewWriterLevel(&compBuf, zlib.BestCompression)
		var batchV2 derive.BatchV2
		if err := batchV2.MergeBatchV1s(batchV1s[channelStartIdx:i+1], originChangedBit, config.GenesisTimestamp); err != nil {
			return err
		}
		var buf bytes.Buffer
		rlp.Encode(&buf, derive.InitBatchDataV2(batchV2))
		comp.Write(buf.Bytes())
		comp.Flush()
		if compBuf.Len() > config.ChannelSize {
			fmt.Println("SPAN_CHANNEL", channelStartIdx, i-1, i-channelStartIdx, spanResultBuf.Len())
			spanResultBuf.Reset()
			channelStartIdx = i
		} else {
			spanResultBuf.Reset()
			io.Copy(&spanResultBuf, &compBuf)
		}
	}
	fmt.Println("SPAN_CHANNEL", channelStartIdx, len(batchV1s)-1, len(batchV1s)-channelStartIdx, spanResultBuf.Len())

	return nil
}
