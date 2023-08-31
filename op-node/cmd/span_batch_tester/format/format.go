package format

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"log"
	"math/big"
	"os"
	"path"

	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/analyze"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/convert"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
)

type Config struct {
	InSpanBatchDirectory string
	OutDirectory         string
	ChainID              *big.Int
	Permutation          []int
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

func Compare(permutation *[]int, batchV2 convert.SpanBatchWithMetadata) (int, int) {
	derive.BatchV2TxsV3FieldPerm = []int{0, 1, 2, 3, 4, 5, 6}
	spanBatchEncoded, err := batchV2.BatchV2.EncodeBytes()
	if err != nil {
		log.Fatal(err)
	}

	spanBatchCompressedSizeA := calcCompressedSize(spanBatchEncoded)

	// choose your permutation
	// derive.BatchV2TxsV3FieldPerm = []int{0, 1, 2, 3, 4, 6, 5}
	derive.BatchV2TxsV3FieldPerm = *permutation

	spanBatchEncoded, err = batchV2.BatchV2.EncodeBytes()
	if err != nil {
		log.Fatal(err)
	}
	spanBatchCompressedSizeB := calcCompressedSize(spanBatchEncoded)

	// if delta is positive, our new permutation gives smaller size
	delta := spanBatchCompressedSizeA - spanBatchCompressedSizeB

	return delta, spanBatchCompressedSizeA
}

func Format(config Config) {
	// update global varibles. Weird but works
	derive.ChainID = config.ChainID

	// out directory currently not used
	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		log.Fatal(err)
	}

	spanBatchFiles, err := os.ReadDir(config.InSpanBatchDirectory)
	if err != nil {
		log.Fatal(err)
	}
	derive.InitializePermutations()

	cnt := 0
	deltaSum := 0
	originalCompressedSizeSum := 0
	for i, spanBatchFile := range spanBatchFiles {
		batchV2Filename := path.Join(config.InSpanBatchDirectory, spanBatchFile.Name())
		fmt.Println(i, batchV2Filename)

		// always reset perm to pass span batch hash check
		derive.BatchV2TxsV3FieldPerm = []int{0, 1, 2, 3, 4, 5, 6}
		batchV2 := analyze.LoadSpanBatch(batchV2Filename)
		delta, originalCompressedSize := Compare(&config.Permutation, batchV2)
		if delta >= 0 {
			cnt++
		}
		deltaSum += delta
		originalCompressedSizeSum += originalCompressedSize
		fmt.Printf("[%d/%d] %d %d %d\n", cnt, len(spanBatchFiles), delta, deltaSum, originalCompressedSizeSum)
		fmt.Println(100 * float64(deltaSum) / float64(originalCompressedSizeSum))
	}
}
