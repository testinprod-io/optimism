package analyze

import "fmt"

type Config struct {
	InChannelDirectory            string
	InSpanBatchDirectoryDirectory string
	OutDirectory                  string
}

func Analyze(config Config) {
	fmt.Println("eeor")
}
