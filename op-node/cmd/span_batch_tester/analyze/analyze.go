package analyze

import (
	"fmt"
	"log"
	"os"
)

type Config struct {
	InChannelDirectory            string
	InSpanBatchDirectoryDirectory string
	OutDirectory                  string
}

type Result struct {
}

// Load channel ids which can be analyzed
func LoadChannelIDs(config Config) []string {
	spanBatchFileNameSet := map[string]struct{}{}
	spanBatchFiles, err := os.ReadDir(config.InSpanBatchDirectoryDirectory)
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
		_, ok := spanBatchFileNameSet[channelFile.Name()]
		if ok {
			out = append(out, channelFile.Name())
			fmt.Println(channelFile.Name())
		}
	}
	return out
}

func Analyze(config Config) {
	if err := os.MkdirAll(config.OutDirectory, 0750); err != nil {
		log.Fatal(err)
	}
	channelIDs := LoadChannelIDs(config)
	_ = channelIDs
}
