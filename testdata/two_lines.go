package main

import "os"

func main() {
	// One write, two lines: arrives as a single chunk on the pipe.
	os.Stdout.Write([]byte("FIRST_LINE\nSECOND_LINE\n"))
}
