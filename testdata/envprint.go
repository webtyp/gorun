package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("GORUN_TEST_KEY=" + os.Getenv("GORUN_TEST_KEY"))
}
