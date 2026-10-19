package qualification

import "os"

func platformMemory() (uint64, error) { return linuxMemory(os.DirFS("/")) }
