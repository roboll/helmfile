package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

// Copyright (c) 2017 Roland Singer [roland.singer@desertbit.com]
//
// Shamelessly borrowed from @r0l1's awesome work that is available at https://gist.github.com/r0l1/3dcbb0c8f6cfe9c66ab8008f55f8f28b
func askForConfirmation(s string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s [y/n]: ", s)

		response, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" {
			return true
		} else if response == "n" || response == "no" {
			return false
		}
	}
}
