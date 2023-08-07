package convert

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/ethereum-optimism/optimism/op-node/cmd/batch_decoder/reassemble"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
)

type Config struct {
	InDirectory      string
	OutDirectory     string
	GenesisTimestamp uint64
}

type ChannelWithConversion struct {
	reassemble.ChannelWithMetadata
	BatchV2 derive.BatchV2
}

func LoadChannels(dir string) []reassemble.ChannelWithMetadata {
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	var out []reassemble.ChannelWithMetadata
	for _, file := range files {
		f := path.Join(dir, file.Name())
		chm := loadChannelFile(f)
		out = append(out, chm)
	}
	return out
}

func loadChannelFile(file string) reassemble.ChannelWithMetadata {
	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var chm reassemble.ChannelWithMetadata
	if err := dec.Decode(&chm); err != nil {
		log.Fatalf("Failed to decode %v. Err: %v\n", file, err)
	}
	return chm
}

func ConvertChannel(chm reassemble.ChannelWithMetadata, genesisTimestamp uint64) {
	framesDataLen := 0
	for _, frame := range chm.Frames {
		framesDataLen += len(frame.Frame.Data)
	}
	fmt.Println("number of frames: ", len(chm.Frames))
	fmt.Println("number of frame data sum(zlib): ", framesDataLen)

	// TODO: get real value using L2
	originChangedBit := uint(1)

	var batchV2 derive.BatchV2
	if err := batchV2.MergeBatchV1s(chm.Batches, originChangedBit, genesisTimestamp); err != nil {
		log.Fatal(err)
	}

	enc, err := batchV2.EncodeBytes()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("batchV2 marshal size(not zlibbed): ", len(enc))
	var b bytes.Buffer
	w, err := zlib.NewWriterLevel(&b, zlib.BestCompression)
	if err != nil {
		log.Fatal(err)
	}
	w.Write(enc)
	w.Close()

	fmt.Println("batchV2 marshal size(zlib): ", len(b.Bytes()))

	percent := 100.0 * (1.0 - float64(len(b.Bytes()))/float64(framesDataLen))
	fmt.Println("percent: ", percent)
}

func Convert(config Config) error {
	channels := LoadChannels(config.InDirectory)
	for _, channel := range channels {
		if !channel.IsReady {
			continue
		}
		ConvertChannel(channel, config.GenesisTimestamp)
	}
	return nil
}
