package convert

import (
	"encoding/json"
	"log"
	"os"
	"path"

	"github.com/ethereum-optimism/optimism/op-node/cmd/batch_decoder/reassemble"
)

type Config struct {
	InDirectory  string
	OutDirectory string
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

func Convert(config Config) error {
	channels := LoadChannels(config.InDirectory)
	for _, channel := range channels {
		if !channel.IsReady {
			continue
		}
		// TODO: convert channel
	}
	return nil
}
