// This real executable proves that an imported installer can copy and launch
// a payload on the host OS. It never writes outside its installed directory.
package main

import (
	"os"
	"path/filepath"
	"strconv"
)

func main() {
	executable, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(executable), "launched.txt"), []byte("launched "+strconv.Itoa(os.Getpid())), 0600); err != nil {
		os.Exit(1)
	}
}
