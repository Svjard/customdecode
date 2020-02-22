package main

/*

  In Terminal:

    # build the file
    go build -o example

    # set the environment variable
    TEST_STRING=Hello

    # run the code
    ./example

    # run the code with a different variable for
    # this particular execution of the program
    TEST_STRING=Goodbye ./example

*/

import (
	"log"
	"os"

	"github.com/Svjard/customdecode"
)

type config struct {
	TestString string `custom:"TEST_STRING"`
}

func FetchEnvVar(s string) string {
    return os.Getenv(s)
}

func main() {

	var cfg config
	if err := customdecode.Decode(&cfg, FetchEnvVar); err != nil {
		log.Fatalf("Failed to decode: %s", err)
	}

	log.Println("TEST_STRING:", cfg.TestString)

}
