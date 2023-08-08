package analyze

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ethereum-optimism/optimism/op-node/cmd/batch_decoder/reassemble"
	"github.com/ethereum-optimism/optimism/op-node/cmd/span_batch_tester/convert"
)

type Config struct {
	InChannelDirectory   string
	InSpanBatchDirectory string
	OutDirectory         string
}

type Result struct {
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
	fmt.Println(file)
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
	return sbm
}

func CompareBatches(channel *reassemble.ChannelWithMetadata, sbm *convert.SpanBatchWithMetadata) {
	// TODO
}

func Analyze(config Config) {
	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		log.Fatal(err)
	}
	channelIDs := LoadChannelIDs(config)
	for _, channelID := range channelIDs {
		channelFilename := path.Join(config.InChannelDirectory, fmt.Sprintf("%s.json", channelID))
		channel := convert.LoadChannelFile(channelFilename)
		batchV2Filename := path.Join(config.InSpanBatchDirectory, fmt.Sprintf("%s.json", channelID))
		batchV2 := LoadSpanBatch(batchV2Filename)
		CompareBatches(&channel, &batchV2)
	}
}
